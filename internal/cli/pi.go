package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"

	"github.com/spf13/cobra"
)

// newPICommand builds the `pi` group: every subcommand reads the
// ProjectInterface only, so they work without a device or a controller.
func newPICommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pi",
		Aliases: []string{"if", "interface"},
		Short:   i18n.Text("Inspect the ProjectInterface", "检查 ProjectInterface"),
		Long: i18n.Text(`Reads the ProjectInterface named by -if/--interface (default ./interface.json)
and reports what it declares. No controller is created, so a device is not needed.

Resource inspection lives in "maactl resource".`, `读取 -if/--interface 指定的 ProjectInterface（默认 ./interface.json）并报告其中
的声明。不会创建控制器，因此不需要设备。

资源检查在 "maactl resource" 下。`),
		Example: `  maactl pi info -if D:\MaaMio
  maactl pi t -c Android
  maactl pi validate -st`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(
		newPIInfoCommand(global),
		newPIValidateCommand(global),
		newPIControllersCommand(global),
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

// piQuerySpec describes one read-only `pi` subcommand. They all share the same
// shape — load the ProjectInterface, then report something about it — so they
// differ only in this spec.
type piQuerySpec struct {
	use     string
	aliases []string
	short   string
	long    string
	example string
	// flags registers the command's own flags; nil when it has none.
	flags func(*cobra.Command)
	// run reports the query result once the project is loaded.
	run func(ctx *piContext, out io.Writer) error
}

// newPIQueryCommand builds a `pi` query subcommand from spec.
func newPIQueryCommand(global *GlobalOptions, spec piQuerySpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.use,
		Aliases: spec.aliases,
		Short:   spec.short,
		Long:    spec.long,
		Example: spec.example,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return spec.run(ctx, cmd.OutOrStdout())
		},
	}
	if spec.flags != nil {
		spec.flags(cmd)
	}
	return cmd
}

func newPIInfoCommand(global *GlobalOptions) *cobra.Command {
	return newPIQueryCommand(global, piQuerySpec{
		use:     "info",
		aliases: []string{"i"},
		short:   i18n.Text("Show the project summary", "显示项目概览"),
		run: func(ctx *piContext, out io.Writer) error {
			return outputInfo(out, ctx)
		},
	})
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
	// project.Languages is a map, so the codes are sorted to keep the JSON array
	// in a stable order.
	sort.Strings(languages)
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
	return emitLines(out, ctx.global.JSON, info, func(out io.Writer) error {
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
	})
}

func newPIValidateCommand(global *GlobalOptions) *cobra.Command {
	var strict bool
	return newPIQueryCommand(global, piQuerySpec{
		use:     "validate",
		aliases: []string{"v"},
		short:   i18n.Text("Validate the interface", "校验接口"),
		long: i18n.Text(`Checks parsing, name uniqueness, every cross reference, and the declared
files. -st/--strict also fails on advisory findings (a controller this platform
cannot create, a missing resource path or language file).`, `检查解析、名称唯一性、全部交叉引用与声明的文件。-st/--strict 会把提示性问题
（当前平台无法创建的控制器、缺失的资源路径或语言文件）也视为失败。`),
		example: `  maactl pi v -st -j`,
		flags: func(cmd *cobra.Command) {
			cmd.Flags().BoolVarP(&strict, "strict", "s", false, i18n.Text("treat advisory findings as errors", "把提示性问题视为错误"))
		},
		run: func(ctx *piContext, out io.Writer) error {
			report := ctx.project.Validate(pi.ValidateOptions{Strict: strict})
			if err := emitLines(out, ctx.global.JSON, report, func(out io.Writer) error {
				for _, issue := range report.Issues {
					fmt.Fprintf(out, "%s: %s: %s\n", strings.ToUpper(string(issue.Level)), issue.Path, issue.Message)
				}
				if report.OK() {
					fmt.Fprintf(out, "Valid ProjectInterface: %s (%d warning(s))\n", ctx.project.Path, report.Warnings())
				}
				return nil
			}); err != nil {
				return err
			}
			if !report.OK() {
				return exitErrorf(ExitUsage, "%s failed validation: %d error(s), %d warning(s)", ctx.project.Path, report.Errors(), report.Warnings())
			}
			return nil
		},
	})
}

func newPIControllersCommand(global *GlobalOptions) *cobra.Command {
	var typeFilter string
	return newPIQueryCommand(global, piQuerySpec{
		use:     "controllers",
		aliases: []string{"c"},
		short:   i18n.Text("List controllers", "列出控制器"),
		flags: func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&typeFilter, "type", "", i18n.Text("only this controller type", "只显示该类型的控制器"))
		},
		run: func(ctx *piContext, out io.Writer) error {
			return listControllers(out, ctx, typeFilter)
		},
	})
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
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.Name, row.Label, row.Type, yesNo(row.Runnable), fmt.Sprint(row.Options), fmt.Sprint(row.Resources)})
	}
	return emit(out, ctx.global.JSON, rows, headers(
		"name", "名称",
		"label", "显示名称",
		"type", "类型",
		"runnable", "可运行",
		"options", "选项数",
		"resources", "兼容资源数",
	), tableRows)
}

