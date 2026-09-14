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

func newPIOptionsCommand(global *GlobalOptions) *cobra.Command {
	var taskName, controllerFilter, resourceFilter string
	var all bool
	cmd := &cobra.Command{
		Use:     "options",
		Aliases: []string{"o"},
		Short:   i18n.Text("Show the option tree", "显示配置项树"),
		Long: i18n.Text(`Shows which options are active and in which order they merge:
global_option → resource.option → controller.option → task.option, including
the nested options activated by the selected cases.

-all/--all also lists options that do not apply here and the options no layer
references.`, `显示哪些配置项会被激活，以及合并顺序：
global_option → resource.option → controller.option → task.option，
并展开被选中 case 激活的子配置项。

-all/--all 还会列出当前不适用的配置项，以及没有任何层引用的配置项。`),
		Example: `  maactl pi o -if D:\MaaMio
  maactl pi o -t 常规作战 -c Windows -r Official
  maactl pi o -all -j`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listOptions(cmd.OutOrStdout(), ctx, controllerFilter, resourceFilter, taskName, all)
		},
	}
	cmd.Flags().StringVarP(&controllerFilter, "controller", "c", "", i18n.Text("controller to check applicability against", "用于判断适用性的控制器"))
	cmd.Flags().StringVarP(&resourceFilter, "resource", "r", "", i18n.Text("resource to check applicability against", "用于判断适用性的资源"))
	cmd.Flags().StringVarP(&taskName, "task", "t", "", i18n.Text("include the options this task references", "包含该任务引用的配置项"))
	cmd.Flags().BoolVar(&all, "all", false, i18n.Text("include inactive and unreferenced options", "包含不适用和未被引用的配置项"))
	return cmd
}

// optionPlanRow is one row of `pi options`.
type optionPlanRow struct {
	Layer       string   `json:"layer"`
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type"`
	Parent      string   `json:"parent,omitempty"`
	Depth       int      `json:"depth"`
	Active      bool     `json:"active"`
	Reason      string   `json:"reason,omitempty"`
	Cases       []string `json:"cases,omitempty"`
	Default     any      `json:"default,omitempty"`
	Fields      []string `json:"fields,omitempty"`
	MinCount    *int     `json:"min_count,omitempty"`
	MaxCount    *int     `json:"max_count,omitempty"`
	Controllers []string `json:"controllers,omitempty"`
	Resources   []string `json:"resources,omitempty"`
}

func listOptions(out io.Writer, ctx *piContext, controllerFilter, resourceFilter, taskName string, all bool) error {
	controllerName, resourceName, task, err := resolveOptionScope(ctx, controllerFilter, resourceFilter, taskName)
	if err != nil {
		return err
	}
	entries := ctx.project.OptionPlan(controllerName, resourceName, task, all)
	rows := make([]optionPlanRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, describeOption(ctx, entry))
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	for i, row := range rows {
		if i == 0 || row.Layer != rows[i-1].Layer {
			fmt.Fprintf(out, "[%s]\n", row.Layer)
		}
		fmt.Fprintln(out, renderOptionRow(row))
	}
	return nil
}

// resolveOptionScope picks the controller/resource/task that applicability and
// the option tree are computed against, applying the same "only when
// unambiguous" rule as run.
func resolveOptionScope(ctx *piContext, controllerFilter, resourceFilter, taskName string) (string, string, *pi.Task, error) {
	project := ctx.project
	controllerName := strings.TrimSpace(controllerFilter)
	if controllerName == "" && len(project.Controller) == 1 {
		controllerName = project.Controller[0].Name
	}
	var ctrl *pi.Controller
	if controllerName != "" {
		resolved, err := project.FindController(controllerName)
		if err != nil {
			return "", "", nil, withExitCode(ExitUsage, err)
		}
		ctrl = resolved
	}
	resourceName := strings.TrimSpace(resourceFilter)
	if resourceName == "" && ctrl != nil && len(project.Resource) == 1 {
		resourceName = project.Resource[0].Name
	}
	if resourceName != "" {
		if _, err := project.FindResource(resourceName, ctrl); err != nil {
			return "", "", nil, withExitCode(ExitUsage, err)
		}
	}
	var task *pi.Task
	if strings.TrimSpace(taskName) != "" {
		found := project.LookupTask(taskName, ctx.global.Language())
		if found == nil {
			return "", "", nil, exitErrorf(ExitUsage, "task %q not found", taskName)
		}
		task = found
	}
	return controllerName, resourceName, task, nil
}

// describeOption converts a plan entry into its display/JSON form.
func describeOption(ctx *piContext, entry pi.OptionPlanEntry) optionPlanRow {
	row := optionPlanRow{
		Layer:  string(entry.Layer),
		Name:   entry.Name,
		Parent: entry.Parent,
		Depth:  entry.Depth,
		Active: entry.Active,
		Reason: entry.Reason,
	}
	option := entry.Option
	if option == nil {
		return row
	}
	row.Label = ctx.label(option.Label, "")
	row.Type = string(option.Kind())
	row.Controllers = option.Controller
	row.Resources = option.Resource
	row.MinCount = option.MinCount
	row.MaxCount = option.MaxCount
	for _, item := range option.Cases {
		row.Cases = append(row.Cases, item.Name)
	}
	if def := option.DefaultCase; def != nil {
		if def.List != nil {
			row.Default = def.List
		} else {
			row.Default = def.Scalar
		}
	}
	for _, field := range option.Inputs {
		row.Fields = append(row.Fields, field.Name)
	}
	for _, field := range option.Hotkeys {
		row.Fields = append(row.Fields, field.Name)
	}
	return row
}

// renderOptionRow writes one indented option line.
func renderOptionRow(row optionPlanRow) string {
	indent := strings.Repeat("  ", row.Depth)
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "%s%s  [%s]", indent, row.Name, row.Type)
	if row.Label != "" && row.Label != row.Name {
		fmt.Fprintf(builder, "  %s", row.Label)
	}
	if len(row.Cases) > 0 {
		fmt.Fprintf(builder, "  cases=%s", pi.Join(row.Cases))
	}
	if row.Default != nil {
		fmt.Fprintf(builder, "  default=%v", row.Default)
	}
	if len(row.Fields) > 0 {
		fmt.Fprintf(builder, "  fields=%s", pi.Join(row.Fields))
	}
	if row.MinCount != nil || row.MaxCount != nil {
		fmt.Fprintf(builder, "  count=%s..%s", optionalInt(row.MinCount), optionalInt(row.MaxCount))
	}
	if len(row.Controllers) > 0 {
		fmt.Fprintf(builder, "  controllers=%s", pi.Join(row.Controllers))
	}
	if len(row.Resources) > 0 {
		fmt.Fprintf(builder, "  resources=%s", pi.Join(row.Resources))
	}
	if !row.Active {
		fmt.Fprintf(builder, "  (unavailable: %s)", row.Reason)
	}
	return builder.String()
}

func optionalInt(value *int) string {
	if value == nil {
		return "*"
	}
	return fmt.Sprint(*value)
}

// sortedKeys returns map keys in a stable order; used by config reporting.
func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
