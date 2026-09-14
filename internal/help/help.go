// Package help renders the compact maactl help layout: command descriptions,
// grouped flags, and separately listed planned features.
package help

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"maactl/internal/i18n"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Help renderer annotations. They let the renderer group planned features and
// shared execution flags without parsing description text.
const (
	PlannedAnnotation = "maactl.planned"
	SectionAnnotation = "maactl.help.section"

	// SectionAction groups action-selecting flags such as interface --show.
	SectionAction = "action"
	// SectionExecution groups execution flags shared by run and its subcommands.
	SectionExecution = "execution"
)

// SetFullHelp replaces cobra's default help with a compact layout:
// command descriptions are always shown, flags are grouped by owner, and
// planned features are listed separately.
func SetFullHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		renderHelp(cmd, cmd.OutOrStdout())
	})
}

// MarkCommandPlanned flags a command as published but not implemented yet.
func MarkCommandPlanned(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[PlannedAnnotation] = "true"
}

// MarkFlagsPlanned flags flags as published but not implemented yet.
func MarkFlagsPlanned(flags *pflag.FlagSet, names ...string) {
	for _, name := range names {
		_ = flags.SetAnnotation(name, PlannedAnnotation, []string{"true"})
	}
}

// MarkFlagsSection assigns flags to a help section.
func MarkFlagsSection(flags *pflag.FlagSet, section string, names ...string) {
	for _, name := range names {
		_ = flags.SetAnnotation(name, SectionAnnotation, []string{section})
	}
}

// CommandPlanned reports whether a command is marked as planned.
func CommandPlanned(cmd *cobra.Command) bool {
	return cmd.Annotations[PlannedAnnotation] == "true"
}

func renderHelp(cmd *cobra.Command, out io.Writer) {
	var b strings.Builder
	writeDescription(&b, cmd)
	writeUsage(&b, cmd)
	writeCommands(&b, cmd)
	writeFlags(&b, cmd)
	writeExamples(&b, cmd)
	writeFooter(&b, cmd)
	_, _ = io.WriteString(out, b.String())
}

func writeDescription(b *strings.Builder, cmd *cobra.Command) {
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	switch {
	case short == "" && long == "":
		return
	case short == "":
		b.WriteString(long + "\n")
	case long == "" || long == short:
		b.WriteString(short + "\n")
	default:
		b.WriteString(short + "\n\n" + long + "\n")
	}
}

func writeUsage(b *strings.Builder, cmd *cobra.Command) {
	writeHeading(b, i18n.Text("Usage:", "用法："))
	if cmd == cmd.Root() {
		fmt.Fprintf(b, "  %s [command]\n", cmd.CommandPath())
		return
	}
	fmt.Fprintf(b, "  %s\n", cmd.UseLine())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(b, "  %s [command]\n", cmd.CommandPath())
	}
}

// writeCommands lists available child commands one level deep. Deeper commands
// are only shown when their parent help is requested.
func writeCommands(b *strings.Builder, cmd *cobra.Command) {
	var children []*cobra.Command
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			children = append(children, child)
		}
	}
	if len(children) == 0 {
		return
	}
	names := make([]string, len(children))
	descriptions := make([]string, len(children))
	width := 0
	for i, child := range children {
		names[i] = commandDisplayName(child)
		descriptions[i] = child.Short
		if CommandPlanned(child) {
			descriptions[i] = strings.TrimSpace(descriptions[i] + i18n.Text(" (planned)", "（计划中）"))
		}
		if len(names[i]) > width {
			width = len(names[i])
		}
	}
	writeHeading(b, i18n.Text("Commands:", "命令："))
	for i := range children {
		fmt.Fprintf(b, "  %-*s   %s\n", width, names[i], descriptions[i])
	}
}

func commandDisplayName(cmd *cobra.Command) string {
	name := cmd.Name()
	rest := strings.TrimSpace(strings.TrimPrefix(cmd.Use, name))
	if rest != "" {
		return name + " " + rest
	}
	return name
}

