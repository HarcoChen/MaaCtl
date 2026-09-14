package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"maactl/internal/event"
	"maactl/internal/maafw"
	"maactl/internal/output"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// execute runs a prepared task: pretasks, resource loading, agents, controller,
// and the Pipeline itself, mapping each failure onto its documented exit code.
func (p *preparedRun) execute() error {
	mode, err := event.ParseMode(p.opt.events)
	if err != nil {
		return exitErrorf(ExitUsage, "%v", err)
	}
	display, err := event.ParseDisplay(p.opt.focusDisplay)
	if err != nil {
		return exitErrorf(ExitUsage, "%v", err)
	}
	if err := p.initFramework(); err != nil {
		return err
	}
	defer func() { _ = maa.Release() }()

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignal)

	if err := p.runPretasks(); err != nil {
		return err
	}

	res, err := p.loadResource()
	if err != nil {
		return err
	}
	defer res.Destroy()

	agents, err := p.startAgents(res, stopSignal)
	if err != nil {
		return err
	}
	defer stopAgents(agents)

	ctrl, err := p.createController()
	if err != nil {
		return err
	}
	defer ctrl.Destroy()
	fmt.Fprintf(os.Stderr, "connecting controller %s\n", p.controller.Name)
	if !ctrl.PostConnect().Wait().Success() {
		return exitErrorf(ExitController, "connect controller %q", p.controller.Name)
	}

	tasker, err := maa.NewTasker()
	if err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("create Maa tasker: %w", err))
	}
	defer tasker.Destroy()
	if err := tasker.BindResource(res); err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("bind Maa resource: %w", err))
	}
	if err := tasker.BindController(ctrl); err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("bind Maa controller: %w", err))
	}
	if !tasker.Initialized() {
		return exitErrorf(ExitInternal, "initialize Maa tasker")
	}

	sink := &event.Sink{JSON: p.global.JSON, Mode: mode, Display: display}
	tasker.AddSink(&event.TaskerSink{Sink: sink})
	res.AddSink(&event.ResourceSink{Sink: sink})
	ctrl.AddSink(&event.ControllerSink{Sink: sink})
	tasker.AddContextSink(&event.ContextSink{Sink: sink})

	return p.runTaskOn(tasker, stopSignal)
}

// initFramework loads MaaFramework, honouring --lib-dir and --log-dir.
func (p *preparedRun) initFramework() error {
	libDir, err := maafw.ResolveLibDir(p.global.LibDir)
	if err != nil {
		return withExitCode(ExitInternal, err)
	}
	if err := maafw.Init(libDir, p.global.LogDir); err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err))
	}
	return nil
}

// loadResource creates the MaaFramework resource, loads the selected paths in
// order, verifies resource.hash after the base paths, and finally loads any
// controller.attach_resource_path entries and --overlay directories.
func (p *preparedRun) loadResource() (*maa.Resource, error) {
	res, err := maa.NewResource()
	if err != nil {
		return nil, withExitCode(ExitResource, fmt.Errorf("create Maa resource: %w", err))
	}
	paths := p.resourcePaths()
	for _, path := range paths {
		if !isDirectory(path) {
			res.Destroy()
			return nil, exitErrorf(ExitResource, "resource path %s is not a directory", path)
		}
		if job := res.PostBundle(path).Wait(); !job.Success() {
			res.Destroy()
			return nil, exitErrorf(ExitResource, "load resource %s: %s", path, job.Status())
		}
	}
	// Protocol: resource.hash covers only resource.path, before
	// attach_resource_path. Verification never blocks the run unless the caller
	// asked for it.
	if err := p.verifyHash(res); err != nil {
		res.Destroy()
		return nil, err
	}
	for _, path := range p.controller.AttachResourcePath {
		full := resolveAgainst(p.project.Dir, path)
		if !isDirectory(full) {
			res.Destroy()
			return nil, exitErrorf(ExitResource, "attach_resource_path %s is not a directory", full)
		}
		if job := res.PostBundle(full).Wait(); !job.Success() {
			res.Destroy()
			return nil, exitErrorf(ExitResource, "load controller.attach_resource_path %s: %s", full, job.Status())
		}
	}
	return res, nil
}

