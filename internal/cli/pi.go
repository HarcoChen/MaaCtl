package cli

import (
	"fmt"
	"io"
	"strings"

	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"
	"maactl/internal/table"

	"github.com/spf13/cobra"
)

// newPICommand builds the `pi` group: every subcommand reads the
// ProjectInterface only, so they work without a device or a controller.
func newPICommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pi",
		Aliases: []string{"interface"},
		Short:   i18n.Text("Inspect and validate a ProjectInterface", "检查和验证 ProjectInterface"),
		Long: i18n.Text(`pi reads the ProjectInterface named by --interface/-f (default: ./interface.json)
and reports what it declares. No controller is created and no Pipeline runs, so
these commands work without a connected device.`, `pi 读取 --interface/-f 指定的 ProjectInterface（默认：./interface.json）
并报告其中的声明。不会创建控制器，也不会运行 Pipeline，因此不需要连接设备。`),
		Example: `  maactl pi info -f D:\01_Projects\github\MaaMio
  maactl pi tasks -f D:\01_Projects\github\MaaMio
  maactl pi validate -f D:\01_Projects\github\MaaMio --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(
		newPIInfoCommand(global),
		newPIValidateCommand(global),
		newPIControllersCommand(global),
		newPIResourcesCommand(global),
		newPITasksCommand(global),
		newPIGroupsCommand(global),
		newPIOptionsCommand(global),
		newPIPresetsCommand(global),
		newPISettingsCommand(global),
	)
	return cmd
}

// piContext is the loaded project plus the language used for labels.
type piContext struct {
	global     *GlobalOptions
	project    *pi.Loaded
	translator *pi.Translator
}

// loadPI loads the project and builds its translator.
func loadPI(global *GlobalOptions) (*piContext, error) {
	project, err := global.LoadProject()
	if err != nil {
		return nil, err
	}
	return &piContext{global: global, project: project, translator: project.Translator(global.Language())}, nil
}

// label resolves a PI label, falling back to the name.
func (c *piContext) label(label, name string) string {
	if strings.TrimSpace(label) == "" {
		return name
	}
	return c.translator.Resolve(label)
}

func newPIInfoCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: i18n.Text("Show the ProjectInterface summary", "显示 ProjectInterface 概览"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return outputInfo(cmd.OutOrStdout(), ctx)
		},
	}
}

// infoOutput is the JSON shape of `pi info`.
type infoOutput struct {
	Path             string   `json:"path"`
	Files            []string `json:"files"`
	Name             string   `json:"name"`
	Label            string   `json:"label"`
	Version          string   `json:"version,omitempty"`
	InterfaceVersion int      `json:"interface_version"`
	ProtocolVersion  string   `json:"protocol_version"`
	Description      string   `json:"description,omitempty"`
	Languages        []string `json:"languages,omitempty"`
	Language         string   `json:"language,omitempty"`
	Controllers      int      `json:"controllers"`
	Resources        int      `json:"resources"`
	Tasks            int      `json:"tasks"`
	Groups           int      `json:"groups"`
	Options          int      `json:"options"`
	Presets          int      `json:"presets"`
	Settings         int      `json:"settings"`
	Pretasks         int      `json:"pretasks"`
	Agents           int      `json:"agents"`
	GlobalOption     []string `json:"global_option,omitempty"`
	RunnableTypes    []string `json:"runnable_controller_types"`
	Telemetry        bool     `json:"telemetry"`
}

func outputInfo(out io.Writer, ctx *piContext) error {
	project := ctx.project
	languages := make([]string, 0, len(project.Languages))
	for code := range project.Languages {
		languages = append(languages, code)
	}
	info := infoOutput{
		Path:             project.Path,
		Files:            project.Files,
		Name:             project.Name,
		Label:            ctx.label(project.Label, project.Name),
		Version:          project.Version,
		InterfaceVersion: project.InterfaceVersion,
		ProtocolVersion:  pi.ProtocolVersion,
		Description:      ctx.translator.Resolve(project.Description),
		Languages:        languages,
		Language:         ctx.translator.Lang(),
		Controllers:      len(project.Controller),
		Resources:        len(project.Resource),
		Tasks:            len(project.Task),
		Groups:           len(project.Group),
		Options:          project.Option.Len(),
		Presets:          len(project.Preset),
		Settings:         len(project.Setting),
		Pretasks:         len(project.Pretask),
		Agents:           len(project.Agent),
		GlobalOption:     project.GlobalOption,
		RunnableTypes:    pi.RunnableControllerTypes(),
		Telemetry:        project.Telemetry != nil && project.Telemetry.Sentry != nil && project.Telemetry.Sentry.DSN != "",
	}
	if ctx.global.JSON {
		return output.JSON(out, info)
	}
	fmt.Fprintf(out, "%s (%s) %s\n", output.Value(info.Label), output.Value(info.Name), output.Value(info.Version))
	fmt.Fprintf(out, "interface: %s\n", info.Path)
	if len(info.Files) > 1 {
		fmt.Fprintf(out, "files: %d (import merged)\n", len(info.Files))
	}
	fmt.Fprintf(out, "protocol: PI %s (interface_version %d)\n", info.ProtocolVersion, info.InterfaceVersion)
	if info.Language != "" {
		fmt.Fprintf(out, "language: %s\n", info.Language)
	}
	fmt.Fprintf(out, "controllers: %d\nresources: %d\ntasks: %d\ngroups: %d\noptions: %d\npresets: %d\nsettings: %d\n",
		info.Controllers, info.Resources, info.Tasks, info.Groups, info.Options, info.Presets, info.Settings)
	if info.Pretasks > 0 || info.Agents > 0 {
		fmt.Fprintf(out, "pretasks: %d\nagents: %d\n", info.Pretasks, info.Agents)
	}
	if info.Telemetry {
		fmt.Fprintf(out, "telemetry: declared (maactl does not report telemetry)\n")
	}
	return nil
}

func newPIValidateCommand(global *GlobalOptions) *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: i18n.Text("Validate the ProjectInterface", "校验 ProjectInterface"),
		Long: i18n.Text(`Checks that the ProjectInterface parses, that every name is unique, that every
