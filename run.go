package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/MaaXYZ/maa-framework-go/v3/controller/adb"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type runOptions struct {
	resource, controller, adbAddress string
	override, overrideFile           string
	optionValues, overlay            []string
	events                           string
	stopAfter                        time.Duration
	noAgent                          bool
}

func newRunCommand(global *cliOptions) *cobra.Command {
	var taskName, nodeName string
	var opt runOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: localized("Run PI tasks or Pipeline nodes", "运行 PI task 或 Pipeline 节点"),
		Long: localized(`Run a task declared in ProjectInterface, or run a Pipeline node directly.

Give exactly one of --task/-t, --node/-n, or a subcommand:
  maactl run -t <task-name>
  maactl run -n <node-name>`, `运行 ProjectInterface 中声明的 task，或直接运行 Pipeline 节点。

--task/-t、--node/-n 和子命令三者只能选其一：
  maactl run -t <task-name>
  maactl run -n <node-name>`),
		Example: `  maactl run -t "自动挂机卖蛋" -f D:\MaaMio --stop-after 10s
  maactl run node "签到-开始签到" --adb-address 127.0.0.1:16384 --events all`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if taskName != "" && nodeName != "" {
				return fmt.Errorf("choose only one run shortcut: -t/--task or -n/--node")
			}
			if taskName != "" {
				return runTask(global, taskName, opt)
			}
			if nodeName != "" {
				return runNode(global, nodeName, opt)
			}
			return c.Help()
		},
	}
	cmd.Flags().StringVarP(&taskName, "task", "t", "", localized(`shortcut for "maactl run task <task-name>"`, `等价于 "maactl run task <task-name>"`))
	cmd.Flags().StringVarP(&nodeName, "node", "n", "", localized(`shortcut for "maactl run node <node-name>"`, `等价于 "maactl run node <node-name>"`))
	// Shared execution flags live on run only and are inherited by its subcommands.
	addRunFlags(cmd.PersistentFlags(), &opt)
	cmd.AddCommand(newRunTaskCommand(global, &opt), newRunNodeCommand(global, &opt), plannedRunCommand("preset", localized("Run the enabled tasks in a PI preset", "运行 PI preset 中启用的任务")))
	return cmd
}

