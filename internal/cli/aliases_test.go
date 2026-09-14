package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestNormalizeArgsRewritesAliases covers the multi-letter short flags that
// pflag cannot register as shorthands.
func TestNormalizeArgsRewritesAliases(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"separate value", []string{"-if", "D:\\proj"}, []string{"--interface", "D:\\proj"}},
		{"equals value", []string{"-if=D:\\proj"}, []string{"--interface=D:\\proj"}},
		{"boolean", []string{"-all"}, []string{"--all"}},
		{"short flag untouched", []string{"-j"}, []string{"-j"}},
		{"long flag untouched", []string{"--interface", "x"}, []string{"--interface", "x"}},
		{"bare dash untouched", []string{"-"}, []string{"-"}},
		{"unknown alias untouched", []string{"-nope"}, []string{"-nope"}},
		{"subcommand untouched", []string{"pi", "tasks"}, []string{"pi", "tasks"}},
		{"value that looks like a flag", []string{"-if", "-weird"}, []string{"--interface", "-weird"}},
		{
			"after double dash nothing changes",
			[]string{"run", "node", "--", "-if", "x"},
			[]string{"run", "node", "--", "-if", "x"},
		},
		{
			"several aliases",
			[]string{"pi", "t", "-if", "x", "-all", "-j"},
			[]string{"pi", "t", "--interface", "x", "--all", "-j"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeArgs(tc.in)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("NormalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestFlagAliasesAreUnique guards the normalization table: a duplicate alias
// would silently redirect one flag to another.
func TestFlagAliasesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, alias := range flagAliases {
		if len(alias.short) == 0 {
			t.Errorf("alias for --%s is empty", alias.long)
		}
		if previous, ok := seen[alias.short]; ok {
			t.Errorf("alias %q is used by both --%s and --%s", alias.short, previous, alias.long)
		}
		seen[alias.short] = alias.long
	}
}

// TestEveryFlagHasAShortForm checks the promise that every flag can be reached
// with a short flavor: either a one-letter shorthand or a 2-3 letter alias.
func TestEveryFlagHasAShortForm(t *testing.T) {
	root := NewRootCommand("test")
	walkCommands(root, func(cmd *cobra.Command) {
		// The generated completion subtree is cobra's, not ours.
		if strings.Contains(cmd.CommandPath(), "completion") {
			return
		}
		cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" || flag.Name == "version" {
				return
			}
			if flag.Shorthand != "" || FlagAliasOf(flag.Name) != "" {
				return
			}
			t.Errorf("command %q flag --%s has no short form", cmd.CommandPath(), flag.Name)
		})
	})
}