reference points at something that exists, and that the declared files are on
disk. --strict also fails on advisory findings (a controller this platform
cannot create, a missing resource path, a missing language file).`, `校验 ProjectInterface 能否解析、名称是否唯一、引用是否存在，以及声明的
文件是否在磁盘上。--strict 会把提示性问题（当前平台无法创建的控制器、缺失的资源
路径、缺失的语言文件）也视为失败。`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			report := ctx.project.Validate(pi.ValidateOptions{Strict: strict})
			out := cmd.OutOrStdout()
			if global.JSON {
				if err := output.JSON(out, report); err != nil {
					return err
				}
			} else {
				for _, issue := range report.Issues {
					fmt.Fprintf(out, "%s: %s: %s\n", strings.ToUpper(string(issue.Level)), issue.Path, issue.Message)
				}
				if report.OK() {
					fmt.Fprintf(out, "Valid ProjectInterface: %s (%d warning(s))\n", ctx.project.Path, report.Warnings())
				}
			}
			if !report.OK() {
				return exitErrorf(ExitUsage, "%s failed validation: %d error(s), %d warning(s)", ctx.project.Path, report.Errors(), report.Warnings())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, i18n.Text("treat advisory findings as errors", "把提示性问题视为错误"))
	return cmd
}

func newPIControllersCommand(global *GlobalOptions) *cobra.Command {
	var typeFilter string
	cmd := &cobra.Command{
		Use:   "controllers",
		Short: i18n.Text("List controllers", "列出控制器"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listControllers(cmd.OutOrStdout(), ctx, typeFilter)
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", i18n.Text("only show this controller type", "只显示该类型的控制器"))
	return cmd
}

// controllerRow is one row of `pi controllers`.
type controllerRow struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Runnable  bool   `json:"runnable"`
	Options   int    `json:"options"`
	Resources int    `json:"resources"`
}

func listControllers(out io.Writer, ctx *piContext, typeFilter string) error {
	rows := make([]controllerRow, 0, len(ctx.project.Controller))
	for i := range ctx.project.Controller {
		ctrl := &ctx.project.Controller[i]
		if typeFilter != "" && !strings.EqualFold(ctrl.Type, typeFilter) {
			continue
		}
		resources := 0
		for j := range ctx.project.Resource {
			if pi.Compatible(ctx.project.Resource[j].Controller, ctrl.Name) {
				resources++
			}
		}
		rows = append(rows, controllerRow{
			Name:      ctrl.Name,
			Label:     ctx.label(ctrl.Label, ctrl.Name),
			Type:      ctrl.Type,
			Runnable:  pi.ControllerRunnable(ctrl),
			Options:   len(ctrl.Option),
			Resources: resources,
		})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		runnable := i18n.Text("no", "否")
		if row.Runnable {
			runnable = i18n.Text("yes", "是")
		}
		tableRows = append(tableRows, []string{row.Name, row.Label, row.Type, runnable, fmt.Sprint(row.Options), fmt.Sprint(row.Resources)})
	}
	return table.Print(out, []string{
		i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("type", "类型"),
		i18n.Text("runnable", "可运行"), i18n.Text("options", "选项数"), i18n.Text("resources", "兼容资源数"),
	}, tableRows)
}

func newPIResourcesCommand(global *GlobalOptions) *cobra.Command {
	var controllerFilter string
	cmd := &cobra.Command{
		Use:   "resources",
		Short: i18n.Text("List resources", "列出资源"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listResources(cmd.OutOrStdout(), ctx, controllerFilter)
		},
	}
	cmd.Flags().StringVarP(&controllerFilter, "controller", "c", "", i18n.Text("only show resources compatible with this controller", "只显示与指定控制器兼容的资源"))
	return cmd
}

// resourceRow is one row of `pi resources`.
type resourceRow struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Path        []string `json:"path"`
	Hash        string   `json:"hash,omitempty"`
	Controllers []string `json:"controllers,omitempty"`
	Options     int      `json:"options"`
	Compatible  bool     `json:"compatible"`
}

func listResources(out io.Writer, ctx *piContext, controllerFilter string) error {
	if controllerFilter != "" {
		if _, err := ctx.project.FindController(controllerFilter); err != nil {
			return withExitCode(ExitUsage, err)
		}
	}
	rows := make([]resourceRow, 0, len(ctx.project.Resource))
	for i := range ctx.project.Resource {
		res := &ctx.project.Resource[i]
		compatible := controllerFilter == "" || pi.Compatible(res.Controller, controllerFilter)
		rows = append(rows, resourceRow{
			Name:        res.Name,
			Label:       ctx.label(res.Label, res.Name),
			Path:        res.Path,
			Hash:        res.Hash,
			Controllers: res.Controller,
			Options:     len(res.Option),
			Compatible:  compatible,
		})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		compatible := "-"
		if controllerFilter != "" {
			if row.Compatible {
				compatible = i18n.Text("yes", "是")
			} else {
				compatible = i18n.Text("no", "否")
			}
		}
		tableRows = append(tableRows, []string{row.Name, row.Label, pi.Join(row.Path), output.Value(row.Hash), formatList(row.Controllers), compatible})
	}
	return table.Print(out, []string{
		i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("path", "资源路径"),
		i18n.Text("hash", "hash"), i18n.Text("controllers", "限定控制器"), i18n.Text("compatible", "兼容"),
	}, tableRows)
}

func newPITasksCommand(global *GlobalOptions) *cobra.Command {
	var controllerFilter, resourceFilter, groupFilter string
	var all bool
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: i18n.Text("List tasks", "列出任务"),
		Long: i18n.Text(`Lists tasks with their entry node, groups, and option count. Tasks that do not
