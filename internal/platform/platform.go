// Package platform describes the operating systems and CPU architectures
// MaaFramework publishes releases for, together with the platform this build of
// maactl runs on.
//
// The identifiers follow the MaaFramework release assets, which are named
// "MAA-<os>-<arch>-<version>.zip" with os in {win, linux, macos} and arch in
// {x86_64, aarch64}. Android releases exist upstream but are meant to be
// embedded in an APK, so no maactl build targets them.
package platform

import (
	"fmt"
	"runtime"
	"strings"
)

// OS is the operating-system part of a MaaFramework platform id.
type OS string

// The operating systems MaaFramework ships desktop releases for.
const (
	Windows OS = "win"
	Linux   OS = "linux"
	MacOS   OS = "macos"
)

// Arch is the CPU architecture part of a MaaFramework platform id.
type Arch string

// The CPU architectures MaaFramework ships releases for.
const (
	AMD64   Arch = "x86_64"
	AArch64 Arch = "aarch64"
)

// Target is one MaaFramework release platform, for example win-x86_64.
type Target struct {
	OS   OS
	Arch Arch
}

// Supported lists every platform maactl can be built for, in the order the
// release artifacts are published.
var Supported = []Target{
	{OS: Windows, Arch: AMD64},
	{OS: Windows, Arch: AArch64},
	{OS: Linux, Arch: AMD64},
	{OS: Linux, Arch: AArch64},
	{OS: MacOS, Arch: AMD64},
	{OS: MacOS, Arch: AArch64},
}

// ID renders the platform the way MaaFramework names it, e.g. "win-x86_64".
func (t Target) ID() string {
	return string(t.OS) + "-" + string(t.Arch)
}

// Title renders the platform for humans, e.g. "Windows x86_64".
func (t Target) Title() string {
	name := t.OS.Name()
	if name == "" {
		return t.ID()
	}
	return name + " " + string(t.Arch)
}

// Name spells out the operating system, or "" for an unknown one.
func (o OS) Name() string {
	switch o {
	case Windows:
		return "Windows"
	case Linux:
		return "Linux"
	case MacOS:
		return "macOS"
	default:
		return ""
	}
}

// GoOS maps the platform onto Go's GOOS value.
func (t Target) GoOS() string {
	switch t.OS {
	case Windows:
		return "windows"
	case Linux:
		return "linux"
	case MacOS:
		return "darwin"
	default:
		return ""
	}
}

// GoARCH maps the platform onto Go's GOARCH value.
func (t Target) GoARCH() string {
	switch t.Arch {
	case AMD64:
		return "amd64"
	case AArch64:
		return "arm64"
	default:
		return ""
	}
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

// Executable appends the platform's executable suffix, e.g. "maactl" on Linux
// and "maactl.exe" on Windows.
func (t Target) Executable(name string) string {
	if t.OS == Windows {
		return name + ".exe"
	}
	return name
}

// Archive names the release archive holding the executables, e.g.
// "maactl-0.1.0-linux-x86_64.zip".
func (t Target) Archive(base, version string) string {
	return fmt.Sprintf("%s-%s-%s.zip", base, version, t.ID())
}

// Known reports whether the target names a platform maactl can be built for.
func (t Target) Known() bool {
	for _, candidate := range Supported {
		if candidate == t {
			return true
		}
	}
	return false
}

// Host returns the platform this build of maactl runs on. It is the zero
// Target (and Known reports false) on a platform maactl does not support.
func Host() Target {
	return Target{OS: osName(runtime.GOOS), Arch: archName(runtime.GOARCH)}
}

// Parse turns a platform id such as "win-x86_64" into a Target.
func Parse(id string) (Target, error) {
	for _, target := range Supported {
		if strings.EqualFold(id, target.ID()) {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("unknown platform %q; supported: %s", id, IDs())
}

// IDs lists every supported platform id, comma separated.
func IDs() string {
	ids := make([]string, len(Supported))
	for i, target := range Supported {
		ids[i] = target.ID()
	}
	return strings.Join(ids, ", ")
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
