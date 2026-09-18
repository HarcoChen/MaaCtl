package maafw

import (
	"fmt"
	"os"
	"path/filepath"

	"maactl/internal/maafw/bundled"
)

// bundledLibDir extracts the MaaFramework libraries embedded in this build into
// the user cache directory and returns the directory holding them.
//
// Extracting once per payload keeps start-up fast while never modifying the
// directory the executable lives in, which may be read-only.
func bundledLibDir() (string, error) {
	target, err := bundledTargetDir()
	if err != nil {
		return "", err
	}
	if _, err := bundled.Extract(target); err != nil {
		return "", err
	}
	return target, nil
}

// bundledTargetDir returns the cache directory the embedded payload is
// extracted into, without extracting it. It reports the same errors
// bundledLibDir does, so callers can tell whether a resolved directory came
// from the embedded payload.
func bundledTargetDir() (string, error) {
	switch {
	case !bundled.Compiled():
		return "", errNoBundle
	case !bundled.Available():
		return "", fmt.Errorf("this build was compiled with -tags bundled but carries no MaaFramework payload; run \"go run ./tools/packmaafw\" before building")
	}
	root, err := os.UserCacheDir()
	if err != nil {
		root, err = fallbackCacheDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(root, "maactl", "maafw", bundled.CacheID(), "bin"), nil
}

// BundledLibDir returns the directory this build's embedded payload would be
// extracted into, or "" when the build has no usable payload. It never
// extracts the payload, so callers can compare it against a resolved library
// directory to tell whether the runtime actually came from the embedded
// payload rather than from ./maafw/bin.
func BundledLibDir() string {
	dir, err := bundledTargetDir()
	if err != nil {
		return ""
	}
	return dir
}

// fallbackCacheDir names a cache root under the system temporary directory for
// the case where the user cache directory is unavailable. The temporary
// directory is world-writable, so the name carries the user id (or, on Windows
// where os.Getuid reports -1, the process id, which never spells an invalid path
// name) and the directory is created readable by its owner only.
func fallbackCacheDir() (string, error) {
	owner := os.Getuid()
	if owner < 0 {
		owner = os.Getpid()
	}
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("maactl-%d", owner))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create MaaFramework cache directory: %w", err)
	}
	return dir, nil
}
