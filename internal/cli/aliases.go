package cli

import (
	"strings"

	"maactl/internal/help"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagAlias maps a multi-letter short alias onto the long flag name. Single
// letters are registered with pflag as shorthands; everything here exists
// because pflag shorthands are limited to one character.
//
// Aliases must be unique across the whole command tree, because arguments are
// normalized before cobra parses them.
type flagAlias struct {
	short string
	long  string
}

// flagAliases is the single source of truth for both argument normalization and
// the alias shown in --help.
var flagAliases = []flagAlias{
	// Global flags.
	{"if", "interface"},
	{"lib", "lib-dir"},
	{"lg", "lang"},
	{"cfg", "config"},
	{"nocfg", "no-config"},
	{"log", "log-dir"},
	{"vb", "verbose"},

	// Project queries.
	{"ty", "type"},
	{"st", "strict"},
	{"all", "all"},
	{"gr", "group"},

	// Resource queries.
	{"pa", "path"},
	{"ol", "overlay"},
	{"vf", "verify"},

	// Run target.
	{"nm", "name"},
	{"ap", "adb-path"},
	{"wh", "win32-handle"},
	{"wc", "win32-class"},
	{"ww", "win32-window"},
	{"ws", "win32-screencap"},
	{"wm", "win32-mouse"},
	{"wk", "win32-keyboard"},
	{"gt", "gamepad-type"},

	// Run options and overrides.
	{"opt", "option"},
	{"of", "option-file"},
	{"ovf", "override-file"},

	// Platform-specific targets: macOS, PlayCover, and Linux controllers.
	{"mw", "macos-window"},
	{"mid", "macos-window-id"},
	{"ms", "macos-screencap"},
	{"mi", "macos-input"},
	{"pca", "playcover-address"},
	{"pcu", "playcover-uuid"},
	{"ls", "linux-socket"},
	{"lv", "linux-vk"},

	// Run control.
	{"fd", "focus-display"},
	{"dr", "dry-run"},
	{"to", "timeout"},
	{"sa", "stop-after"},
	{"sto", "stop-timeout"},
	{"rh", "require-resource-hash"},
	{"na", "no-agent"},
	{"al", "agent-log"},
	{"coe", "continue-on-error"},
}

// aliasToLong is flagAliases indexed for lookup.
var aliasToLong = func() map[string]string {
	table := make(map[string]string, len(flagAliases))
	for _, alias := range flagAliases {
		table[alias.short] = alias.long
	}
	return table
}()

// FlagAliasOf returns the multi-letter alias registered for a long flag name,
// or "".
func FlagAliasOf(long string) string {
	for _, alias := range flagAliases {
		if alias.long == long {
			return alias.short
		}
	}
	return ""
}

// NormalizeArgs rewrites multi-letter short flags (`-if value`, `-if=value`)
// into their long form so cobra's single-letter shorthand parser never sees
// them. Everything after a bare `--` is passed through untouched.
func NormalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			out = append(out, arg)
			continue
		}
		name := arg[1:]
		value := ""
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value = name[eq:]
			name = name[:eq]
		}
		long, ok := aliasToLong[name]
		if !ok {
			out = append(out, arg)
			continue
		}
		out = append(out, "--"+long+value)
	}
	return out
}

// applyFlagAliases annotates every flag that has a multi-letter alias, so help
// can print it next to the long name.
func applyFlagAliases(root *cobra.Command) {
	walkCommands(root, func(cmd *cobra.Command) {
		cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
			if alias := FlagAliasOf(flag.Name); alias != "" {
				help.MarkFlagAlias(flag, alias)
			}
		})
	})
}

// walkCommands visits every command in the tree.
func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, fn)
	}
}