// verifyHash compares the loaded hash with resource.hash.
func (p *preparedRun) verifyHash(res *maa.Resource) error {
	if strings.TrimSpace(p.resource.Hash) == "" {
		return nil
	}
	actual, err := res.GetHash()
	if err != nil {
		return withExitCode(ExitResource, fmt.Errorf("read resource hash: %w", err))
	}
	if actual == p.resource.Hash {
		return nil
	}
	message := fmt.Sprintf("resource %s hash mismatch: got %s, expected %s; the resource package may need to be re-downloaded", p.resource.Name, actual, p.resource.Hash)
	if p.opt.requireHash {
		return exitErrorf(ExitResource, "%s", message)
	}
	fmt.Fprintf(os.Stderr, "warning: %s\n", message)
	return nil
}

// runPretasks runs the declared pretasks that apply, in order, before the
// controller is created. A non-zero exit stops the run.
func (p *preparedRun) runPretasks() error {
	for i := range p.project.Pretask {
		pretask := &p.project.Pretask[i]
		if !pi.Compatible(pretask.Controller, p.controller.Name) || !pi.Compatible(pretask.Resource, p.resource.Name) {
			continue
		}
		args, err := p.pretaskArgs(pretask)
		if err != nil {
			return err
		}
		name := pretask.Identifier()
		fmt.Fprintf(os.Stderr, "pretask %s: %s %s\n", name, pretask.Exec, strings.Join(args, " "))
		cmd := exec.Command(execPath(p.project.Dir, pretask.Exec), args...)
		cmd.Dir = p.project.Dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return withExitCode(ExitPretask, fmt.Errorf("pretask %s failed: %w", name, err))
		}
	}
	return nil
}

// pretaskArgs returns the fixed arguments plus, when the pretask declares
// options, one compact JSON argument with their current values. Password values
// are passed in clear (the pretask needs them) but never logged.
func (p *preparedRun) pretaskArgs(pretask *pi.Pretask) ([]string, error) {
	if len(pretask.Option) == 0 {
		return append([]string(nil), pretask.Args...), nil
	}
	values := map[string]any{}
	for _, name := range pretask.Option {
		value, ok := p.resolution.Value(name)
		if !ok {
			// The option is not part of this task's layers; resolve it on its own.
			resolved, err := p.project.ResolveOption(name, p.optionRequest(nil, nil, nil))
			if err != nil {
				return nil, withExitCode(ExitUsage, err)
			}
			if resolved == nil {
				continue
			}
			value = resolved
		}
		values[name] = value
	}
	if len(values) == 0 {
		return append([]string(nil), pretask.Args...), nil
	}
	serialized, err := json.Marshal(values)
	if err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("serialize pretask options: %w", err))
	}
	return append(append([]string(nil), pretask.Args...), string(serialized)), nil
}

// optionRequest rebuilds a Resolve request without the task-specific values, for
// options resolved outside the task's layer list.
func (p *preparedRun) optionRequest(cliValues, optionFile map[string]any, presetEntry *pi.PresetTask) pi.Request {
	var presetOptions map[string]any
	if presetEntry != nil {
		presetOptions = presetEntry.Option
	}
	var configTask map[string]any
	if p.task != nil {
		configTask = p.config.TaskOptions(p.task.Name)
	}
	return pi.Request{
		ControllerName: p.controller.Name,
		ResourceName:   p.resource.Name,
		Task:           p.task,
		ConfigGlobal:   p.config.GlobalOptions(),
		ConfigTask:     configTask,
		PresetOptions:  presetOptions,
		OptionFile:     optionFile,
		CLI:            cliValues,
	}
}

// startAgents starts and connects the declared agents with the PI_* environment
// the protocol requires.
func (p *preparedRun) startAgents(res *maa.Resource, stop <-chan os.Signal) ([]*agentProcess, error) {
	env, err := p.agentEnv()
	if err != nil {
		return nil, err
	}
	specs := declaredAgents(p.project.Agent)
	if len(specs) == 0 {
		return nil, nil
	}
	if p.opt.noAgent {
		fmt.Fprintf(os.Stderr, "not starting %d declared agent(s): --no-agent is set\n", len(specs))
		return nil, nil
	}
	return startAgents(p.project, res, p.resource.Name, p.opt, stop, env)
}

