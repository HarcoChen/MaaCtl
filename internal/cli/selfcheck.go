package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/platform"

	"github.com/spf13/cobra"
)

// newSelfCheckCommand builds the hidden `selfcheck` command: it loads
// MaaFramework from wherever this build would load it from and reports the
// version it found. It is the shortest way to tell "the runtime is missing or
// broken" apart from "the project or the device is wrong", and CI runs it for
// every platform to prove that a packaged runtime actually loads.
func newSelfCheckCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:    "selfcheck",
		Hidden: true,
		Short:  i18n.Text("Check that the MaaFramework runtime loads", "检查 MaaFramework 运行库能否加载"),
		Long: i18n.Text(`Loads MaaFramework the way every other command does—from --lib-dir, then bundled libraries,
else ./maafw/bin—and prints the version that was loaded. Fails with a non-zero
exit code when the libraries cannot be loaded at all.`, `按其它命令相同的方式加载 MaaFramework：先看 --lib-dir，其次是自带运行库，
最后是 ./maafw/bin。打印实际加载到的版本；无法加载时以非零退出码失败。`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			version, libDir, err := maafw.RuntimeVersion(global.LibDir, global.LogDir)
			if err != nil {
				return withExitCode(ExitInternal, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "MaaFramework %s (%s, %s) from %s\n",
				version, platform.Host().ID(), librarySource(global.LibDir, libDir), filepath.Clean(libDir))
			return nil
		},
	}
}

// librarySource names where the runtime was loaded from. It compares the
// resolved directory against the embedded payload's cache directory, so a
// bundled build that fell back to ./maafw/bin is reported as "local" instead
// of claiming the version it carries.
func librarySource(explicit, resolved string) string {
	switch {
	case strings.TrimSpace(explicit) != "":
		return "--lib-dir"
	case resolved != "" && resolved == maafw.BundledLibDir():
		return "bundled"
	default:
		return "local"
	}
}
