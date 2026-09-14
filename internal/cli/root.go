// Package cli builds the maactl command tree.
package cli

import (
	"fmt"
	"strings"

	"maactl/internal/clientconfig"
	"maactl/internal/help"
	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/pi"

	"github.com/spf13/cobra"
)

// GlobalOptions holds the flags shared by every command.
type GlobalOptions struct {
	// InterfacePath is the PI file or project directory (-f/--interface).
	InterfacePath string
	// LibDir overrides the MaaFramework runtime directory (-l/--lib-dir).
	LibDir string
	// JSON switches query output and sink events to JSON (-j/--json).
	JSON bool
	// Lang selects the PI label language (--lang).
	Lang string
	// ConfigPath points at the client configuration file (--config).
	ConfigPath string
	// NoConfig disables client configuration discovery (--no-config).
	NoConfig bool
	// LogDir is MaaFramework's log directory (--log-dir).
	LogDir string
	// Verbose adds selection and merge details (--verbose).
	Verbose bool
}

// NewRootCommand builds the maactl command tree. version is injected by the
// main package, normally through -ldflags "-X main.version=<version>".
func NewRootCommand(version string) *cobra.Command {
	clientVersion = version
	// Keep the intentional command order instead of alphabetical sorting.
	cobra.EnableCommandSorting = false
	var global GlobalOptions
	root := &cobra.Command{
		Use:     "maactl",
		Version: versionLabel(version),
		Short:   i18n.Text("MaaFramework and ProjectInterface command-line client", "MaaFramework 与 ProjectInterface 命令行客户端"),
		Long: i18n.Text(`MaaCtl loads ProjectInterface v2 projects, inspects MaaFramework resources,
and runs Pipeline tasks.

Commands are grouped by what they act on: pi inspects the project, device lists
controllers, resource inspects loaded resources, and run executes tasks.

Use positional arguments only for commands and required names. Every option
starts with - or --.`, `MaaCtl 加载 ProjectInterface v2 项目，检查 MaaFramework 资源，
并运行 Pipeline 任务。

命令按操作对象分组：pi 检查项目，device 列出设备，resource 检查已加载资源，
run 执行任务。

只有命令名和必需的名称使用位置参数。所有选项以 - 或 -- 开头。`),
		Example: `  maactl pi tasks -f D:\projects\demo
  maactl run task "签到" -f D:\01_Projects\github\MaaMio --stop-after 30s
  maactl run -t "签到" --explain --dry-run
  maactl device adb --json`,
		SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return withExitCode(ExitUsage, err) })
	root.PersistentFlags().StringVarP(&global.InterfacePath, "interface", "f", "", i18n.Text("ProjectInterface file or project directory (default: ./interface.json)", "ProjectInterface 文件或项目目录（默认：./interface.json）"))
	root.PersistentFlags().StringVarP(&global.LibDir, "lib-dir", "l", "", i18n.Text("MaaFramework DLL directory (default: ./maafw/bin)", "MaaFramework DLL 目录（默认：./maafw/bin）"))
	root.PersistentFlags().BoolVarP(&global.JSON, "json", "j", false, i18n.Text("output JSON; run emits sink events as JSON", "输出 JSON；运行时 sink 事件也输出 JSON"))
	root.PersistentFlags().StringVar(&global.Lang, "lang", "", i18n.Text("language for PI labels, e.g. zh_cn or en_us (default: system)", "解析 PI label 的语言，如 zh_cn 或 en_us（默认：跟随系统）"))
	root.PersistentFlags().StringVar(&global.ConfigPath, "config", "", i18n.Text("client configuration file (default: <PI>/config/maa_pi_config.json)", "客户端配置文件（默认：<PI>/config/maa_pi_config.json）"))
	root.PersistentFlags().BoolVar(&global.NoConfig, "no-config", false, i18n.Text("do not read the client configuration file", "不读取客户端配置文件"))
	root.PersistentFlags().StringVar(&global.LogDir, "log-dir", "", i18n.Text("MaaFramework log directory", "MaaFramework 日志目录"))
	root.PersistentFlags().BoolVar(&global.Verbose, "verbose", false, i18n.Text("show selection and merge details", "输出选择与合并细节"))

	// Define --version here with the -v shorthand. It stays a root-local flag so
	// subcommands are free to use -v for their own meaning (none does today).
	root.Flags().BoolP("version", "v", false, i18n.Text("print version information", "显示版本信息"))

	root.AddCommand(
		newPICommand(&global),
		newDeviceCommand(&global),
		newResourceCommand(&global),
		newRunCommand(&global),
		newConfigCommand(&global),
		newVersionCommand(version),
	)
	root.AddCommand(newLegacyDeviceCommands(&global)...)

	// Create the default completion command now so its help can be localized.
	root.InitDefaultCompletionCmd()
	localizeCompletion(root)
	help.SetFullHelp(root)
	applyUsageArgs(root)
	return root
}

