package tests

import (
	"path/filepath"
	"runtime"
	"testing"

	"maactl/internal/platform"
)

func TestPlatformNamesLibraries(t *testing.T) {
	cases := map[string]struct {
		target    platform.Target
		framework string
		toolkit   string
	}{
		"windows": {platform.Target{OS: platform.Windows, Arch: platform.AMD64}, "MaaFramework.dll", "MaaToolkit.dll"},
		"linux":   {platform.Target{OS: platform.Linux, Arch: platform.AArch64}, "libMaaFramework.so", "libMaaToolkit.so"},
		"macos":   {platform.Target{OS: platform.MacOS, Arch: platform.AMD64}, "libMaaFramework.dylib", "libMaaToolkit.dylib"},
	}
	for name, tc := range cases {
		if got := tc.target.FrameworkLibrary(); got != tc.framework {
			t.Errorf("%s framework library = %q, want %q", name, got, tc.framework)
		}
		if got := tc.target.ToolkitLibrary(); got != tc.toolkit {
			t.Errorf("%s toolkit library = %q, want %q", name, got, tc.toolkit)
		}
		if got := tc.target.Libraries(); len(got) != 2 || got[0] != tc.framework {
			t.Errorf("%s libraries = %v, want %q first", name, got, tc.framework)
		}
	}
}

func TestPlatformHostMatchesTheBuild(t *testing.T) {
	host := platform.Host()
	if host.ID() != string(host.OS)+"-"+string(host.Arch) {
		t.Fatalf("Host() = %s, want the OS and architecture joined by a dash", host.ID())
	}
	// Host() reports the build's GOOS/GOARCH, also on a platform MaaFramework
	// does not ship for (linux/386, freebsd, ...): the file names are then
	// simply never found, which the callers report as a load failure.
	switch runtime.GOOS {
	case "windows":
		if host.OS != platform.Windows {
			t.Errorf("Host() = %s on windows", host.ID())
		}
	case "linux":
		if host.OS != platform.Linux {
			t.Errorf("Host() = %s on linux", host.ID())
		}
	case "darwin":
		if host.OS != platform.MacOS {
			t.Errorf("Host() = %s on darwin", host.ID())
		}
	}
	switch runtime.GOARCH {
	case "amd64":
		if host.Arch != platform.AMD64 {
			t.Errorf("Host() = %s on amd64", host.ID())
		}
	case "arm64":
		if host.Arch != platform.AArch64 {
			t.Errorf("Host() = %s on arm64", host.ID())
		}
	}
	if filepath.Base(host.FrameworkLibrary()) == "" {
		t.Error("the host platform must name a framework library")
	}
}
