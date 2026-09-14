package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	var agent *exec.Cmd
	if !opt.noAgent && project.Agent != nil && project.Agent.ChildExec != "" {
		execPath := project.Agent.ChildExec
		if !filepath.IsAbs(execPath) {
			execPath = filepath.Join(project.Dir, execPath)
		}
		agent = exec.Command(execPath, project.Agent.ChildArgs...)
		agent.Dir = project.Dir
		agent.Stdout, agent.Stderr = os.Stdout, os.Stderr
		if err := agent.Start(); err != nil {
			return fmt.Errorf("start agent: %w", err)
		}
		defer func() {
			if agent.Process != nil {
				_ = agent.Process.Kill()
				_, _ = agent.Process.Wait()
			}
		}()
	}
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
	address, adbPath, config := opt.adbAddress, "", ""
	if address == "" {
		devices := maa.FindAdbDevices()
		if len(devices) == 0 {
			return nil, fmt.Errorf("no ADB devices found; specify --adb-address/-a after connecting a device")
		}
		if len(devices) > 1 {
			names := make([]string, len(devices))
			for i, d := range devices {
				names[i] = d.Address
			}
			return nil, fmt.Errorf("%d ADB devices found; specify --adb-address/-a: %s", len(devices), pi.Join(names))
		}
		address = devices[0].Address
		adbPath = devices[0].AdbPath
		config = devices[0].Config
	}
	sc, err := adb.ParseScreencapMethod(spec.Adb.Screencap)
	if err != nil {
		return nil, err
	}
	if spec.Adb.Screencap == "" {
		sc = adb.ScreencapDefault
	}
	in, err := adb.ParseInputMethod(spec.Adb.Input)
	if err != nil {
		return nil, err
	}
	if spec.Adb.Input == "" {
		in = adb.InputDefault
	}
	ctrl := maa.NewAdbController(adbPath, address, sc, in, config, "")
	if ctrl == nil {
		return nil, fmt.Errorf("create ADB controller for %s", address)
	}
	return ctrl, nil
}
