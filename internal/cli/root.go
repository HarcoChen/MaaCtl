// Package cli builds the maactl command tree.
package cli

import (
	"fmt"

	"maactl/internal/help"
	"maactl/internal/i18n"

	"github.com/spf13/cobra"
)

// Options holds the global flags shared by every command.
type Options struct {
	LibDir        string
	InterfacePath string
	JSON          bool
}

// NewRootCommand builds the maactl command tree. version is injected by the
// main package, normally through -ldflags "-X main.version=<version>".
func NewRootCommand(version string) *cobra.Command {
	// Keep the intentional command order instead of alphabetical sorting.
	cobra.EnableCommandSorting = false
	var global Options
	root := &cobra.Command{
		Use:     "maactl",
		Version: version,
		Short:   i18n.Text("MaaFramework and ProjectInterface command-line client", "MaaFramework 与 ProjectInterface 命令行客户端"),
		Long: i18n.Text(`MaaCtl loads ProjectInterface v2 projects, inspects MaaFramework resources,
and runs Pipeline tasks.

Use positional arguments only for commands and required task/node names.
Every option starts with - or --.`, `MaaCtl 加载 ProjectInterface v2 项目，检查 MaaFramework 资源，
并运行 Pipeline 任务。

只有命令名和必需的 task/node 名称使用位置参数。所有选项以 - 或 -- 开头。`),
		Example: `  maactl interface --show -f D:\projects\demo
  maactl run task "自动挂机卖蛋" -f D:\projects\demo --stop-after 10s
  maactl adb devices --json`,
		SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().StringVarP(&global.LibDir, "lib-dir", "l", "", i18n.Text("MaaFramework DLL directory (default: ./maafw/bin)", "MaaFramework DLL 目录（默认：./maafw/bin）"))
	root.PersistentFlags().StringVarP(&global.InterfacePath, "interface", "f", "", i18n.Text("ProjectInterface file or project directory (default: ./interface.json)", "ProjectInterface 文件或项目目录（默认：./interface.json）"))
	root.PersistentFlags().BoolVarP(&global.JSON, "json", "j", false, i18n.Text("output JSON; run emits sink events as JSON", "输出 JSON；运行时 sink 事件也输出 JSON"))
	// Define --version without a shorthand so -v stays reserved for
	// flag shorthands such as "interface --validate".
	root.Flags().Bool("version", false, i18n.Text("print version information", "显示版本信息"))
	root.AddCommand(newADBCommand(&global), newWin32Command(&global), newInterfaceCommand(&global), newResourceCommand(&global), newRunCommand(&global))
	// Create the default completion command now so its help can be localized.
	root.InitDefaultCompletionCmd()
	localizeCompletion(root)
	help.SetFullHelp(root)
	return root
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
