// Package platform describes the platform this build of maactl runs on and the
// file names MaaFramework uses for it.
//
// The identifiers follow the MaaFramework release assets ("MAA-<os>-<arch>-..."),
// and nothing here knows about versions: maactl never pins a MaaFramework
// release, it only needs to know how the runtime files are spelled on the host
// so it can find and load them.
package platform

import "runtime"

// OS is the operating-system part of a platform id.
type OS string

// The operating systems MaaFramework ships desktop releases for.
const (
	Windows OS = "win"
	Linux   OS = "linux"
	MacOS   OS = "macos"
)

// Arch is the CPU architecture part of a platform id.
type Arch string

// The CPU architectures MaaFramework ships releases for.
const (
	AMD64   Arch = "x86_64"
	AArch64 Arch = "aarch64"
)

// Target is one MaaFramework platform, for example win-x86_64.
type Target struct {
	OS   OS
	Arch Arch
}

// ID renders the platform the way MaaFramework names it, e.g. "win-x86_64".
func (t Target) ID() string {
	return string(t.OS) + "-" + string(t.Arch)
}

// FrameworkLibrary is the file name of the MaaFramework runtime library.
func (t Target) FrameworkLibrary() string {
	return t.library("MaaFramework")
}

// ToolkitLibrary is the file name of the MaaToolkit library.
func (t Target) ToolkitLibrary() string {
	return t.library("MaaToolkit")
}

// Libraries lists the libraries maactl cannot start without, in load order.
func (t Target) Libraries() []string {
	return []string{t.FrameworkLibrary(), t.ToolkitLibrary()}
}

// library renders one library file name for the platform.
func (t Target) library(base string) string {
	switch t.OS {
	case Windows:
		return base + ".dll"
	case Linux:
		return "lib" + base + ".so"
	case MacOS:
		return "lib" + base + ".dylib"
	default:
		return base
	}
}

// Host returns the platform this build of maactl runs on. On a platform
// MaaFramework does not ship for, it reports the raw GOOS/GOARCH values and no
// library can be found, which the callers report as a load failure.
func Host() Target {
	return Target{OS: osName(runtime.GOOS), Arch: archName(runtime.GOARCH)}
}

// osName maps Go's GOOS onto a platform OS.
func osName(goos string) OS {
	switch goos {
	case "windows":
		return Windows
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	default:
		return OS(goos)
	}
}

// archName maps Go's GOARCH onto a platform architecture.
func archName(goarch string) Arch {
	switch goarch {
	case "amd64":
		return AMD64
	case "arm64":
		return AArch64
	default:
		return Arch(goarch)
	}
}
