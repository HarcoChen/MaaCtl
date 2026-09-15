package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maactl/internal/clientconfig"
	"maactl/internal/help"
	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// clientVersion is the running maactl version, reported to agents through
// PI_CLIENT_VERSION.
var clientVersion = "dev"

// runOptions holds every execution flag. They are defined once on `run` and
// inherited by its subcommands, so `run task x -r base` and `run -r base task x`
// behave identically.
type runOptions struct {
	resource   string
	controller string
	adbAddress string
	adbName    string
	adbPath    string

	win32Handle    string
	win32Class     string
	win32Window    string
	win32Screencap string
	win32Mouse     string
	win32Keyboard  string
	gamepadType    string

	macosWindow    string
	macosWindowID  string
	macosScreencap string
	macosInput     string

	playcoverAddress string
	playcoverUUID    string

	linuxSocket string
	linuxVK     bool

	optionValues []string
	optionFile   string
	preset       string
	override     string
	overrideFile string
	overlay      []string
	paths        []string

	events       string
	focusDisplay string
	timeout      time.Duration
	stopAfter    time.Duration

	dryRun      bool
	explain     bool
	requireHash bool

	noAgent         bool
	agentLog        string
	continueOnError bool
}

// newRunCommand builds the `run` group.
func newRunCommand(global *GlobalOptions) *cobra.Command {
	var taskName, nodeName string
	var opt runOptions
	cmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   i18n.Text("Run tasks, presets, or nodes", "运行 task、preset 或节点"),
		Long: i18n.Text(`Runs a task declared in the ProjectInterface, a Pipeline node, or every enabled
task of a preset.

The shortcuts equal the subcommands:
  maactl run -t <name>  ==  maactl run task <name>
  maactl run -n <name>  ==  maactl run node <name>

Use "run preset <name>" for a whole preset; --preset applies one preset's values
to a single task.`, `运行 ProjectInterface 中声明的 task、Pipeline 节点，或 preset 中所有启用的 task。

快捷选项与子命令等价：
  maactl run -t <name>  ==  maactl run task <name>
  maactl run -n <name>  ==  maactl run node <name>

运行整个 preset 用 "run preset <name>"；--preset 只把某个 preset 的取值应用到
单个 task。`),
		Example: `  maactl run -t 签到 -if D:\MaaMio -sa 30s
  maactl run -t 签到 -dr -x
  maactl run preset ALL-IN -e all
  maactl run -n "签到-开始签到"`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			switch {
			case taskName != "" && nodeName != "":
				return exitErrorf(ExitUsage, "choose only one of --task/-t or --node/-n")
			case taskName != "":
				return runTask(global, taskName, opt)
			case nodeName != "":
				return runNode(global, nodeName, opt)
			default:
				return c.Help()
			}
		},
	}
	cmd.Flags().StringVarP(&taskName, "task", "t", "", i18n.Text(`same as "run task <name>"`, `等价于 "run task <name>"`))
	cmd.Flags().StringVarP(&nodeName, "node", "n", "", i18n.Text(`same as "run node <name>"`, `等价于 "run node <name>"`))
	help.MarkFlagsSection(cmd.Flags(), help.SectionShortcut, "task", "node")
	addRunFlags(cmd.PersistentFlags(), &opt)
	cmd.AddCommand(
		newRunTaskCommand(global, &opt),
		newRunPresetCommand(global, &opt),
		newRunNodeCommand(global, &opt),
	)
	return cmd
}

func newRunTaskCommand(global *GlobalOptions, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "task <task-name>",
		Aliases: []string{"t"},
		Short:   i18n.Text("Run a declared task", "运行声明的 task"),
		Long: i18n.Text(`Looks the task up by name, label, or case-insensitive name, resolves its options
and Pipeline overrides, then executes its entry node.`, `按名称、显示名称或大小写不敏感的名称查找 task，解析配置项与 Pipeline 覆盖，
然后执行入口节点。`),
		Example: `  maactl run task "签到" -if D:\MaaMio -e all`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runTask(global, args[0], *opt)
		},
	}
}

