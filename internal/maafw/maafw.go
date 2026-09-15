// Package maafw contains helpers for locating and initializing the MaaFramework
// shared libraries used by maactl.
package maafw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"maactl/internal/maafw/bundled"
	"maactl/internal/platform"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// Init loads MaaFramework from libDir and silences its stdout logging so the
// CLI output stays clean. logDir, when not empty, receives MaaFramework's own
// log files. Callers must call maa.Release when done.
func Init(libDir, logDir string) error {
	options := []maa.InitOption{maa.WithLibDir(libDir), maa.WithStdoutLevel(maa.LoggingLevelOff)}
	if logDir != "" {
		options = append(options, maa.WithLogDir(logDir))
	}
	if err := maa.Init(options...); err != nil {
		return err
	}
	loaded.Store(true)
	return nil
}

// loaded records whether the native library has been initialized, so Version
// can avoid calling into it from tests or from query-only commands.
var loaded atomic.Bool

// Version returns the MaaFramework version of the loaded runtime, falling back
// to the version this build bundles when the runtime is not initialized yet.
func Version() string {
	if !loaded.Load() {
		return BundledVersion()
	}
	return maa.Version()
}

// BundledVersion returns the MaaFramework version carried by this build, or ""
// when the build does not embed MaaFramework.
func BundledVersion() string {
	if !bundled.Available() {
		return ""
	}
	return bundled.Version()
}

// ResolveLibDir returns the directory to load MaaFramework from.
//
// An explicit path always wins. Otherwise the libraries embedded in this build
// are extracted to a cache directory and used, and only builds without an
// embedded payload fall back to a maafw/bin directory next to the working
// directory or the executable.
func ResolveLibDir(explicit string) (string, error) {
	if explicit != "" {
		if dir, ok := firstLibDir([]string{explicit}); ok {
			return dir, nil
		}
		return "", fmt.Errorf("MaaFramework libraries not found in %q (expected %s)", explicit, libraryList())
	}
	dir, err := bundledLibDir()
	switch {
	case err == nil:
		return dir, nil
	case !errors.Is(err, errNoBundle):
		// A build with an embedded payload should always be able to use it, so
		// surface the reason instead of silently switching to other libraries.
		fmt.Fprintf(os.Stderr, "warning: cannot use the bundled MaaFramework: %v\n", err)
	}
	if dir, ok := firstLibDir(searchDirs()); ok {
		return dir, nil
	}
	return "", fmt.Errorf("MaaFramework libraries not found; expected %s under %q or a build with bundled libraries", libraryList(), filepath.Join(".", "maafw", "bin"))
}

// searchDirs lists the maafw/bin directories next to the working directory and
// the executable, in that order. The layout is the same on every platform: a
// MaaFramework release archive unpacks its runtime into bin/.
func searchDirs() []string {
	var dirs []string
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "maafw", "bin"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(dir, "maafw", "bin"), filepath.Join(dir, "..", "maafw", "bin"))
	}
	return dirs
}

// libraryList names the libraries a runtime directory must hold for the
// platform this build targets.
func libraryList() string {
	return strings.Join(platform.Host().Libraries(), " and ")
}

// firstLibDir returns the first candidate that holds the MaaFramework libraries.
func firstLibDir(candidates []string) (string, bool) {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		if hasLibs(absolute) {
			return absolute, true
		}
	}
	return "", false
}

// hasLibs reports whether dir holds the libraries MaaFramework needs to start.
func hasLibs(dir string) bool {
	for _, library := range platform.Host().Libraries() {
		if !fileExists(filepath.Join(dir, library)) {
			return false
		}
	}
	return true
}

// errNoBundle reports a build without an embedded MaaFramework payload.
var errNoBundle = errors.New("no bundled MaaFramework libraries")

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