// Language returns the language code used for PI labels.
func (g *GlobalOptions) Language() string {
	if value := strings.TrimSpace(g.Lang); value != "" {
		return value
	}
	return i18n.Code()
}

// LoadProject loads the ProjectInterface named by the global flags.
func (g *GlobalOptions) LoadProject() (*pi.Loaded, error) {
	project, err := pi.Load(g.InterfacePath)
	if err != nil {
		return nil, withExitCode(ExitUsage, err)
	}
	return project, nil
}

// LoadConfig loads the client configuration: an explicit --config path, the
// discovered conventional file, or an empty config when none exists.
func (g *GlobalOptions) LoadConfig(project *pi.Loaded) (*clientconfig.Config, string, error) {
	if g.NoConfig {
		return &clientconfig.Config{}, "", nil
	}
	if path := strings.TrimSpace(g.ConfigPath); path != "" {
		config, err := clientconfig.Load(path)
		if err != nil {
			return nil, path, withExitCode(ExitUsage, err)
		}
		return config, config.Path, nil
	}
	piPath := ""
	if project != nil {
		piPath = project.Path
	}
	config, path, err := clientconfig.Discover(piPath)
	if err != nil {
		return nil, path, withExitCode(ExitUsage, err)
	}
	return config, path, nil
}

// versionLabel appends the bundled MaaFramework version so users can tell which
// runtime a self-contained executable carries.
func versionLabel(version string) string {
	if bundled := maafw.BundledVersion(); strings.TrimSpace(bundled) != "" {
		return fmt.Sprintf("%s (MaaFramework %s)", version, bundled)
	}
	return version
}

// newVersionCommand prints the version, mirroring the root --version flag.
func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: i18n.Text("Print version information", "显示版本信息"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "maactl version "+versionLabel(version))
			return nil
		},
	}
}

// applyUsageArgs converts cobra's argument validation errors into usage exit
// codes so scripts can tell a bad command line from a failed run.
func applyUsageArgs(root *cobra.Command) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Args != nil {
			inner := cmd.Args
			cmd.Args = func(c *cobra.Command, args []string) error {
				return withExitCode(ExitUsage, inner(c, args))
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

// localizeCompletion translates the cobra-generated completion command. Its
// English descriptions are kept as-is for the English help language.
func localizeCompletion(root *cobra.Command) {
	if i18n.Active() != i18n.ZH {
		return
	}
	for _, cmd := range root.Commands() {
		if cmd.Name() != "completion" {
			continue
		}
		cmd.Short = "为指定的 shell 生成自动补全脚本"
		cmd.Long = "为指定的 shell 生成 maactl 的自动补全脚本。\n每个子命令的帮助中包含生成脚本的使用方法。"
		for _, child := range cmd.Commands() {
			child.Short = fmt.Sprintf("为 %s 生成自动补全脚本", child.Name())
			child.Long = fmt.Sprintf("为 %s 生成 maactl 自动补全脚本。\n", child.Name())
			if flag := child.Flags().Lookup("no-descriptions"); flag != nil {
				flag.Usage = "禁用补全描述"
			}
		}
		return
	}
}
