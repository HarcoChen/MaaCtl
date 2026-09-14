package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"maactl/internal/event"
	"maactl/internal/maafw"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/MaaXYZ/maa-framework-go/v3/controller/adb"
)

func execute(global *Options, project *pi.Loaded, piCtrl *pi.Controller, piRes *pi.Resource, entry string, override any, opt runOptions) error {
	mode, err := event.ParseMode(opt.events)
	if err != nil {
		return err
	}
	libDir, err := maafw.ResolveLibDir(global.LibDir)
	if err != nil {
		return err
	}
	if err := maafw.Init(libDir); err != nil {
		return fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err)
	}
	defer func() { _ = maa.Release() }()
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignal)

	res := maa.NewResource()
	if res == nil {
		return fmt.Errorf("create Maa resource")
	}
	defer res.Destroy()
	for _, p := range piRes.Path {
		full := filepath.Join(project.Dir, p)
		job := res.PostBundle(full).Wait()
		if !job.Success() {
			return fmt.Errorf("load resource %s: %s", full, job.Status())
		}
	}
	// Agents start once the resource is loaded: the AgentClient is bound to that
	// resource, so the agent's custom recognitions and actions are available to
	// the resource the task runs on.
	agents, err := startAgents(project, res, piRes.Name, opt, stopSignal)
	if err != nil {
		return err
	}
	defer stopAgents(agents)
	ctrl, err := createController(piCtrl, opt)
	if err != nil {
		return err
	}
	defer ctrl.Destroy()
	if !ctrl.PostConnect().Wait().Success() {
		return fmt.Errorf("connect controller %q", piCtrl.Name)
	}
	tasker := maa.NewTasker()
	if tasker == nil {
		return fmt.Errorf("create Maa tasker")
	}
	defer tasker.Destroy()
	if !tasker.BindResource(res) || !tasker.BindController(ctrl) || !tasker.Initialized() {
		return fmt.Errorf("initialize Maa tasker")
	}
	sink := &event.Sink{JSON: global.JSON, Mode: mode}
	tasker.AddSink(&event.TaskerSink{Sink: sink})
	// MaaFramework v5.13 emits Pipeline node notifications on the context sink.
	// The ordinary tasker sink only receives Resource/Controller/Tasker events.
	tasker.AddContextSink(&event.ContextSink{Sink: sink})

	fmt.Printf("Running %s (resource=%s controller=%s)\n", entry, piRes.Name, piCtrl.Name)
	job := tasker.PostTask(entry, override)
	if opt.stopAfter > 0 {
		done := make(chan maa.Status, 1)
		go func() { done <- job.Wait().Status() }()
		select {
		case <-stopSignal:
			tasker.PostStop()
			return fmt.Errorf("task interrupted")
		case status := <-done:
			if !status.Success() {
				return fmt.Errorf("task %q finished with %s before --stop-after", entry, status)
			}
			fmt.Println("Task succeeded")
			return nil
		case <-time.After(opt.stopAfter):
			fmt.Fprintf(os.Stderr, "Stopping task after %s\n", opt.stopAfter)
			// PostStop is asynchronous.  Waiting for the original infinite task or
			// the stop job can itself block forever on a misbehaving pipeline.
			// Send the framework stop signal, then give callbacks a brief chance to flush.
			tasker.PostStop()
			time.Sleep(250 * time.Millisecond)
			fmt.Println("Task stopped by --stop-after")
			return nil
		}
	}
	select {
	case <-stopSignal:
		tasker.PostStop()
		return fmt.Errorf("task interrupted")
	default:
	}
	job.Wait()
	if !job.Success() {
		return fmt.Errorf("task %q finished with %s", entry, job.Status())
	}
	fmt.Println("Task succeeded")
	return nil
}

func createController(spec *pi.Controller, opt runOptions) (*maa.Controller, error) {
	if spec.Type != "Adb" {
		return nil, fmt.Errorf("controller %q has unsupported type %q; this build supports Adb task execution", spec.Name, spec.Type)
	}
	// Every ADB connection detail comes from MaaToolkit, so the controller is
	// built from information MaaFramework already discovered and validated. An
	// empty ADB path in particular makes MaaAdbControllerCreate fail, which is
	// why the address alone is never enough.
	device, err := resolveAdbDevice(opt.adbAddress, opt.adbName)
	if err != nil {
		return nil, err
	}
	// MaaToolkit recommends per-device screencap and input methods; the PI
	// controller overrides them when it declares its own.
	sc := device.ScreencapMethod
	if sc == adb.ScreencapNone {
		sc = adb.ScreencapDefault
	}
	if spec.Adb.Screencap != "" {
		sc, err = adb.ParseScreencapMethod(spec.Adb.Screencap)
		if err != nil {
			return nil, err
		}
	}
	in := device.InputMethod
	if in == adb.InputNone {
		in = adb.InputDefault
	}
	if spec.Adb.Input != "" {
		in, err = adb.ParseInputMethod(spec.Adb.Input)
		if err != nil {
			return nil, err
		}
	}
	ctrl := maa.NewAdbController(device.AdbPath, device.Address, sc, in, device.Config, "")
	if ctrl == nil {
		return nil, fmt.Errorf("create ADB controller for %s (adb %s)", device.Address, device.AdbPath)
	}
	return ctrl, nil
}

// resolveAdbDevice picks the ADB device to connect to from the devices
// MaaToolkit discovered. --adb-address matches the device address and --name
// matches the device name; with neither, the only detected device is used.
// The returned device carries the ADB path and config needed to create a
// controller, so callers must pass those to MaaFramework instead of rebuilding
// them from the address.
func resolveAdbDevice(address, name string) (*maa.AdbDevice, error) {
	devices := maa.FindAdbDevices()
	if len(devices) == 0 {
		return nil, fmt.Errorf("no ADB devices found; connect a device and check \"maactl adb devices\"")
	}
	if address == "" && name == "" {
		if len(devices) > 1 {
			return nil, fmt.Errorf("%d ADB devices found; specify --adb-address/-a or --name: %s", len(devices), describeAdbDevices(devices))
		}
		return devices[0], nil
	}
	var matched []*maa.AdbDevice
	for _, device := range devices {
		if address != "" && !strings.EqualFold(device.Address, address) {
			continue
		}
		if name != "" && !strings.EqualFold(device.Name, name) {
			continue
		}
		matched = append(matched, device)
	}
	switch len(matched) {
	case 0:
		return nil, fmt.Errorf("no ADB device matches %s; detected: %s", adbSelector(address, name), describeAdbDevices(devices))
	case 1:
		return matched[0], nil
	default:
		return nil, fmt.Errorf("%d ADB devices match %s: %s", len(matched), adbSelector(address, name), describeAdbDevices(matched))
	}
}

// describeAdbDevices renders discovered devices for error messages, including
// both the address and the name needed for --adb-address/--name.
func describeAdbDevices(devices []*maa.AdbDevice) string {
	descriptions := make([]string, len(devices))
	for i, device := range devices {
		descriptions[i] = fmt.Sprintf("%s (%s)", device.Address, device.Name)
	}
	return pi.Join(descriptions)
}

// adbSelector labels the --adb-address/--name filter in error messages.
func adbSelector(address, name string) string {
	var parts []string
	if address != "" {
		parts = append(parts, fmt.Sprintf("--adb-address %s", address))
	}
	if name != "" {
		parts = append(parts, fmt.Sprintf("--name %s", name))
	}
	return pi.Join(parts)
}