// writeFlags renders flag sections in a stable order: command flags, shared
// execution flags (once), inherited flags with their source, planned flags,
// then root-level global flags.
func writeFlags(b *strings.Builder, cmd *cobra.Command) {
	root := cmd.Root()
	var ownFlags, ownActions, ownExecution, planned, global []*pflag.Flag
	inherited := map[*cobra.Command][]*pflag.Flag{}
	var inheritedOrder []*cobra.Command
	seen := map[*pflag.Flag]bool{}

	add := func(f *pflag.Flag, isOwn bool) {
		if f.Name == "help" || seen[f] {
			return
		}
		seen[f] = true
		switch {
		case isGlobalFlag(cmd, root, f):
			global = append(global, f)
		case flagPlanned(f):
			planned = append(planned, f)
		case isOwn:
			switch flagSection(f) {
			case SectionAction:
				ownActions = append(ownActions, f)
			case SectionExecution:
				ownExecution = append(ownExecution, f)
			default:
				ownFlags = append(ownFlags, f)
			}
		default:
			owner := flagOwner(cmd, f)
			if _, ok := inherited[owner]; !ok {
				inheritedOrder = append(inheritedOrder, owner)
			}
			inherited[owner] = append(inherited[owner], f)
		}
	}
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) { add(f, true) })
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) { add(f, false) })

	writeFlagGroup(b, i18n.Text("Flags:", "选项："), ownFlags)
	writeFlagGroup(b, i18n.Text("Actions:", "操作："), ownActions)
	writeFlagGroup(b, i18n.Text("Execution Flags (shared with subcommands):", "执行选项（与子命令共用）："), ownExecution)
	for _, owner := range inheritedOrder {
		var flags, execution []*pflag.Flag
		for _, f := range inherited[owner] {
			if flagSection(f) == SectionExecution {
				execution = append(execution, f)
			} else {
				flags = append(flags, f)
			}
		}
		writeFlagGroup(b, fmt.Sprintf(i18n.Text("Inherited Flags (from %q):", "继承选项（来自 %q）："), owner.CommandPath()), flags)
		writeFlagGroup(b, fmt.Sprintf(i18n.Text("Execution Flags (inherited from %q):", "执行选项（继承自 %q）："), owner.CommandPath()), execution)
	}
	writeFlagGroup(b, i18n.Text("Planned Flags:", "计划中的选项："), planned)
	writeFlagGroup(b, i18n.Text("Global Flags:", "全局选项："), append(global, syntheticHelpFlag()))
}

func isGlobalFlag(cmd, root *cobra.Command, f *pflag.Flag) bool {
	if root.PersistentFlags().Lookup(f.Name) == f {
		return true
	}
	return cmd == root && root.Flags().Lookup(f.Name) == f
}

func flagOwner(cmd *cobra.Command, f *pflag.Flag) *cobra.Command {
	for c := cmd.Parent(); c != nil; c = c.Parent() {
		if c.PersistentFlags().Lookup(f.Name) == f {
			return c
		}
	}
	return cmd.Root()
}

func flagPlanned(f *pflag.Flag) bool {
	for _, value := range f.Annotations[PlannedAnnotation] {
		if value == "true" {
			return true
		}
	}
	return false
}

func flagSection(f *pflag.Flag) string {
	for _, value := range f.Annotations[SectionAnnotation] {
		return value
	}
	return ""
}

func syntheticHelpFlag() *pflag.Flag {
	flags := pflag.NewFlagSet("help", pflag.ContinueOnError)
	flags.BoolP("help", "h", false, i18n.Text("show help", "显示帮助"))
	return flags.Lookup("help")
}

// defaultSuffixPattern matches pflag's line-ending "(default ...)" annotation.
var defaultSuffixPattern = regexp.MustCompile(` \(default (.*)\)$`)

// localizeFlagUsages translates pflag's generated default annotations so
// Chinese help does not mix in English "(default ...)" suffixes.
func localizeFlagUsages(usages string) string {
	if i18n.Active() != i18n.ZH {
		return usages
	}
	lines := strings.Split(usages, "\n")
	for i, line := range lines {
		lines[i] = defaultSuffixPattern.ReplaceAllString(line, "（默认 $1）")
	}
	return strings.Join(lines, "\n")
}

func writeFlagGroup(b *strings.Builder, title string, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}
	set := pflag.NewFlagSet(title, pflag.ContinueOnError)
	for _, f := range flags {
		set.AddFlag(f)
	}
	writeHeading(b, title)
	b.WriteString(localizeFlagUsages(set.FlagUsages()))
}

func writeExamples(b *strings.Builder, cmd *cobra.Command) {
	if strings.TrimSpace(cmd.Example) == "" {
		return
	}
	writeHeading(b, i18n.Text("Examples:", "示例："))
	for _, line := range strings.Split(cmd.Example, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			fmt.Fprintf(b, "  %s\n", line)
		}
	}
}

func writeFooter(b *strings.Builder, cmd *cobra.Command) {
	if !cmd.HasAvailableSubCommands() {
		return
	}
	target := "maactl <command> --help"
	if cmd != cmd.Root() {
		target = cmd.CommandPath() + " <command> --help"
	}
	b.WriteString("\n")
	fmt.Fprintf(b, i18n.Text("Use %q for more information about a command.\n", "使用 %q 查看某个命令的更多信息。\n"), target)
}

func writeHeading(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}
