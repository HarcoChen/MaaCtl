// Package maafw contains helpers for locating and initializing the MaaFramework
// shared libraries used by maactl.
package maafw

import (
	"fmt"
	"os"
	"path/filepath"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
)

// Init loads MaaFramework from libDir and silences its stdout logging so the
// CLI output stays clean. Callers must call maa.Release when done.
func Init(libDir string) error {
	return maa.Init(maa.WithLibDir(libDir), maa.WithStdoutLevel(maa.LoggingLevelOff))
}

// ResolveLibDir locates the directory holding MaaFramework.dll and
// MaaToolkit.dll. An explicit path wins; otherwise the current working
// directory and the executable's neighborhood are searched.
func ResolveLibDir(explicit string) (string, error) {
	candidates := make([]string, 0, 4)
	if explicit != "" {
		candidates = append(candidates, explicit)
	} else {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, "maafw", "bin"))
		}
		if exe, err := os.Executable(); err == nil {
			d := filepath.Dir(exe)
			candidates = append(candidates, filepath.Join(d, "maafw", "bin"), filepath.Join(d, "..", "maafw", "bin"))
		}
	}
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
		if fileExists(filepath.Join(absolute, "MaaFramework.dll")) && fileExists(filepath.Join(absolute, "MaaToolkit.dll")) {
			return absolute, nil
		}
	}
	if explicit != "" {
		return "", fmt.Errorf("MaaFramework DLLs not found in %q (expected MaaFramework.dll and MaaToolkit.dll)", explicit)
	}
	return "", fmt.Errorf("MaaFramework DLLs not found; expected them under %q", filepath.Join(".", "maafw", "bin"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