func newRunPresetCommand(global *GlobalOptions, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "preset <preset-name>",
		Aliases: []string{"p"},
		Short:   i18n.Text("Run every enabled task of a preset", "运行 preset 中所有启用的 task"),
		Long: i18n.Text(`Runs the preset's tasks in declaration order, stopping at the first failure unless
-k/--continue-on-error is set. Command-line option values override the preset's,
but only for options the task actually references.`, `按声明顺序运行 preset 中的 task；除非指定 -k/--continue-on-error，否则遇到第一个
失败即停止。命令行配置项只覆盖该 task 真正引用的同名配置项。`),
		Example: `  maactl run preset 刷日常 -e off`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runPreset(global, args[0], *opt)
		},
	}
}

func newRunNodeCommand(global *GlobalOptions, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "node <node-name>",
		Aliases: []string{"n"},
		Short:   i18n.Text("Run a Pipeline node", "运行 Pipeline 节点"),
		Long: i18n.Text(`Uses the node name as the task entry. The resource comes from the selected PI
resource, or from -pa/--path.`, `节点名称直接作为任务入口。资源来自所选 PI 资源，或用 -pa/--path 指定。`),
		Example: `  maactl run node "签到-开始签到" -e all
  maactl run node Login -pa D:\pkg\resource`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runNode(global, args[0], *opt)
		},
	}
}

// addRunFlags declares the shared execution flags, one function per help
// section; each group's help section is annotated at the end of this function.
func addRunFlags(flags *pflag.FlagSet, opt *runOptions) {
	addTargetFlags(flags, opt)
	addOptionFlags(flags, opt)
	addResourceFlags(flags, opt)
	addOutputFlags(flags, opt)
	addControlFlags(flags, opt)

	help.MarkFlagsSection(flags, help.SectionTarget,
		"resource", "controller", "adb-address", "name", "adb-path",
		"win32-handle", "win32-class", "win32-window", "win32-screencap", "win32-mouse", "win32-keyboard",
		"gamepad-type",
		"macos-window", "macos-window-id", "macos-screencap", "macos-input",
		"playcover-address", "playcover-uuid",
		"linux-socket", "linux-vk")
	help.MarkFlagsSection(flags, help.SectionOptions,
		"option", "option-file", "preset", "override", "override-file")
	help.MarkFlagsSection(flags, help.SectionResources,
		"path", "overlay", "require-resource-hash")
	help.MarkFlagsSection(flags, help.SectionOutput,
		"events", "focus-display")
	help.MarkFlagsSection(flags, help.SectionControl,
		"dry-run", "explain", "timeout", "stop-after", "no-agent", "agent-log", "continue-on-error")
}

