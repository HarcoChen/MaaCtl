package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"maactl/internal/event"
	"maactl/internal/maafw"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

func execute(global *Options, project *pi.Loaded, piCtrl *pi.Controller, piRes *pi.Resource, entry string, override any, opt runOptions) error {
	if opt.stopAfter < 0 || opt.stopTimeout <= 0 {
		return fmt.Errorf("--stop-after must be nonnegative and --stop-timeout must be positive")
	}
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
	safeToDestroy := true
	defer func() {
		if safeToDestroy {
			_ = maa.Release()
		}
	}()
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignal)

	res, err := maa.NewResource()
	if err != nil {
		return fmt.Errorf("create Maa resource: %w", err)
	}
	defer func() {
		if safeToDestroy {
			res.Destroy()
		}
	}()
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
	defer func() {
		if safeToDestroy {
			stopAgents(agents)
		} else {
			// Native calls may still be running. Kill children without destroying
			// AgentClient/Tasker objects still in use; process exit reclaims them.
			for _, agent := range agents {
				agent.stopProcess()
			}
		}
	}()
	ctrl, err := createController(piCtrl, opt)
	if err != nil {
		return err
	}
	defer func() {
		if safeToDestroy {
			ctrl.Destroy()
		}
	}()
	if !ctrl.PostConnect().Wait().Success() {
		return fmt.Errorf("connect controller %q", piCtrl.Name)
	}
	tasker, err := maa.NewTasker()
	if err != nil {
		return fmt.Errorf("create Maa tasker: %w", err)
	}
	defer func() {
		if safeToDestroy {
			tasker.Destroy()
		}
	}()
	if err := tasker.BindResource(res); err != nil {
		return fmt.Errorf("bind Maa resource: %w", err)
	}
	if err := tasker.BindController(ctrl); err != nil {
		return fmt.Errorf("bind Maa controller: %w", err)
	}
	if !tasker.Initialized() {
		return fmt.Errorf("initialize Maa tasker")
	}
	sink := &event.Sink{JSON: global.JSON, Mode: mode}
	tasker.AddSink(&event.TaskerSink{Sink: sink})
	res.AddSink(&event.ResourceSink{Sink: sink})
	ctrl.AddSink(&event.ControllerSink{Sink: sink})
	// Pipeline node notifications arrive on the context sink; the tasker and
	// resource/controller sinks carry their own event categories.
	tasker.AddContextSink(&event.ContextSink{Sink: sink})

	fmt.Printf("Running %s (resource=%s controller=%s)\n", entry, piRes.Name, piCtrl.Name)
	job := tasker.PostTask(entry, override)
	err = waitExecution(
		func() maa.Status { return job.Wait().Status() },
		func() maa.Status { return tasker.PostStop().Wait().Status() },
		stopSignal, opt.stopAfter, opt.stopTimeout,
	)
	var stopErr *stopError
	if errors.As(err, &stopErr) {
		safeToDestroy = false
	}
	if err != nil {
		return err
	}
	fmt.Println("Task finished")
	return nil
}
