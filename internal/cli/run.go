package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"maactl/internal/help"
	"maactl/internal/i18n"
	"maactl/internal/pi"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type runOptions struct {
	resource, controller, adbAddress string
	adbName                          string
	win32Handle, win32Class          string
	win32Window                      string
	win32Screencap, win32Mouse       string
	win32Keyboard                    string
	gamepadType                      string
	override, overrideFile           string
	optionValues, overlay            []string
	events                           string
	stopAfter                        time.Duration
	stopTimeout                      time.Duration
	noAgent                          bool
	agentLog                         string
}

func newRunCommand(global *Options) *cobra.Command {
	var taskName, nodeName string
	var opt runOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: i18n.Text("Run PI tasks or Pipeline nodes", "运行 PI task 或 Pipeline 节点"),
		Long: i18n.Text(`Run a task declared in ProjectInterface, or run a Pipeline node directly.

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
	cmd.Flags().StringVarP(&taskName, "task", "t", "", i18n.Text(`shortcut for "maactl run task <task-name>"`, `等价于 "maactl run task <task-name>"`))
	cmd.Flags().StringVarP(&nodeName, "node", "n", "", i18n.Text(`shortcut for "maactl run node <node-name>"`, `等价于 "maactl run node <node-name>"`))
	// Shared execution flags live on run only and are inherited by its subcommands.
	addRunFlags(cmd.PersistentFlags(), &opt)
	cmd.AddCommand(newRunTaskCommand(global, &opt), newRunNodeCommand(global, &opt), plannedRunCommand("preset", i18n.Text("Run the enabled tasks in a PI preset", "运行 PI preset 中启用的任务")))
	return cmd
}

func plannedRunCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use + " <name>", Short: short, Long: i18n.Text("This command is part of the published CLI contract but is not implemented yet. It returns an error when invoked.", "该命令属于已公布的 CLI 契约，但尚未实现，调用时会返回错误。"), Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, _ []string) error { return fmt.Errorf("run %s is not implemented yet", use) }}
	help.MarkCommandPlanned(cmd)
	return cmd
}

func addRunFlags(flags *pflag.FlagSet, opt *runOptions) {
	flags.DurationVar(&opt.stopTimeout, "stop-timeout", 8*time.Second, i18n.Text("maximum wait for task and stop job completion", "等待原任务和停止任务结束的最长时间"))
	flags.StringVarP(&opt.resource, "resource", "r", "", i18n.Text("PI resource name (default: first compatible resource)", "PI 资源名称（默认：第一个兼容资源）"))
	flags.StringVarP(&opt.controller, "controller", "c", "", i18n.Text("PI controller name (default: only controller)", "PI 控制器名称（默认：唯一的控制器）"))
	flags.StringVarP(&opt.adbAddress, "adb-address", "a", "", i18n.Text("ADB device serial/address (default: only detected device)", "ADB 设备序列号/地址（默认：唯一检测到的设备）"))
	flags.StringVar(&opt.adbName, "name", "", i18n.Text("ADB device name reported by MaaToolkit (default: only detected device)", "MaaToolkit 报告的设备名称（默认：唯一检测到的设备）"))
	flags.StringVar(&opt.win32Handle, "win32-handle", "", i18n.Text("Win32 window handle in decimal or 0x-prefixed hex (default: PI win32 regexes, then the only window)", "Win32 窗口句柄，十进制或 0x 开头的十六进制（默认：PI win32 正则，其次唯一窗口）"))
	flags.StringVar(&opt.win32Class, "win32-class", "", i18n.Text("Win32 window class regex (default: PI win32.class_regex)", "Win32 窗口类名正则（默认：PI win32.class_regex）"))
	flags.StringVar(&opt.win32Window, "win32-window", "", i18n.Text("Win32 window title regex (default: PI win32.window_regex)", "Win32 窗口标题正则（默认：PI win32.window_regex）"))
	flags.StringVar(&opt.win32Screencap, "win32-screencap", "", i18n.Text("Win32 screencap method (default: PI win32.screencap, then all methods)", "Win32 截图方式（默认：PI win32.screencap，其次全部方式）"))
	flags.StringVar(&opt.win32Mouse, "win32-mouse", "", i18n.Text("Win32 mouse method (default: PI win32.mouse, then Seize)", "Win32 鼠标方式（默认：PI win32.mouse，其次 Seize）"))
	flags.StringVar(&opt.win32Keyboard, "win32-keyboard", "", i18n.Text("Win32 keyboard method (default: PI win32.keyboard, then Seize)", "Win32 键盘方式（默认：PI win32.keyboard，其次 Seize）"))
	flags.StringVar(&opt.gamepadType, "gamepad-type", "", i18n.Text("Gamepad type: Xbox360 or DualShock4 (default: PI gamepad.gamepad_type, then Xbox360)", "Gamepad 类型：Xbox360 或 DualShock4（默认：PI gamepad.gamepad_type，其次 Xbox360）"))
	flags.StringVarP(&opt.override, "override", "o", "", i18n.Text("final Pipeline override JSON", "最终 Pipeline override JSON"))
	flags.StringVarP(&opt.overrideFile, "override-file", "O", "", i18n.Text("file containing the final Pipeline override JSON", "包含最终 Pipeline override JSON 的文件"))
	flags.StringArrayVarP(&opt.optionValues, "option", "p", nil, i18n.Text("option value as name=<JSON>; repeatable", "option 值，格式 name=<JSON>；可重复"))
	flags.StringArrayVar(&opt.overlay, "overlay", nil, i18n.Text("additional resource root loaded after the selected resource; repeatable", "在所选资源之后加载的额外资源根目录；可重复"))
	flags.String("option-file", "", i18n.Text("JSON file of option values", "包含 option 值的 JSON 文件"))
	flags.Bool("dry-run", false, i18n.Text("resolve and display execution without connecting a controller", "仅解析并显示执行内容，不连接控制器"))
	flags.Bool("explain", false, i18n.Text("display resource and Pipeline override layers", "显示资源与 Pipeline override 各层内容"))
	flags.StringVar(&opt.events, "events", "focus", i18n.Text("event output: focus (PI text), all (all sink events), or off", "事件输出：focus（仅 PI 文本）、all（全部 sink 事件）或 off"))
	flags.DurationVar(&opt.stopAfter, "stop-after", 0, i18n.Text("stop a running task after this duration; for bounded runs and tests", "运行指定时长后停止任务；用于限时运行和测试"))
	flags.BoolVar(&opt.noAgent, "no-agent", false, i18n.Text("do not start the ProjectInterface agent", "不启动 ProjectInterface agent"))
	flags.StringVar(&opt.agentLog, "agent-log", "term", i18n.Text("agent output: term (default, print to this terminal), off (discard), or a directory for one log file per agent", "agent 输出：term（默认，输出到当前终端）、off（丢弃）或目录（每个 agent 一个日志文件）"))
	flags.VisitAll(func(f *pflag.Flag) {
		_ = flags.SetAnnotation(f.Name, help.SectionAnnotation, []string{help.SectionExecution})
	})
	help.MarkFlagsPlanned(flags, "option", "overlay", "option-file", "dry-run", "explain")
}

func newRunTaskCommand(global *Options, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "task <task-name>",
		Short: i18n.Text("Run a task declared in ProjectInterface", "运行 ProjectInterface 中声明的 task"),
		Long: i18n.Text(`Looks up the task by name, resolves its entry node and Pipeline overrides,
then executes it on the selected controller.`, `按名称查找 task，解析其入口节点和 Pipeline override，
然后在所选控制器上执行。`),
		Example: `  maactl run task "自动挂机卖蛋" -f D:\MaaMio --stop-after 10s`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runTask(global, args[0], *opt)
		},
	}
}

func newRunNodeCommand(global *Options, opt *runOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "node <node-name>",
		Short: i18n.Text("Run a Pipeline node from a PI resource", "运行 PI 资源中的 Pipeline 节点"),
		Long: i18n.Text(`The node name is used directly as the task entry; the node is executed with
the selected resource and Pipeline overrides.`, `节点名称直接作为任务入口，
使用所选的资源和 Pipeline override 执行。`),
		Example: `  maactl run node "签到-开始签到" --adb-address 127.0.0.1:16384 --events all`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runNode(global, args[0], *opt)
		},
	}
}

func runTask(global *Options, name string, opt runOptions) error {
	project, err := pi.Load(global.InterfacePath)
	if err != nil {
		return err
	}
	ctrl, err := project.FindController(opt.controller)
	if err != nil {
		return err
	}
	res, err := project.FindResource(opt.resource, ctrl)
	if err != nil {
		return err
	}
	override, err := readOverride(opt)
	if err != nil {
		return err
	}
	t, err := project.FindTask(name, ctrl, res)
	if err != nil {
		return err
	}
	return execute(global, project, ctrl, res, t.Entry, override, opt)
}

func runNode(global *Options, name string, opt runOptions) error {
	project, err := pi.Load(global.InterfacePath)
	if err != nil {
		return err
	}
	ctrl, err := project.FindController(opt.controller)
	if err != nil {
		return err
	}
	res, err := project.FindResource(opt.resource, ctrl)
	if err != nil {
		return err
	}
	override, err := readOverride(opt)
	if err != nil {
		return err
	}
	return execute(global, project, ctrl, res, name, override, opt)
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
