package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"maactl/internal/event"
	"maactl/internal/maafw"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
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