// addTargetFlags declares the flags that select the controller target.
func addTargetFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.StringVarP(&opt.resource, "resource", "r", "", i18n.Text("PI resource (default: config, then the first compatible)", "PI 资源（默认：配置文件，其次第一个兼容资源）"))
	flags.StringVarP(&opt.controller, "controller", "c", "", i18n.Text("PI controller (default: config, then the only one)", "PI 控制器（默认：配置文件，其次唯一的控制器）"))
	flags.StringVarP(&opt.adbAddress, "adb-address", "a", "", i18n.Text("ADB address (default: config, then the only device)", "ADB 地址（默认：配置文件，其次唯一设备）"))
	flags.StringVar(&opt.adbName, "name", "", i18n.Text("ADB device name reported by MaaToolkit", "MaaToolkit 报告的 ADB 设备名称"))
	flags.StringVar(&opt.adbPath, "adb-path", "", i18n.Text("adb executable used to build the controller", "用于创建控制器的 adb 可执行文件"))
	flags.StringVar(&opt.win32Handle, "win32-handle", "", i18n.Text("Win32 window handle, decimal or 0x hex", "Win32 窗口句柄，十进制或 0x 十六进制"))
	flags.StringVar(&opt.win32Class, "win32-class", "", i18n.Text("Win32 window class regex (default: PI win32.class_regex)", "Win32 窗口类名正则（默认：PI win32.class_regex）"))
	flags.StringVar(&opt.win32Window, "win32-window", "", i18n.Text("Win32 window title regex (default: PI win32.window_regex)", "Win32 窗口标题正则（默认：PI win32.window_regex）"))
	flags.StringVar(&opt.win32Screencap, "win32-screencap", "", i18n.Text("Win32 screencap method (default: PI config, then all methods)", "Win32 截图方式（默认：PI 配置，其次全部方式）"))
	flags.StringVar(&opt.win32Mouse, "win32-mouse", "", i18n.Text("Win32 mouse method (default: PI config, then Seize)", "Win32 鼠标方式（默认：PI 配置，其次 Seize）"))
	flags.StringVar(&opt.win32Keyboard, "win32-keyboard", "", i18n.Text("Win32 keyboard method (default: PI config, then Seize)", "Win32 键盘方式（默认：PI 配置，其次 Seize）"))
	flags.StringVar(&opt.gamepadType, "gamepad-type", "", i18n.Text("Gamepad type: Xbox360 or DualShock4", "Gamepad 类型：Xbox360 或 DualShock4"))
	flags.StringVar(&opt.macosWindow, "macos-window", "", i18n.Text("macOS window title regex (default: PI macos.title_regex)", "macOS 窗口标题正则（默认：PI macos.title_regex）"))
	flags.StringVar(&opt.macosWindowID, "macos-window-id", "", i18n.Text("macOS window id from \"maactl device window\"", "macOS 窗口 id，取自 \"maactl device window\""))
	flags.StringVar(&opt.macosScreencap, "macos-screencap", "", i18n.Text("macOS screencap method (default: PI config, then ScreenCaptureKit)", "macOS 截图方式（默认：PI 配置，其次 ScreenCaptureKit）"))
	flags.StringVar(&opt.macosInput, "macos-input", "", i18n.Text("macOS input method: GlobalEvent or PostToPid (default: PI config, then GlobalEvent)", "macOS 输入方式：GlobalEvent 或 PostToPid（默认：PI 配置，其次 GlobalEvent）"))
	flags.StringVar(&opt.playcoverAddress, "playcover-address", "", i18n.Text("PlayTools service address (default: config playcover.address)", "PlayTools 服务地址（默认：配置 playcover.address）"))
	flags.StringVar(&opt.playcoverUUID, "playcover-uuid", "", i18n.Text("PlayCover bundle identifier (default: PI playcover.uuid, then maa.playcover)", "PlayCover 应用标识（默认：PI playcover.uuid，其次 maa.playcover）"))
	flags.StringVar(&opt.linuxSocket, "linux-socket", "", i18n.Text("Wayland socket of the compositor (default: config, then $WAYLAND_DISPLAY)", "合成器的 Wayland socket（默认：配置，其次 $WAYLAND_DISPLAY）"))
	flags.BoolVar(&opt.linuxVK, "linux-vk", false, i18n.Text("treat Linux key codes as Win32 virtual-key codes", "把 Linux 按键视为 Win32 Virtual-Key 键码"))
}

// addOptionFlags declares the flags that supply option and override values.
func addOptionFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.StringArrayVar(&opt.optionValues, "option", nil, i18n.Text("name=value, name=a,b, or name.field=value; repeatable", "name=value、name=a,b 或 name.field=value；可重复"))
	flags.StringVar(&opt.optionFile, "option-file", "", i18n.Text("JSON file of option values", "包含配置项取值的 JSON 文件"))
	flags.StringVarP(&opt.preset, "preset", "p", "", i18n.Text("apply this preset's values to the task", "把该 preset 的取值应用到 task"))
	flags.StringVarP(&opt.override, "override", "o", "", i18n.Text("final Pipeline override JSON", "最终 Pipeline override JSON"))
	flags.StringVar(&opt.overrideFile, "override-file", "", i18n.Text("file with the final Pipeline override JSON", "包含最终 Pipeline override JSON 的文件"))
}

// addResourceFlags declares the flags that describe the loaded resources.
func addResourceFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.StringArrayVar(&opt.overlay, "overlay", nil, i18n.Text("resource root loaded last; repeatable", "最后加载的资源根目录；可重复"))
	flags.StringArrayVar(&opt.paths, "path", nil, i18n.Text("resource root for `run node`, replacing the PI resource", "`run node` 的资源根目录（替代 PI 资源）"))
}

// addOutputFlags declares the flags that select what run reports.
func addOutputFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.StringVarP(&opt.events, "events", "e", "focus", i18n.Text("focus (PI text), all (every sink event), or off", "focus（PI 文本）、all（全部 sink 事件）或 off"))
	flags.StringVar(&opt.focusDisplay, "focus-display", "log", i18n.Text("channels: log,toast,notification,dialog,modal or all", "渠道：log,toast,notification,dialog,modal 或 all"))
}