func newPITasksCommand(global *GlobalOptions) *cobra.Command {
	var controllerFilter, resourceFilter, groupFilter string
	var all bool
	return newPIQueryCommand(global, piQuerySpec{
		use:     "tasks",
		aliases: []string{"t"},
		short:   i18n.Text("List tasks", "列出任务"),
		long: i18n.Text(`Lists tasks with their entry node, groups, and option count. Tasks that do not
match the selected controller/resource are hidden unless -all/--all is given.`, `列出任务及其入口节点、分组和选项数量。与所选控制器/资源不匹配的任务默认隐藏，
-all/--all 会列出并说明原因。`),
		example: `  maactl pi t -c Android -r base
  maactl pi t -all -j`,
		flags: func(cmd *cobra.Command) {
			cmd.Flags().StringVarP(&controllerFilter, "controller", "c", "", i18n.Text("filter by controller", "按控制器过滤"))
			cmd.Flags().StringVarP(&resourceFilter, "resource", "r", "", i18n.Text("filter by resource", "按资源过滤"))
			cmd.Flags().StringVar(&groupFilter, "group", "", i18n.Text("filter by group", "按分组过滤"))
			cmd.Flags().BoolVar(&all, "all", false, i18n.Text("include tasks that do not apply, with the reason", "包含不适用的任务并给出原因"))
		},
		run: func(ctx *piContext, out io.Writer) error {
			return listTasks(out, ctx, controllerFilter, resourceFilter, groupFilter, all)
		},
	})
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
		if ctrl == nil && len(ctx.project.Controller) == 1 {
			ctrl = &ctx.project.Controller[0]
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
	headersList := []string{
		i18n.Text("name", "名称"), i18n.Text("label", "显示名称"), i18n.Text("entry", "入口节点"),
		i18n.Text("group", "分组"), i18n.Text("options", "选项数"),
	}
	if all {
		headersList = append(headersList, i18n.Text("unavailable", "不可用原因"))
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := []string{row.Name, row.Label, output.Value(row.Entry), output.Value(pi.Join(row.Group)), fmt.Sprint(row.Options)}
		if all {
			values = append(values, output.Value(strings.Join(row.Unavailable, "; ")))
		}
		tableRows = append(tableRows, values)
	}
	return emit(out, ctx.global.JSON, rows, headersList, tableRows)
}

func newPIGroupsCommand(global *GlobalOptions) *cobra.Command {
	return newPIQueryCommand(global, piQuerySpec{
		use:     "groups",
		aliases: []string{"g"},
		short:   i18n.Text("List task groups", "列出任务分组"),
		run: func(ctx *piContext, out io.Writer) error {
			return listGroups(out, ctx)
		},
	})
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
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.Name, row.Label, yesNo(row.DefaultExpand), fmt.Sprint(row.Tasks)})
	}
	return emit(out, ctx.global.JSON, rows, headers(
		"name", "名称", "label", "显示名称", "default_expand", "默认展开", "tasks", "任务数",
	), tableRows)
}

func newPIPresetsCommand(global *GlobalOptions) *cobra.Command {
	return newPIQueryCommand(global, piQuerySpec{
		use:     "presets",
		aliases: []string{"p"},
		short:   i18n.Text("List presets", "列出预设"),
		run: func(ctx *piContext, out io.Writer) error {
			return listPresets(out, ctx)
		},
	})
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
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.Name, row.Label, fmt.Sprint(len(row.Tasks)), fmt.Sprint(len(row.Disabled))})
	}
	return emit(out, ctx.global.JSON, ctx.project.Preset, headers(
		"name", "名称", "label", "显示名称", "tasks", "启用任务数", "disabled", "禁用任务数",
	), tableRows)
}

func newPISettingsCommand(global *GlobalOptions) *cobra.Command {
	return newPIQueryCommand(global, piQuerySpec{
		use:     "settings",
		aliases: []string{"s"},
		short:   i18n.Text("List setting sections", "列出设置分区"),
		run: func(ctx *piContext, out io.Writer) error {
			return listSettings(out, ctx)
		},
	})
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
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{row.Name, row.Label, yesNo(row.DefaultExpand), output.Value(pi.Join(row.Option))})
	}
	return emit(out, ctx.global.JSON, rows, headers(
		"name", "名称", "label", "显示名称", "default_expand", "默认展开", "options", "选项",
	), tableRows)
}

// headers builds a localized header row from "en, zh" pairs.
func headers(pairs ...string) []string {
	out := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, i18n.Text(pairs[i], pairs[i+1]))
	}
	return out
}

// yesNo renders a boolean for a table cell.
func yesNo(value bool) string {
	if value {
		return i18n.Text("yes", "是")
	}
	return i18n.Text("no", "否")
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