match the selected controller/resource are hidden unless --all is given, which
also prints why they are unavailable.`, `列出任务及其入口节点、分组和选项数量。与所选控制器/资源不匹配的任务默认隐藏；
使用 --all 会一并列出并说明不可用原因。`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listTasks(cmd.OutOrStdout(), ctx, controllerFilter, resourceFilter, groupFilter, all)
		},
	}
	cmd.Flags().StringVarP(&controllerFilter, "controller", "c", "", i18n.Text("filter by controller", "按控制器过滤"))
	cmd.Flags().StringVarP(&resourceFilter, "resource", "r", "", i18n.Text("filter by resource", "按资源过滤"))
	cmd.Flags().StringVar(&groupFilter, "group", "", i18n.Text("filter by group", "按分组过滤"))
	cmd.Flags().BoolVar(&all, "all", false, i18n.Text("include tasks that do not apply, with the reason", "包含不适用的任务并给出原因"))
	return cmd
}

// taskRow is one row of `pi tasks`.
type taskRow struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Entry       string   `json:"entry"`
	Group       []string `json:"group,omitempty"`
	Options     int      `json:"options"`
	Compatible  bool     `json:"compatible"`
	Unavailable []string `json:"unavailable,omitempty"`
}

func listTasks(out io.Writer, ctx *piContext, controllerFilter, resourceFilter, groupFilter string, all bool) error {
	var ctrl *pi.Controller
	if controllerFilter != "" {
		resolved, err := ctx.project.FindController(controllerFilter)
		if err != nil {
			return withExitCode(ExitUsage, err)
		}
		ctrl = resolved
	}
	var res *pi.Resource
	if resourceFilter != "" {
		if ctrl == nil {
			if len(ctx.project.Controller) == 1 {
				ctrl = &ctx.project.Controller[0]
			}
		}
		resolved, err := ctx.project.FindResource(resourceFilter, ctrl)
		if err != nil {
			return withExitCode(ExitUsage, err)
		}
		res = resolved
	}
	rows := make([]taskRow, 0, len(ctx.project.Task))
	for i := range ctx.project.Task {
		task := &ctx.project.Task[i]
		if groupFilter != "" && !containsName(task.Group, groupFilter) {
			continue
		}
		reasons := ctx.project.TaskReasons(task, ctrl, res)
		if len(reasons) > 0 && !all {
			continue
		}
		rows = append(rows, taskRow{
			Name:        task.Name,
			Label:       ctx.label(task.Label, task.Name),
			Entry:       task.Entry,
			Group:       task.Group,
			Options:     len(task.Option),
			Compatible:  len(reasons) == 0,
			Unavailable: reasons,
		})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	headers := []string{i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("entry", "入口节点"), i18n.Text("group", "分组"), i18n.Text("options", "选项数")}
	if all {
		headers = append(headers, i18n.Text("unavailable", "不可用原因"))
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := []string{row.Name, row.Label, output.Value(row.Entry), formatList(row.Group), fmt.Sprint(row.Options)}
		if all {
			values = append(values, output.Value(strings.Join(row.Unavailable, "; ")))
		}
		tableRows = append(tableRows, values)
	}
	return table.Print(out, headers, tableRows)
}

func newPIGroupsCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "groups",
		Short: i18n.Text("List task groups", "列出任务分组"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listGroups(cmd.OutOrStdout(), ctx)
		},
	}
}

// groupRow is one row of `pi groups`.
type groupRow struct {
	Name          string `json:"name"`
	Label         string `json:"label"`
	DefaultExpand bool   `json:"default_expand"`
	Tasks         int    `json:"tasks"`
}

func listGroups(out io.Writer, ctx *piContext) error {
	rows := make([]groupRow, 0, len(ctx.project.Group))
	for i := range ctx.project.Group {
		group := &ctx.project.Group[i]
		tasks := 0
		for j := range ctx.project.Task {
			if pi.Compatible(ctx.project.Task[j].Group, group.Name) {
				tasks++
			}
		}
		rows = append(rows, groupRow{Name: group.Name, Label: ctx.label(group.Label, group.Name), DefaultExpand: group.Expands(), Tasks: tasks})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		expand := i18n.Text("no", "否")
		if row.DefaultExpand {
			expand = i18n.Text("yes", "是")
		}
		tableRows = append(tableRows, []string{row.Name, row.Label, expand, fmt.Sprint(row.Tasks)})
	}
	return table.Print(out, []string{i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("default_expand", "默认展开"), i18n.Text("tasks", "任务数")}, tableRows)
}

func newPIPresetsCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "presets",
		Short: i18n.Text("List presets", "列出预设"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listPresets(cmd.OutOrStdout(), ctx)
		},
	}
}

// presetRow is one row of `pi presets`.
type presetRow struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Tasks       []string `json:"tasks"`
	Disabled    []string `json:"disabled,omitempty"`
}

func listPresets(out io.Writer, ctx *piContext) error {
	rows := make([]presetRow, 0, len(ctx.project.Preset))
	for i := range ctx.project.Preset {
		preset := &ctx.project.Preset[i]
		row := presetRow{
			Name:        preset.Name,
			Label:       ctx.label(preset.Label, preset.Name),
			Description: ctx.translator.Resolve(preset.Description),
		}
		for _, entry := range preset.Task {
			if entry.EnabledOrDefault() {
				row.Tasks = append(row.Tasks, entry.Name)
			} else {
				row.Disabled = append(row.Disabled, entry.Name)
			}
		}
		rows = append(rows, row)
	}
	if ctx.global.JSON {
		return output.JSON(out, ctx.project.Preset)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.Name, row.Label, fmt.Sprint(len(row.Tasks)), fmt.Sprint(len(row.Disabled))})
	}
	return table.Print(out, []string{i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("tasks", "启用任务数"), i18n.Text("disabled", "禁用任务数")}, tableRows)
}

func newPISettingsCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "settings",
		Short: i18n.Text("List setting sections", "列出设置分区"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listSettings(cmd.OutOrStdout(), ctx)
		},
	}
}

// settingRow is one row of `pi settings`.
type settingRow struct {
	Name          string   `json:"name"`
	Label         string   `json:"label"`
	DefaultExpand bool     `json:"default_expand"`
	Option        []string `json:"option,omitempty"`
}

func listSettings(out io.Writer, ctx *piContext) error {
	rows := make([]settingRow, 0, len(ctx.project.Setting))
	for i := range ctx.project.Setting {
		setting := &ctx.project.Setting[i]
		rows = append(rows, settingRow{Name: setting.Name, Label: ctx.label(setting.Label, setting.Name), DefaultExpand: setting.Expands(), Option: setting.Option})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		expand := i18n.Text("no", "否")
		if row.DefaultExpand {
			expand = i18n.Text("yes", "是")
		}
		tableRows = append(tableRows, []string{row.Name, row.Label, expand, formatList(row.Option)})
	}
	return table.Print(out, []string{i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("default_expand", "默认展开"), i18n.Text("options", "选项")}, tableRows)
}

// formatList renders a string list for a table cell.
func formatList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return pi.Join(items)
}

// containsName reports exact membership, used for --group filtering where an
// empty list means "not in any group" rather than "unrestricted".
func containsName(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