// addControlFlags declares the flags that control how the run itself behaves.
func addControlFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.DurationVar(&opt.timeout, "timeout", 0, i18n.Text("stop and fail after this duration", "超过该时长后停止并失败"))
	flags.DurationVar(&opt.stopAfter, "stop-after", 0, i18n.Text("stop after this duration and succeed (debug aid)", "运行该时长后停止并视为成功（调试用）"))
	flags.BoolVar(&opt.dryRun, "dry-run", false, i18n.Text("resolve and print the run without connecting a controller", "只解析并打印本次运行，不连接控制器"))
	flags.BoolVarP(&opt.explain, "explain", "x", false, i18n.Text("print selections and every override layer", "打印选择结果与每一层 override"))
	// Shown under Resources, declared here: the declaration order is part of
	// the command's contract.
	flags.BoolVar(&opt.requireHash, "require-resource-hash", false, i18n.Text("fail when resource.hash does not match", "resource.hash 不匹配时失败"))
	flags.BoolVar(&opt.noAgent, "no-agent", false, i18n.Text("do not start declared agents", "不启动声明的 agent"))
	flags.StringVar(&opt.agentLog, "agent-log", "term", i18n.Text("term, off, or a directory for one log per agent", "term、off 或目录（每个 agent 一个日志）"))
	flags.BoolVarP(&opt.continueOnError, "continue-on-error", "k", false, i18n.Text("keep running preset tasks after a failure", "preset 中某个 task 失败后继续"))
}

// preparedRun is everything needed to execute one entry: the selected entities,
// the resolved options, and the final Pipeline override.
type preparedRun struct {
	global     *GlobalOptions
	project    *pi.Loaded
	config     *clientconfig.Config
	configPath string
	controller *pi.Controller
	resource   *pi.Resource
	task       *pi.Task
	entry      string
	resolution *pi.Resolution
	override   pi.Pipeline
	opt        runOptions
}

// loadProjectAndConfig loads the PI and its client configuration once.
func loadProjectAndConfig(global *GlobalOptions) (*pi.Loaded, *clientconfig.Config, string, error) {
	project, err := global.LoadProject()
	if err != nil {
		return nil, nil, "", err
	}
	config, configPath, err := global.LoadConfig(project)
	if err != nil {
		return nil, nil, "", err
	}
	return project, config, configPath, nil
}

// selectController resolves the controller: flag, then config, then the only
// declared controller.
func (p *preparedRun) selectController() error {
	name := strings.TrimSpace(p.opt.controller)
	source := "flag"
	if name == "" {
		if fromConfig := p.config.DefaultController(); fromConfig != "" {
			name = fromConfig
			source = "config"
		}
	}
	controller, err := p.project.FindController(name)
	if err != nil {
		return withExitCode(ExitUsage, err)
	}
	if name == "" {
		source = "the only controller"
	}
	p.controller = controller
	fmt.Fprintf(os.Stderr, "controller: %s (%s)\n", controller.Name, source)
	return nil
}

// selectResource resolves the resource: flag, then config, then the first
// compatible one.
func (p *preparedRun) selectResource() error {
	name := strings.TrimSpace(p.opt.resource)
	source := "flag"
	if name == "" {
		if fromConfig := p.config.DefaultResource(); fromConfig != "" {
			name = fromConfig
			source = "config"
		}
	}
	resource, err := p.project.FindResource(name, p.controller)
	if err != nil {
		return withExitCode(ExitUsage, err)
	}
	if name == "" {
		source = "first compatible"
	}
	p.resource = resource
	fmt.Fprintf(os.Stderr, "resource: %s (%s)\n", resource.Name, source)
	return nil
}

// parseCLIOptions turns repeated --option flags into the protocol's value map.
func parseCLIOptions(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	values := map[string]any{}
	for _, item := range raw {
		name, value, err := parseOptionFlag(item)
		if err != nil {
			return nil, err
		}
		// Repeated `--option name.field=value` flags build one field map.
		if fields, ok := value.(map[string]any); ok {
			if existing, ok := values[name].(map[string]any); ok {
				for field, fieldValue := range fields {
					existing[field] = fieldValue
				}
				continue
			}
		}
		values[name] = value
	}
	return values, nil
}