func newResourceCommand(global *cliOptions) *cobra.Command {
	var resourceName string
	var inspect, nodes bool
	cmd := &cobra.Command{
		Use: "resource", Short: localized("Load and inspect PI resources", "加载并检查 PI 资源"),
		Long: localized(`resource inspect shows loaded metadata; resource nodes lists Pipeline nodes
available in the loaded resource. The shortcut flags -i and -n are equivalent
to the two subcommands.`, `resource inspect 显示已加载资源的元数据；resource nodes 列出资源中
可用的 Pipeline 节点。快捷选项 -i 和 -n 分别等价于这两个子命令。`),
		Example: `  maactl resource inspect -f D:\MaaMio
  maactl resource nodes -r base -f D:\MaaMio --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !inspect && !nodes {
				return c.Help()
			}
			if inspect && nodes {
				return fmt.Errorf("choose only one resource shortcut")
			}
			pi, err := loadPI(global.interfacePath)
			if err != nil {
				return err
			}
			var ctrl *controller
			if len(pi.Controller) == 1 {
				ctrl = &pi.Controller[0]
			} else {
				ctrl = &controller{}
			}
			res, err := pi.findResource(resourceName, ctrl)
			if err != nil {
				return err
			}
			return inspectResource(global, pi, res, nodes)
		},
	}
	add := func(use string, aliases []string, short, long string, nodes bool) {
		cmd.AddCommand(&cobra.Command{Use: use, Aliases: aliases, Short: short, Long: long, Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
			pi, err := loadPI(global.interfacePath)
			if err != nil {
				return err
			}
			var ctrl *controller
			if len(pi.Controller) == 1 {
				ctrl = &pi.Controller[0]
			} else {
				ctrl = &controller{}
			}
			res, err := pi.findResource(resourceName, ctrl)
			if err != nil {
				return err
			}
			return inspectResource(global, pi, res, nodes)
		}})
	}
	add("inspect", nil, localized("Show loaded resource metadata", "显示已加载资源的元数据"), localized("Load the selected resource and show its paths, hash, and node count.", "加载所选资源并显示其路径、hash 和节点数量。"), false)
	add("nodes", nil, localized("List loaded Pipeline nodes", "列出已加载的 Pipeline 节点"), localized("Load the selected resource and list all Pipeline nodes it provides.", "加载所选资源并列出其中所有 Pipeline 节点。"), true)
	cmd.AddCommand(plannedResourceCommand("hash", localized("Print or verify the loaded resource hash", "打印或校验已加载资源的 hash")))
	cmd.PersistentFlags().StringVarP(&resourceName, "resource", "r", "", localized("PI resource name (default: first resource)", "PI 资源名称（默认：第一个资源）"))
	cmd.Flags().BoolVarP(&inspect, "inspect", "i", false, localized(`shortcut for "maactl resource inspect"`, `等价于 "maactl resource inspect"`))
	cmd.Flags().BoolVarP(&nodes, "nodes", "n", false, localized(`shortcut for "maactl resource nodes"`, `等价于 "maactl resource nodes"`))
	return cmd
}

func plannedRunCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use + " <name>", Short: short, Long: localized("This command is part of the published CLI contract but is not implemented yet. It returns an error when invoked.", "该命令属于已公布的 CLI 契约，但尚未实现，调用时会返回错误。"), Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, _ []string) error { return fmt.Errorf("run %s is not implemented yet", use) }}
	markCommandPlanned(cmd)
	return cmd
}

func plannedResourceCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Long: localized("This command is part of the published CLI contract but is not implemented yet. It returns an error when invoked.", "该命令属于已公布的 CLI 契约，但尚未实现，调用时会返回错误。"), Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return fmt.Errorf("resource %s is not implemented yet", use) }}
	markCommandPlanned(cmd)
	return cmd
}

func inspectResource(global *cliOptions, pi *loadedPI, spec *resource, listNodes bool) error {
	libDir, err := resolveLibDir(global.libDir)
	if err != nil {
		return err
	}
	if err := maa.Init(maa.WithLibDir(libDir), maa.WithStdoutLevel(maa.LoggingLevelOff)); err != nil {
		return err
	}
	defer func() { _ = maa.Release() }()
	res := maa.NewResource()
	if res == nil {
		return fmt.Errorf("create Maa resource")
	}
	defer res.Destroy()
	paths := make([]string, 0, len(spec.Path))
	for _, path := range spec.Path {
		full := filepath.Join(pi.Dir, path)
		if !res.PostBundle(full).Wait().Success() {
			return fmt.Errorf("load resource %s", full)
		}
		paths = append(paths, full)
	}
	if listNodes {
		nodes, ok := res.GetNodeList()
		if !ok {
			return fmt.Errorf("read resource nodes")
		}
		if global.json {
			return printJSON(nodes)
		}
		for _, node := range nodes {
			fmt.Println(node)
		}
		return nil
	}
	hash, _ := res.GetHash()
	nodes, _ := res.GetNodeList()
	result := map[string]any{"name": spec.Name, "paths": paths, "hash": hash, "expected_hash": spec.Hash, "node_count": len(nodes)}
	if global.json {
		return printJSON(result)
	}
	fmt.Printf("resource: %s\nhash: %s\nnodes: %d\n", spec.Name, hash, len(nodes))
	return nil
}

func addRunFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.StringVarP(&opt.resource, "resource", "r", "", localized("PI resource name (default: first compatible resource)", "PI 资源名称（默认：第一个兼容资源）"))
	flags.StringVarP(&opt.controller, "controller", "c", "", localized("PI controller name (default: only controller)", "PI 控制器名称（默认：唯一的控制器）"))
	flags.StringVarP(&opt.adbAddress, "adb-address", "a", "", localized("ADB device serial/address (default: only detected device)", "ADB 设备序列号/地址（默认：唯一检测到的设备）"))
	flags.StringVarP(&opt.override, "override", "o", "", localized("final Pipeline override JSON", "最终 Pipeline override JSON"))
	flags.StringVarP(&opt.overrideFile, "override-file", "O", "", localized("file containing the final Pipeline override JSON", "包含最终 Pipeline override JSON 的文件"))
	flags.StringArrayVarP(&opt.optionValues, "option", "p", nil, localized("option value as name=<JSON>; repeatable", "option 值，格式 name=<JSON>；可重复"))
	flags.StringArrayVar(&opt.overlay, "overlay", nil, localized("additional resource root loaded after the selected resource; repeatable", "在所选资源之后加载的额外资源根目录；可重复"))
	flags.String("option-file", "", localized("JSON file of option values", "包含 option 值的 JSON 文件"))
	flags.Bool("dry-run", false, localized("resolve and display execution without connecting a controller", "仅解析并显示执行内容，不连接控制器"))
	flags.Bool("explain", false, localized("display resource and Pipeline override layers", "显示资源与 Pipeline override 各层内容"))
	flags.StringVar(&opt.events, "events", "focus", localized("event output: focus (PI text), all (all sink events), or off", "事件输出：focus（仅 PI 文本）、all（全部 sink 事件）或 off"))
	flags.DurationVar(&opt.stopAfter, "stop-after", 0, localized("stop a running task after this duration; for bounded runs and tests", "运行指定时长后停止任务；用于限时运行和测试"))
	flags.BoolVar(&opt.noAgent, "no-agent", false, localized("do not start the ProjectInterface agent", "不启动 ProjectInterface agent"))
	flags.VisitAll(func(f *pflag.Flag) {
		_ = flags.SetAnnotation(f.Name, sectionAnnotation, []string{sectionExecution})
	})
	markFlagsPlanned(flags, "option", "overlay", "option-file", "dry-run", "explain")
}

func newRunTaskCommand(global *cliOptions, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "task <task-name>",
		Short: localized("Run a task declared in ProjectInterface", "运行 ProjectInterface 中声明的 task"),
		Long: localized(`Looks up the task by name, resolves its entry node and Pipeline overrides,
then executes it on the selected controller.`, `按名称查找 task，解析其入口节点和 Pipeline override，
然后在所选控制器上执行。`),
		Example: `  maactl run task "自动挂机卖蛋" -f D:\MaaMio --stop-after 10s`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runTask(global, args[0], *opt)
		},
	}
}

func newRunNodeCommand(global *cliOptions, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "node <node-name>",
		Short: localized("Run a Pipeline node from a PI resource", "运行 PI 资源中的 Pipeline 节点"),
		Long: localized(`The node name is used directly as the task entry; the node is executed with
the selected resource and Pipeline overrides.`, `节点名称直接作为任务入口，
使用所选的资源和 Pipeline override 执行。`),
		Example: `  maactl run node "签到-开始签到" --adb-address 127.0.0.1:16384 --events all`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runNode(global, args[0], *opt)
		},
	}
}

func runTask(global *cliOptions, name string, opt runOptions) error {
	pi, err := loadPI(global.interfacePath)
	if err != nil {
		return err
	}
	ctrl, err := pi.findController(opt.controller)
	if err != nil {
		return err
	}
	res, err := pi.findResource(opt.resource, ctrl)
	if err != nil {
		return err
	}
	override, err := readOverride(opt)
	if err != nil {
		return err
	}
	t, err := pi.findTask(name, ctrl, res)
	if err != nil {
		return err
	}
	return execute(global, pi, ctrl, res, t.Entry, override, opt)
}

func runNode(global *cliOptions, name string, opt runOptions) error {
	pi, err := loadPI(global.interfacePath)
	if err != nil {
		return err
	}
	ctrl, err := pi.findController(opt.controller)
	if err != nil {
		return err
	}
	res, err := pi.findResource(opt.resource, ctrl)
	if err != nil {
		return err
	}
	override, err := readOverride(opt)
	if err != nil {
		return err
	}
	return execute(global, pi, ctrl, res, name, override, opt)
}

func readOverride(opt runOptions) (any, error) {
	if opt.override != "" && opt.overrideFile != "" {
		return nil, fmt.Errorf("--override/-o and --override-file/-O cannot be used together")
	}
	b := []byte(opt.override)
	if opt.overrideFile != "" {
		var err error
		b, err = os.ReadFile(opt.overrideFile)
		if err != nil {
			return nil, fmt.Errorf("read override: %w", err)
		}
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("parse pipeline override: %w", err)
	}
	return out, nil
}

func execute(global *cliOptions, pi *loadedPI, piCtrl *controller, piRes *resource, entry string, override any, opt runOptions) error {
	if opt.events != "focus" && opt.events != "all" && opt.events != "off" {
		return fmt.Errorf("invalid --events value %q; use focus, all, or off", opt.events)
	}
	libDir, err := resolveLibDir(global.libDir)
	if err != nil {
		return err
	}
	if err := maa.Init(maa.WithLibDir(libDir), maa.WithStdoutLevel(maa.LoggingLevelOff)); err != nil {
		return fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err)
	}
	defer func() { _ = maa.Release() }()
	var agent *exec.Cmd
	if !opt.noAgent && pi.Agent != nil && pi.Agent.ChildExec != "" {
		execPath := pi.Agent.ChildExec
		if !filepath.IsAbs(execPath) {
			execPath = filepath.Join(pi.Dir, execPath)
		}
		agent = exec.Command(execPath, pi.Agent.ChildArgs...)
		agent.Dir = pi.Dir
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
		full := filepath.Join(pi.Dir, p)
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
	sink := &consoleTaskerSink{json: global.json, mode: opt.events}
	tasker.AddSink(sink)
	// MaaFramework v5.13 emits Pipeline node notifications on the context sink.
	// The ordinary tasker sink only receives Resource/Controller/Tasker events.
	tasker.AddContextSink(&consoleContextSink{sink: sink})

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

func createController(spec *controller, opt runOptions) (*maa.Controller, error) {
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
			return nil, fmt.Errorf("%d ADB devices found; specify --adb-address/-a: %s", len(devices), join(names))
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

// consoleTaskerSink prints every node event emitted through MaaFramework's tasker sink.
type consoleTaskerSink struct {
	json bool
	mode string
	mu   sync.Mutex
}

func (s *consoleTaskerSink) output(kind string, event maa.EventStatus, detail any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(detail)
	var values map[string]any
	_ = json.Unmarshal(b, &values)
	message := kind + "." + eventName(event)
	focus := renderFocus(message, values)
	if s.mode == "off" || (s.mode == "focus" && focus == "") {
		return
	}
	if s.json {
		out := map[string]any{"focus": focus}
		if s.mode == "all" {
			out["event"] = message
			out["status"] = event
			out["detail"] = values
		}
		b, _ = json.Marshal(out)
		fmt.Println(string(b))
		return
	}
	if s.mode == "focus" {
		fmt.Printf("%s\n", focus)
		return
	}
	if focus != "" {
		fmt.Printf("%s\n", focus)
	}
	fmt.Printf("%s %s\n", message, b)
}

func eventName(event maa.EventStatus) string {
	switch event {
	case maa.EventStatusStarting:
		return "Starting"
	case maa.EventStatusSucceeded:
		return "Succeeded"
	case maa.EventStatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}
func (s *consoleTaskerSink) OnResourceLoading(_ *maa.Tasker, e maa.EventStatus, d maa.ResourceLoadingDetail) {
	s.output("Resource.Loading", e, d)
}
func (s *consoleTaskerSink) OnControllerAction(_ *maa.Tasker, e maa.EventStatus, d maa.ControllerActionDetail) {
	s.output("Controller.Action", e, d)
}
func (s *consoleTaskerSink) OnTaskerTask(_ *maa.Tasker, e maa.EventStatus, d maa.TaskerTaskDetail) {
	s.output("Tasker.Task", e, d)
}
func (s *consoleTaskerSink) OnNodePipelineNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodePipelineNodeDetail) {
	s.output("Node.PipelineNode", e, d)
}
func (s *consoleTaskerSink) OnNodeRecognitionNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodeRecognitionNodeDetail) {
	s.output("Node.RecognitionNode", e, d)
}
func (s *consoleTaskerSink) OnNodeActionNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodeActionNodeDetail) {
	s.output("Node.ActionNode", e, d)
}
func (s *consoleTaskerSink) OnTaskNextList(_ *maa.Tasker, e maa.EventStatus, d maa.NodeNextListDetail) {
	s.output("Node.NextList", e, d)
}
func (s *consoleTaskerSink) OnTaskRecognition(_ *maa.Tasker, e maa.EventStatus, d maa.NodeRecognitionDetail) {
	s.output("Node.Recognition", e, d)
}
func (s *consoleTaskerSink) OnTaskAction(_ *maa.Tasker, e maa.EventStatus, d maa.NodeActionDetail) {
	s.output("Node.Action", e, d)
}
func (s *consoleTaskerSink) OnUnknownEvent(_ *maa.Tasker, msg, details string) {
	if s.mode != "all" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Printf("%s %s\n", msg, details)
}

// consoleContextSink receives node-level callbacks, including the focus field.
// It forwards their original event category and status to the same formatter.
type consoleContextSink struct{ sink *consoleTaskerSink }

func (s *consoleContextSink) OnResourceLoading(_ *maa.Context, e maa.EventStatus, d maa.ResourceLoadingDetail) {
	s.sink.output("Resource.Loading", e, d)
}
func (s *consoleContextSink) OnControllerAction(_ *maa.Context, e maa.EventStatus, d maa.ControllerActionDetail) {
	s.sink.output("Controller.Action", e, d)
}
func (s *consoleContextSink) OnTaskerTask(_ *maa.Context, e maa.EventStatus, d maa.TaskerTaskDetail) {
	s.sink.output("Tasker.Task", e, d)
}
func (s *consoleContextSink) OnNodePipelineNode(_ *maa.Context, e maa.EventStatus, d maa.NodePipelineNodeDetail) {
	s.sink.output("Node.PipelineNode", e, d)
}
func (s *consoleContextSink) OnNodeRecognitionNode(_ *maa.Context, e maa.EventStatus, d maa.NodeRecognitionNodeDetail) {
	s.sink.output("Node.RecognitionNode", e, d)
}
func (s *consoleContextSink) OnNodeActionNode(_ *maa.Context, e maa.EventStatus, d maa.NodeActionNodeDetail) {
	s.sink.output("Node.ActionNode", e, d)
}
func (s *consoleContextSink) OnTaskNextList(_ *maa.Context, e maa.EventStatus, d maa.NodeNextListDetail) {
	s.sink.output("Node.NextList", e, d)
}
func (s *consoleContextSink) OnTaskRecognition(_ *maa.Context, e maa.EventStatus, d maa.NodeRecognitionDetail) {
	s.sink.output("Node.Recognition", e, d)
}
func (s *consoleContextSink) OnTaskAction(_ *maa.Context, e maa.EventStatus, d maa.NodeActionDetail) {
	s.sink.output("Node.Action", e, d)
}
func (s *consoleContextSink) OnUnknownEvent(_ *maa.Context, msg, details string) {
	if s.sink.mode != "all" {
		return
	}
	s.sink.mu.Lock()
	defer s.sink.mu.Unlock()
	fmt.Printf("%s %s\n", msg, details)
}
