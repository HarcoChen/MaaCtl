// Package maafw contains helpers for locating and initializing the MaaFramework
// shared libraries used by maactl.
package maafw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"maactl/internal/maafw/bundled"

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
	return maa.Init(options...)
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
		return "", fmt.Errorf("MaaFramework DLLs not found in %q (expected MaaFramework.dll and MaaToolkit.dll)", explicit)
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
	return "", fmt.Errorf("MaaFramework DLLs not found; expected them under %q or in a build with bundled libraries", filepath.Join(".", "maafw", "bin"))
}

// searchDirs lists the maafw/bin directories next to the working directory and
// the executable, in that order.
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
	return fileExists(filepath.Join(dir, "MaaFramework.dll")) && fileExists(filepath.Join(dir, "MaaToolkit.dll"))
}

// errNoBundle reports a build without an embedded MaaFramework payload.
var errNoBundle = errors.New("no bundled MaaFramework libraries")

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