// parseOptionFlag parses one --option argument. The accepted forms are
// `name=value` (select/switch), `name=a,b` / `name=["a","b"]` (checkbox), and
// `name.field=value` (input/hotkey).
func parseOptionFlag(raw string) (string, any, error) {
	index := strings.Index(raw, "=")
	if index <= 0 {
		return "", nil, exitErrorf(ExitUsage, "invalid --option %q: expected name=value", raw)
	}
	name := strings.TrimSpace(raw[:index])
	text := strings.TrimSpace(raw[index+1:])
	if name == "" {
		return "", nil, exitErrorf(ExitUsage, "invalid --option %q: the option name is empty", raw)
	}
	if dot := strings.LastIndex(name, "."); dot > 0 {
		return name[:dot], map[string]any{name[dot+1:]: text}, nil
	}
	if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return "", nil, exitErrorf(ExitUsage, "invalid --option %s=%s: %v", name, text, err)
		}
		return name, decoded, nil
	}
	return name, text, nil
}

// parseOptionFile reads an --option-file document: the same object shape as a
// preset's option map.
func parseOptionFile(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("read option file: %w", err))
	}
	var values map[string]any
	if err := json.Unmarshal(pi.StripJSONC(b), &values); err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("parse option file %s: %w", path, err))
	}
	return values, nil
}

// readOverride reads the final Pipeline override from --override or
// --override-file.
func readOverride(opt runOptions) (pi.Pipeline, error) {
	if opt.override != "" && opt.overrideFile != "" {
		return nil, exitErrorf(ExitUsage, "--override/-o and --override-file cannot be used together")
	}
	var raw []byte
	switch {
	case opt.override != "":
		raw = []byte(opt.override)
	case opt.overrideFile != "":
		b, err := os.ReadFile(opt.overrideFile)
		if err != nil {
			return nil, withExitCode(ExitUsage, fmt.Errorf("read override file: %w", err))
		}
		raw = b
	default:
		return nil, nil
	}
	var object map[string]any
	if err := json.Unmarshal(pi.StripJSONC(raw), &object); err != nil {
		return nil, withExitCode(ExitUsage, fmt.Errorf("parse Pipeline override: %w", err))
	}
	return object, nil
}

// resolveOptions builds the option request for one task and resolves it.
func (p *preparedRun) resolveOptions(cliValues, optionFile map[string]any, presetEntry *pi.PresetTask) error {
	request := p.optionRequest(cliValues, optionFile)
	if presetEntry != nil {
		request.PresetOptions = presetEntry.Option
	}
	extra, err := readOverride(p.opt)
	if err != nil {
		return err
	}
	request.ExtraOverride = extra
	resolution, err := p.project.Resolve(request)
	if err != nil {
		return withExitCode(ExitUsage, err)
	}
	p.resolution = resolution
	p.override = resolution.Override
	return nil
}

// prepareTaskRun resolves a task (or one preset entry) into a runnable plan.
func prepareTaskRun(global *GlobalOptions, project *pi.Loaded, config *clientconfig.Config, configPath string, opt runOptions, taskName string, presetEntry *pi.PresetTask) (*preparedRun, error) {
	p := &preparedRun{global: global, project: project, config: config, configPath: configPath, opt: opt}
	if err := p.selectController(); err != nil {
		return nil, err
	}
	if err := p.selectResource(); err != nil {
		return nil, err
	}
	task := project.LookupTask(taskName, global.Language())
	if task == nil {
		return nil, exitErrorf(ExitUsage, "task %q not found; available: %s", taskName, pi.JoinTaskNames(project.Task))
	}
	p.task = task
	p.entry = task.Entry
	if reasons := project.TaskReasons(task, p.controller, p.resource); len(reasons) > 0 {
		return nil, exitErrorf(ExitUsage, "task %q is unavailable here: %s", task.Name, strings.Join(reasons, "; "))
	}
	cliValues, err := parseCLIOptions(opt.optionValues)
	if err != nil {
		return nil, err
	}
	optionFile, err := parseOptionFile(opt.optionFile)
	if err != nil {
		return nil, err
	}
	if err := p.resolveOptions(cliValues, optionFile, presetEntry); err != nil {
		return nil, err
	}
	return p, nil
}

// optionRequest builds the Resolve request for this run. Passing nil cliValues
// and optionFile yields the request used for standalone option lookups such as
// pretask arguments.
func (p *preparedRun) optionRequest(cliValues, optionFile map[string]any) pi.Request {
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
		OptionFile:     optionFile,
		CLI:            cliValues,
	}
}

