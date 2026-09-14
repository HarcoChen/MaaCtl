// Package help renders maactl's compact help layout: one-line summaries, short
// aliases, flags grouped by purpose, and a single example block.
package help

import (
	"fmt"
	"io"
	"strings"

	"maactl/internal/i18n"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Help renderer annotations. They let the renderer group flags by purpose and
// show the multi-letter aliases that pflag cannot carry as shorthands.
const (
	// AliasAnnotation holds a short alias such as "if" for "--interface".
	AliasAnnotation = "maactl.help.alias"
	// SectionAnnotation holds the flag's group key.
	SectionAnnotation = "maactl.help.section"
)

// Flag groups, in render order. An unannotated flag goes into sectionPlain.
const (
	sectionPlain     = ""
	SectionShortcut  = "shortcut"
	SectionTarget    = "target"
	SectionOptions   = "options"
	SectionResources = "resources"
	SectionOutput    = "output"
	SectionControl   = "control"
)

// sectionOrder is the order flag groups are printed in.
var sectionOrder = []string{
	sectionPlain,
	SectionShortcut,
	SectionTarget,
	SectionOptions,
	SectionResources,
	SectionOutput,
	SectionControl,
}

// sectionTitle returns the localized heading for a flag group.
func sectionTitle(section string) string {
	switch section {
	case SectionShortcut:
		return i18n.Text("Shortcuts:", "快捷方式：")
	case SectionTarget:
		return i18n.Text("Target:", "目标选择：")
	case SectionOptions:
		return i18n.Text("Options:", "配置项与覆盖：")
	case SectionResources:
		return i18n.Text("Resources:", "资源：")
	case SectionOutput:
		return i18n.Text("Output:", "输出：")
	case SectionControl:
		return i18n.Text("Control:", "运行控制：")
	default:
		return i18n.Text("Flags:", "选项：")
	}
}

// MarkFlagsSection assigns flags to a help section.
func MarkFlagsSection(flags *pflag.FlagSet, section string, names ...string) {
	for _, name := range names {
		if flag := flags.Lookup(name); flag != nil {
			_ = flags.SetAnnotation(name, SectionAnnotation, []string{section})
		}
	}
}

// MarkFlagAlias records the multi-letter alias of a flag so help can print it.
func MarkFlagAlias(flag *pflag.Flag, alias string) {
	if flag == nil || alias == "" {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[AliasAnnotation] = []string{alias}
}

// FlagAlias returns the multi-letter alias of a flag, or "".
func FlagAlias(flag *pflag.Flag) string {
	for _, value := range flag.Annotations[AliasAnnotation] {
		return value
	}
	return ""
}

// SetFullHelp replaces cobra's default help with the compact layout.
func SetFullHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		renderHelp(cmd, cmd.OutOrStdout())
	})
}

func renderHelp(cmd *cobra.Command, out io.Writer) {
	var b strings.Builder
	writeTitle(&b, cmd)
	writeUsage(&b, cmd)
	writeCommands(&b, cmd)
	writeFlags(&b, cmd)
	writeExample(&b, cmd)
	writeFooter(&b, cmd)
	_, _ = io.WriteString(out, b.String())
}

// writeTitle prints "name (aliases): summary" followed by the long description
// only when it adds something the summary does not.
func writeTitle(b *strings.Builder, cmd *cobra.Command) {
	label := cmd.Name()
	if len(cmd.Aliases) > 0 {
		label += " (" + strings.Join(cmd.Aliases, ", ") + ")"
	}
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	if short == "" && long == "" {
		return
	}
	if short == "" {
		fmt.Fprintf(b, "%s: %s\n", label, long)
		return
	}
	fmt.Fprintf(b, "%s: %s\n", label, short)
	if long != "" && long != short {
		b.WriteString("\n")
		b.WriteString(long)
		b.WriteString("\n")
	}
}

func writeUsage(b *strings.Builder, cmd *cobra.Command) {
	writeHeading(b, i18n.Text("Usage:", "用法："))
	if cmd == cmd.Root() {
		fmt.Fprintf(b, "  %s <command> [flags]\n", cmd.CommandPath())
		return
	}
	fmt.Fprintf(b, "  %s\n", cmd.UseLine())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(b, "  %s <command> [flags]\n", cmd.CommandPath())
	}
}

// writeCommands lists the child commands with their aliases, one level deep.
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
	labels := make([]string, len(children))
	descriptions := make([]string, len(children))
	width := 0
	for i, child := range children {
		labels[i] = commandLabel(child)
		descriptions[i] = child.Short
		if len(labels[i]) > width {
			width = len(labels[i])
		}
	}
	writeHeading(b, i18n.Text("Commands:", "命令："))
	for i := range children {
		fmt.Fprintf(b, "  %-*s  %s\n", width, labels[i], descriptions[i])
	}
}

