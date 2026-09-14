package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Help renderer annotations. They let the renderer group planned features and
// shared execution flags without parsing description text.
const (
	plannedAnnotation = "maactl.planned"
	sectionAnnotation = "maactl.help.section"

	sectionAction    = "action"
	sectionExecution = "execution"
)

// setFullHelp replaces cobra's default help with a compact layout:
// command descriptions are always shown, flags are grouped by owner, and
// planned features are listed separately.
func setFullHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		renderHelp(cmd, cmd.OutOrStdout())
	})
}

func markCommandPlanned(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[plannedAnnotation] = "true"
}

func markFlagsPlanned(flags *pflag.FlagSet, names ...string) {
	for _, name := range names {
		_ = flags.SetAnnotation(name, plannedAnnotation, []string{"true"})
	}
}

func markFlagsSection(flags *pflag.FlagSet, section string, names ...string) {
	for _, name := range names {
		_ = flags.SetAnnotation(name, sectionAnnotation, []string{section})
	}
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
	writeHeading(b, "Usage:")
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
		if commandPlanned(child) {
			descriptions[i] = strings.TrimSpace(descriptions[i] + " (planned)")
		}
		if len(names[i]) > width {
			width = len(names[i])
		}
	}
	writeHeading(b, "Commands:")
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
			case sectionAction:
				ownActions = append(ownActions, f)
			case sectionExecution:
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

	writeFlagGroup(b, "Flags:", ownFlags)
	writeFlagGroup(b, "Actions:", ownActions)
	writeFlagGroup(b, "Execution Flags (shared with subcommands):", ownExecution)
	for _, owner := range inheritedOrder {
		var flags, execution []*pflag.Flag
		for _, f := range inherited[owner] {
			if flagSection(f) == sectionExecution {
				execution = append(execution, f)
			} else {
				flags = append(flags, f)
			}
		}
		writeFlagGroup(b, fmt.Sprintf("Inherited Flags (from %q):", owner.CommandPath()), flags)
		writeFlagGroup(b, fmt.Sprintf("Execution Flags (inherited from %q):", owner.CommandPath()), execution)
	}
	writeFlagGroup(b, "Planned Flags:", planned)
	writeFlagGroup(b, "Global Flags:", append(global, syntheticHelpFlag()))
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
	for _, value := range f.Annotations[plannedAnnotation] {
		if value == "true" {
			return true
		}
	}
	return false
}

func flagSection(f *pflag.Flag) string {
	for _, value := range f.Annotations[sectionAnnotation] {
		return value
	}
	return ""
}

func commandPlanned(cmd *cobra.Command) bool {
	return cmd.Annotations[plannedAnnotation] == "true"
}

func syntheticHelpFlag() *pflag.Flag {
	flags := pflag.NewFlagSet("help", pflag.ContinueOnError)
	flags.BoolP("help", "h", false, "show help")
	return flags.Lookup("help")
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
	b.WriteString(set.FlagUsages())
}

func writeExamples(b *strings.Builder, cmd *cobra.Command) {
	if strings.TrimSpace(cmd.Example) == "" {
		return
	}
	writeHeading(b, "Examples:")
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
	fmt.Fprintf(b, "Use %q for more information about a command.\n", target)
}

func writeHeading(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}