// agentEnv builds the PI_* environment variables defined by protocol v2.5.0.
func (p *preparedRun) agentEnv() ([]string, error) {
	translator := p.project.Translator(p.global.Language())
	controllerJSON, err := json.Marshal(translator.ResolveLabels(p.controller))
	if err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("encode PI_CONTROLLER: %w", err))
	}
	resourceJSON, err := json.Marshal(translator.ResolveLabels(p.resource))
	if err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("encode PI_RESOURCE: %w", err))
	}
	clientVersionValue := clientVersion
	return []string{
		"PI_INTERFACE_VERSION=" + pi.ProtocolVersion,
		"PI_CLIENT_NAME=" + pi.ClientName,
		"PI_CLIENT_VERSION=" + clientVersionValue,
		"PI_CLIENT_LANGUAGE=" + p.global.Language(),
		"PI_CLIENT_MAAFW_VERSION=" + maafw.Version(),
		"PI_VERSION=" + p.project.Version,
		"PI_CONTROLLER=" + string(controllerJSON),
		"PI_RESOURCE=" + string(resourceJSON),
	}, nil
}

// createController builds the MaaFramework controller for the selected PI
// controller, using run flags and the client config for anything PI leaves open.
func (p *preparedRun) createController() (*maa.Controller, error) {
	ctrl, err := createController(p.controller, p.opt, p.config)
	if err != nil {
		return nil, withExitCode(ExitController, err)
	}
	return ctrl, nil
}

// runTaskOn posts the task and waits, honouring --stop-after, --timeout, and
// interrupts.
func (p *preparedRun) runTaskOn(tasker *maa.Tasker, stopSignal <-chan os.Signal) error {
	started := time.Now()
	fmt.Fprintf(os.Stderr, "running %s (resource=%s controller=%s)\n", p.entry, p.resource.Name, p.controller.Name)
	job := tasker.PostTask(p.entry, p.override)

	done := make(chan maa.Status, 1)
	go func() { done <- job.Wait().Status() }()

	var stopAfter <-chan time.Time
	if p.opt.stopAfter > 0 {
		timer := time.NewTimer(p.opt.stopAfter)
		defer timer.Stop()
		stopAfter = timer.C
	}
	var deadline <-chan time.Time
	if p.opt.timeout > 0 {
		timer := time.NewTimer(p.opt.timeout)
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-stopSignal:
		tasker.PostStop()
		return exitErrorf(ExitInterrupted, "interrupted")
	case <-stopAfter:
		tasker.PostStop()
		time.Sleep(250 * time.Millisecond)
		p.report("StoppedAfterDuration", started)
		return nil
	case <-deadline:
		tasker.PostStop()
		time.Sleep(250 * time.Millisecond)
		p.report("Timeout", started)
		return exitErrorf(ExitTimeout, "task %q timed out after %s", p.entry, p.opt.timeout)
	case status := <-done:
		if !status.Success() {
			p.report(status.String(), started)
			return exitErrorf(ExitTask, "task %q finished with %s", p.entry, status)
		}
		p.report(status.String(), started)
		return nil
	}
}

// report prints the run summary in text or JSON.
func (p *preparedRun) report(status string, started time.Time) {
	summary := buildSummary(p, status, time.Since(started))
	if p.global.JSON {
		_ = output.JSON(os.Stdout, summary)
		return
	}
	fmt.Fprintf(os.Stdout, "%s: %s (%d ms)\n", status, p.entry, summary.ElapsedMS)
}

// execPath resolves an executable path. A bare name stays untouched so
// exec.Command finds it through PATH; a relative path resolves against dir.
func execPath(dir, exec string) string {
	if filepath.IsAbs(exec) || !strings.ContainsAny(exec, `/\`) {
		return exec
	}
	return filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(exec, `\`, "/")))
}

// isDirectory reports whether path is an existing directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
