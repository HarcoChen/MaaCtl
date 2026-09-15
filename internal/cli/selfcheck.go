package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/platform"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
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
		Long: i18n.Text(`Loads MaaFramework the way every other command does—bundled libraries, --lib-dir,
or ./maafw/bin—and prints the version that was loaded. Fails with a non-zero
exit code when the libraries cannot be loaded at all.`, `按其它命令相同的方式加载 MaaFramework —— 自带运行库、--lib-dir 或 ./maafw/bin ——
并打印实际加载到的版本。无法加载时以非零退出码失败。`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			libDir, err := maafw.ResolveLibDir(global.LibDir)
			if err != nil {
				return withExitCode(ExitInternal, err)
			}
			if err := maafw.Init(libDir, global.LogDir); err != nil {
				return withExitCode(ExitInternal, fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err))
			}
			defer func() { _ = maa.Release() }()
			fmt.Fprintf(cmd.OutOrStdout(), "MaaFramework %s (%s, %s) from %s\n",
				maafw.Version(), platform.Host().ID(), librarySource(global.LibDir), filepath.Clean(libDir))
			return nil
		},
	}
}

// librarySource names where the runtime was loaded from.
func librarySource(explicit string) string {
	switch {
	case strings.TrimSpace(explicit) != "":
		return "--lib-dir"
	case maafw.BundledVersion() != "":
		return "bundled"
	default:
		return "local"
	}
}
