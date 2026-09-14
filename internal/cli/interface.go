package cli

import (
	"fmt"
	"io"

	"maactl/internal/help"
	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"
	"maactl/internal/table"

	"github.com/spf13/cobra"
)

func newInterfaceCommand(global *Options) *cobra.Command {
	actions := []struct{ name, shorthand, description string }{
		{"show", "s", i18n.Text("show ProjectInterface summary", "显示 ProjectInterface 概览")},
		{"controllers", "c", i18n.Text("list controllers (name, label, type)", "列出控制器（名称、显示名称、类型）")},
		{"resources", "r", i18n.Text("list resources (name, label, path)", "列出资源（名称、显示名称、路径）")},
		{"tasks", "t", i18n.Text("list tasks (name, label, entry)", "列出任务（名称、显示名称、入口节点）")},
		{"validate", "v", i18n.Text("validate that the ProjectInterface loads", "验证 ProjectInterface 是否可加载")},
		{"options", "", i18n.Text("list PI option definitions", "列出 PI option 定义")},
		{"presets", "", i18n.Text("list PI presets", "列出 PI preset")},
	}
	cmd := &cobra.Command{
		Use: "interface", Short: i18n.Text("Inspect and validate a ProjectInterface", "检查和验证 ProjectInterface"),
		Long: i18n.Text(`Exactly one action is required: --show, --controllers, --resources, --tasks,
or --validate. --options and --presets are planned.`, `必须且只能选择一个操作：--show、--controllers、--resources、--tasks
或 --validate；--options 和 --presets 尚未实现。`),
		Example: `  maactl interface --tasks -f D:\MaaMio
  maactl interface --validate -f D:\MaaMio --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			action := ""
			for _, candidate := range actions {
				selected, _ := c.Flags().GetBool(candidate.name)
				if !selected {
					continue
				}
				if action != "" {
					return fmt.Errorf("choose only one interface action: --%s or --%s", action, candidate.name)
				}
				action = candidate.name
			}
			if action == "" {
				return c.Help()
			}
			return inspectInterface(c.OutOrStdout(), global, action)
		},
	}
	for _, action := range actions {
		cmd.Flags().BoolP(action.name, action.shorthand, false, action.description)
	}
	help.MarkFlagsSection(cmd.Flags(), help.SectionAction, "show", "controllers", "resources", "tasks", "validate", "options", "presets")
	help.MarkFlagsPlanned(cmd.Flags(), "options", "presets")
	return cmd
}

func inspectInterface(out io.Writer, global *Options, action string) error {
	if action == "options" || action == "presets" {
		return fmt.Errorf("interface --%s is not implemented yet", action)
	}
	project, err := pi.Load(global.InterfacePath)
	if err != nil {
		return err
	}
	var value any
	var headers []string
	var rows [][]string
	switch action {
	case "show":
		value = map[string]any{"path": project.Path, "name": project.Name, "label": project.Label, "interface_version": project.InterfaceVersion, "controllers": len(project.Controller), "resources": len(project.Resource), "tasks": len(project.Task)}
	case "validate":
		value = map[string]any{"valid": true, "path": project.Path}
	case "controllers":
		value = project.Controller
		headers = []string{"名称 (name)", "显示名称 (label)", "类型 (type)"}
		for _, item := range project.Controller {
			rows = append(rows, []string{item.Name, item.Label, item.Type})
		}
	case "resources":
		value = project.Resource
		headers = []string{"名称 (name)", "显示名称 (label)", "资源路径 (path)"}
		for _, item := range project.Resource {
			rows = append(rows, []string{item.Name, item.Label, pi.Join(item.Path)})
		}
	case "tasks":
		value = project.Task
		headers = []string{"名称 (name)", "显示名称 (label)", "入口节点 (entry)"}
		for _, item := range project.Task {
			rows = append(rows, []string{item.Name, item.Label, item.Entry})
		}
	}
	if global.JSON {
		return output.JSON(out, value)
	}
	switch action {
	case "show":
		_, err = fmt.Fprintf(out, "%s (%s)\ninterface: %s\ncontrollers: %d\nresources: %d\ntasks: %d\n", output.Value(project.Label), output.Value(project.Name), project.Path, len(project.Controller), len(project.Resource), len(project.Task))
		return err
	case "validate":
		_, err = fmt.Fprintf(out, "Valid ProjectInterface: %s\n", project.Path)
		return err
	default:
		return table.Print(out, headers, rows)
	}
}