// prepareNodeRun resolves a bare Pipeline node. No task and no preset applies.
func prepareNodeRun(global *GlobalOptions, project *pi.Loaded, config *clientconfig.Config, configPath string, opt runOptions, nodeName string) (*preparedRun, error) {
	if opt.preset != "" {
		return nil, exitErrorf(ExitUsage, "--preset does not apply to `run node`")
	}
	p := &preparedRun{global: global, project: project, config: config, configPath: configPath, opt: opt, entry: nodeName}
	if err := p.selectController(); err != nil {
		return nil, err
	}
	if len(opt.paths) > 0 {
		if opt.resource != "" {
			return nil, exitErrorf(ExitUsage, "--resource and --path cannot be used together")
		}
		absolute := make([]string, 0, len(opt.paths))
		for _, path := range opt.paths {
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil, withExitCode(ExitUsage, err)
			}
			absolute = append(absolute, abs)
		}
		p.resource = &pi.Resource{Name: "ad-hoc", Path: absolute}
		fmt.Fprintf(os.Stderr, "resource: ad-hoc paths\n")
	} else if err := p.selectResource(); err != nil {
		return nil, err
	}
	cliValues, err := parseCLIOptions(opt.optionValues)
	if err != nil {
		return nil, err
	}
	optionFile, err := parseOptionFile(opt.optionFile)
	if err != nil {
		return nil, err
	}
	if err := p.resolveOptions(cliValues, optionFile, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// runTask executes a single PI task.
func runTask(global *GlobalOptions, name string, opt runOptions) error {
	project, config, configPath, err := loadProjectAndConfig(global)
	if err != nil {
		return err
	}
	var presetEntry *pi.PresetTask
	if opt.preset != "" {
		preset, err := project.FindPreset(opt.preset, global.Language())
		if err != nil {
			return withExitCode(ExitUsage, err)
		}
		// The selected task has to exist for a preset entry to match it, so the
		// resolved entry is compared under a nil guard: otherwise two names the
		// project does not know would both resolve to nil and match each other,
		// swallowing the "does not include task" warning below.
		target := project.LookupTask(name, global.Language())
		for i := range preset.Task {
			if target != nil && project.LookupTask(preset.Task[i].Name, global.Language()) == target {
				presetEntry = &preset.Task[i]
				break
			}
		}
		if presetEntry == nil {
			fmt.Fprintf(os.Stderr, "warning: preset %q does not include task %q; no preset values applied\n", preset.Name, name)
		}
	}
	prepared, err := prepareTaskRun(global, project, config, configPath, opt, name, presetEntry)
	if err != nil {
		return err
	}
	return executeRun(prepared)
}

// runPreset runs every enabled task of a preset, in order.
func runPreset(global *GlobalOptions, name string, opt runOptions) error {
	if opt.preset != "" {
		return exitErrorf(ExitUsage, "--preset does not apply to `run preset`; the positional name already selects the preset")
	}
	project, config, configPath, err := loadProjectAndConfig(global)
	if err != nil {
		return err
	}
	preset, err := project.FindPreset(name, global.Language())
	if err != nil {
		return withExitCode(ExitUsage, err)
	}
	var failures []string
	for i := range preset.Task {
		entry := &preset.Task[i]
		if !entry.EnabledOrDefault() {
			continue
		}
		fmt.Fprintf(os.Stderr, "── preset %s: task %s ──\n", preset.Name, entry.Name)
		prepared, err := prepareTaskRun(global, project, config, configPath, opt, entry.Name, entry)
		if err != nil {
			if !opt.continueOnError {
				return err
			}
			fmt.Fprintf(os.Stderr, "task %s failed: %v\n", entry.Name, err)
			failures = append(failures, entry.Name)
			continue
		}
		if err := executeRun(prepared); err != nil {
			if !opt.continueOnError {
				return err
			}
			fmt.Fprintf(os.Stderr, "task %s failed: %v\n", entry.Name, err)
			failures = append(failures, entry.Name)
		}
	}
	if len(failures) > 0 {
		return exitErrorf(ExitTask, "preset %q finished with failures: %s", preset.Name, pi.Join(failures))
	}
	return nil
}

// runNode executes a Pipeline node directly.
func runNode(global *GlobalOptions, name string, opt runOptions) error {
	project, config, configPath, err := loadProjectAndConfig(global)
	if err != nil {
		return err
	}
	prepared, err := prepareNodeRun(global, project, config, configPath, opt, name)
	if err != nil {
		return err
	}
	return executeRun(prepared)
}

// executeRun prints the plan and, unless --dry-run, executes it.
func executeRun(p *preparedRun) error {
	if p.opt.explain || p.opt.dryRun || p.global.Verbose {
		if err := printExplain(p); err != nil {
			return err
		}
	}
	if p.opt.dryRun {
		if p.global.JSON {
			return output.JSON(os.Stdout, dryRunSummary(p))
		}
		fmt.Fprintln(os.Stdout, "dry run: nothing was executed")
		return nil
	}
	return p.execute()
}

// printExplain writes the selections and every override layer. Secrets are
// masked; the effective override printed here is what will be sent.
func printExplain(p *preparedRun) error {
	if p.global.JSON {
		return nil // included in the run summary instead
	}
	out := os.Stderr
	if p.configPath != "" {
		fmt.Fprintf(out, "config: %s\n", p.configPath)
	}
	fmt.Fprintf(out, "entry: %s\n", p.entry)
	for _, selection := range p.resolution.Selections {
		fmt.Fprintf(out, "  option %s = %v (%s, %s)", selection.Name, selection.MaskedValue(), selection.Layer, selection.Source)
		if selection.Parent != "" {
			fmt.Fprintf(out, "  parent=%s", selection.Parent)
		}
		fmt.Fprintln(out)
	}
	for _, skipped := range p.resolution.Skipped {
		fmt.Fprintf(out, "  option %s skipped: %s\n", skipped.Name, skipped.Reason)
	}
	for _, contribution := range p.resolution.Contributions {
		b, _ := json.Marshal(contribution.MaskedOverride())
		fmt.Fprintf(out, "  override [%s] %s\n", contribution.Label, b)
	}
	final, err := json.MarshalIndent(p.resolution.MaskedOverride(), "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "effective override:\n%s\n", final)
	return nil
}

// selectionJSON describes one resolved option in run output.
type selectionJSON struct {
	Name   string `json:"name"`
	Layer  string `json:"layer"`
	Source string `json:"source"`
	Parent string `json:"parent,omitempty"`
	Value  any    `json:"value"`
}

// runSummary is the JSON result of a run or a dry run.
type runSummary struct {
	Interface         string          `json:"interface"`
	Resource          string          `json:"resource"`
	ResourcePaths     []string        `json:"resource_paths,omitempty"`
	Controller        string          `json:"controller"`
	Task              string          `json:"task,omitempty"`
	Entry             string          `json:"entry"`
	Status            string          `json:"status"`
	DryRun            bool            `json:"dry_run,omitempty"`
	ElapsedMS         int64           `json:"elapsed_ms"`
	Selections        []selectionJSON `json:"selections,omitempty"`
	EffectiveOverride any             `json:"effective_override,omitempty"`
}

func buildSummary(p *preparedRun, status string, elapsed time.Duration) runSummary {
	summary := runSummary{
		Interface:         p.project.Path,
		Resource:          p.resource.Name,
		Controller:        p.controller.Name,
		Entry:             p.entry,
		Status:            status,
		ElapsedMS:         elapsed.Milliseconds(),
		EffectiveOverride: p.resolution.MaskedOverride(),
		ResourcePaths:     p.resourcePaths(),
	}
	if p.task != nil {
		summary.Task = p.task.Name
	}
	for _, selection := range p.resolution.Selections {
		summary.Selections = append(summary.Selections, selectionJSON{
			Name:   selection.Name,
			Layer:  string(selection.Layer),
			Source: string(selection.Source),
			Parent: selection.Parent,
			Value:  selection.MaskedValue(),
		})
	}
	return summary
}

func dryRunSummary(p *preparedRun) runSummary {
	summary := buildSummary(p, "DryRun", 0)
	summary.DryRun = true
	return summary
}

// resourcePaths returns the absolute resource roots for the run. PI resource
// paths resolve against the ProjectInterface directory; --path and --overlay are
// command-line paths, so they resolve against the working directory.
func (p *preparedRun) resourcePaths() []string {
	paths := make([]string, 0, len(p.resource.Path)+len(p.opt.overlay))
	for _, path := range p.resource.Path {
		paths = append(paths, resolveAgainst(p.project.Dir, path))
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = p.project.Dir
	}
	for _, overlay := range p.opt.overlay {
		paths = append(paths, resolveAgainst(cwd, overlay))
	}
	return paths
}

// resolveAgainst resolves a possibly relative path against base.
func resolveAgainst(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(path, `\`, "/")))
}