// commandLabel formats "name, alias <argument>" for the command list.
func commandLabel(cmd *cobra.Command) string {
	label := cmd.Name()
	if len(cmd.Aliases) > 0 {
		label += ", " + strings.Join(cmd.Aliases, ", ")
	}
	if rest := strings.TrimSpace(strings.TrimPrefix(cmd.Use, cmd.Name())); rest != "" {
		label += " " + rest
	}
	return label
}

// writeFlags groups every flag this command accepts by purpose, then prints the
// global flags. Shared flags therefore look the same in a group's help and in a
// subcommand's help.
func writeFlags(b *strings.Builder, cmd *cobra.Command) {
	groups := map[string][]*pflag.Flag{}
	seen := map[*pflag.Flag]bool{}
	var global []*pflag.Flag
	collect := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" || seen[f] {
				return
			}
			seen[f] = true
			if isGlobalFlag(cmd, f) {
				global = append(global, f)
				return
			}
			section := flagSection(f)
			groups[section] = append(groups[section], f)
		})
	}
	collect(cmd.NonInheritedFlags())
	collect(cmd.InheritedFlags())

	for _, section := range sectionOrder {
		writeFlagGroup(b, sectionTitle(section), groups[section])
	}
	global = append(global, syntheticHelpFlag())
	writeFlagGroup(b, i18n.Text("Global:", "全局选项："), global)
}

func isGlobalFlag(cmd *cobra.Command, f *pflag.Flag) bool {
	root := cmd.Root()
	if root.PersistentFlags().Lookup(f.Name) == f {
		return true
	}
	return cmd == root && root.Flags().Lookup(f.Name) == f
}

func flagSection(f *pflag.Flag) string {
	for _, value := range f.Annotations[SectionAnnotation] {
		return value
	}
	return sectionPlain
}

// writeFlagGroup prints a flag group with a fixed label column.
func writeFlagGroup(b *strings.Builder, title string, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}
	labels := make([]string, len(flags))
	usages := make([]string, len(flags))
	width := 0
	for i, f := range flags {
		labels[i] = flagLabel(f)
		usages[i] = flagUsage(f)
		if len(labels[i]) > width {
			width = len(labels[i])
		}
	}
	const maxWidth = 34
	if width > maxWidth {
		width = maxWidth
	}
	writeHeading(b, title)
	for i := range flags {
		fmt.Fprintf(b, "  %-*s  %s\n", width, labels[i], usages[i])
	}
}

// flagLabel renders "-s, -alias, --long <type>".
func flagLabel(f *pflag.Flag) string {
	var names []string
	if f.Shorthand != "" {
		names = append(names, "-"+f.Shorthand)
	}
	if alias := FlagAlias(f); alias != "" {
		names = append(names, "-"+alias)
	}
	names = append(names, "--"+f.Name)
	label := strings.Join(names, ", ")
	if f.NoOptDefVal == "" {
		label += " <" + f.Value.Type() + ">"
	}
	return label
}

// flagUsage renders the description plus the default when it is meaningful.
func flagUsage(f *pflag.Flag) string {
	return f.Usage + defaultSuffix(f)
}

// defaultSuffix omits zero defaults so the help stays short, and quotes string
// defaults the way pflag does.
func defaultSuffix(f *pflag.Flag) string {
	def := f.DefValue
	switch def {
	case "", "false", "0", "0s", "0.00", "[]", "[]string{}":
		return ""
	}
	if f.Value.Type() == "string" {
		def = `"` + def + `"`
	}
	return i18n.Text(" (default "+def+")", "（默认 "+def+"）")
}

func syntheticHelpFlag() *pflag.Flag {
	flags := pflag.NewFlagSet("help", pflag.ContinueOnError)
	flags.BoolP("help", "h", false, i18n.Text("show help", "显示帮助"))
	return flags.Lookup("help")
}

func writeExample(b *strings.Builder, cmd *cobra.Command) {
	if strings.TrimSpace(cmd.Example) == "" {
		return
	}
	writeHeading(b, i18n.Text("Example:", "示例："))
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
	b.WriteString("\n")
	if cmd == cmd.Root() {
		fmt.Fprintln(b, i18n.Text(`Use "maactl <command> -h" for command details.`, `使用 "maactl <command> -h" 查看命令细节。`))
		return
	}
	fmt.Fprintf(b, i18n.Text("Use %q for command details.\n", "使用 %q 查看命令细节。\n"), cmd.CommandPath()+" <command> -h")
}

func writeHeading(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}
