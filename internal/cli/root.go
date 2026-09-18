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
		Use: "maactl",
		// No `Version` field: cobra prints that value before any hook runs, and the
		// version line ends with the MaaFramework version, which can only be read
		// after the runtime is loaded—and loading it honors --lib-dir, which is
		// only parsed by then. `-v`/`--version` is therefore answered in Run below.
		Short: i18n.Text("MaaFramework and ProjectInterface command-line client", "MaaFramework 与 ProjectInterface 命令行客户端"),
		Long: i18n.Text(`MaaCtl loads ProjectInterface v2 projects, inspects MaaFramework resources,
and runs Pipeline tasks.

Commands are grouped by what they act on: pi reads the project's declarations,
resource reports resources (declared and loaded), device lists devices and
windows, run executes, and config shows the client configuration.`, `MaaCtl 加载 ProjectInterface v2 项目，检查 MaaFramework 资源，并运行 Pipeline 任务。

命令按操作对象分组：pi 读取项目声明，resource 报告资源（声明的与已加载的），
device 列出设备与窗口，run 执行任务，config 显示客户端配置。`),
		Example: `  maactl pi t -if D:\projects\demo
  maactl resource l -if D:\projects\demo
  maactl run -t 签到 -if D:\01_Projects\github\MaaMio -sa 30s
  maactl device adb -j`,
		SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
				return printVersion(cmd, version, &global)
			}
			return cmd.Help()
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return withExitCode(ExitUsage, err) })
	root.PersistentFlags().StringVarP(&global.InterfacePath, "interface", "f", "", i18n.Text("ProjectInterface file or project directory (default: ./interface.json)", "ProjectInterface 文件或项目目录（默认：./interface.json）"))
	root.PersistentFlags().StringVarP(&global.LibDir, "lib-dir", "l", "", i18n.Text("MaaFramework library directory (default: ./maafw/bin)", "MaaFramework 运行库目录（默认：./maafw/bin）"))
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
		newResourceCommand(&global),
		newDeviceCommand(&global),
		newRunCommand(&global),
		newConfigCommand(&global),
		newVersionCommand(version, &global),
		newSelfCheckCommand(&global),
	)
	root.AddCommand(newLegacyDeviceCommands(&global)...)

	// Create the default completion command now so its help can be localized.
	root.InitDefaultCompletionCmd()
	localizeCompletion(root)
	// Pin flag order before anything else materializes the merged flag sets, and
	// before help annotations are attached.
	preserveFlagOrder(root)
	applyFlagAliases(root)
	help.SetFullHelp(root)
	applyUsageArgs(root)
	return root
}

// preserveFlagOrder disables pflag's alphabetical sorting so help lists flags in
// the order each command declares them, which keeps related flags next to each
// other. Cobra merges persistent flags into per-command sets lazily, so the raw
// sets must be unsorted before that merge happens.
func preserveFlagOrder(root *cobra.Command) {
	walkCommands(root, func(cmd *cobra.Command) {
		cmd.Flags().SortFlags = false
		cmd.PersistentFlags().SortFlags = false
	})
	walkCommands(root, func(cmd *cobra.Command) {
		cmd.LocalFlags().SortFlags = false
		cmd.InheritedFlags().SortFlags = false
	})
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

// runtimeVersion is the seam tests use to answer the version line without a
// MaaFramework release on disk; production always loads the real runtime.
var runtimeVersion = maafw.RuntimeVersion

// printVersion writes the version line: maactl's own version, plus the
// MaaFramework version the runtime reports when one can be loaded.
//
// The runtime part costs a load—there is no build-time stamp to fall back on—so
// a machine without a usable runtime still gets an answer, and --verbose says
// why the runtime part is missing. `selfcheck` is the command that treats such
// problems as failures.
func printVersion(cmd *cobra.Command, version string, global *GlobalOptions) error {
	line := "maactl version " + version
	runtime, _, err := runtimeVersion(global.LibDir, global.LogDir)
	switch {
	case err != nil:
		if global.Verbose {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot read the MaaFramework version: %v\n", err)
		}
	case strings.TrimSpace(runtime) != "":
		line += " (MaaFramework " + strings.TrimSpace(runtime) + ")"
	}
	fmt.Fprintln(cmd.OutOrStdout(), line)
	return nil
}

// newVersionCommand prints the version, mirroring the root --version flag.
func newVersionCommand(version string, global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"ver"},
		Short:   i18n.Text("Print version information", "显示版本信息"),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd, version, global)
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
