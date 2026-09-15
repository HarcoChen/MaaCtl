package maafw

import (
	"fmt"
	"os"
	"path/filepath"

	"maactl/internal/maafw/bundled"
	"maactl/internal/platform"
)

// bundledLibDir extracts the MaaFramework libraries embedded in this build into
// the user cache directory and returns the directory holding them.
//
// Extracting once per payload keeps start-up fast while never modifying the
// directory the executable lives in, which may be read-only.
func bundledLibDir() (string, error) {
	switch {
	case !bundled.Compiled():
		return "", errNoBundle
	case !bundled.Available():
		if target := bundled.Target(); target != "" && target != platform.Host().ID() {
			return "", fmt.Errorf("this build carries MaaFramework libraries for %s but was built for %s; build it on the target platform", target, platform.Host().ID())
		}
		return "", fmt.Errorf("this build was compiled with -tags bundled but carries no MaaFramework payload; run \"go run ./tools/packmaafw\" before building")
	}
	root, err := os.UserCacheDir()
	if err != nil {
		root, err = fallbackCacheDir()
		if err != nil {
			return "", err
		}
	}
	target := filepath.Join(root, "maactl", "maafw", bundled.CacheID(), "bin")
	if _, err := bundled.Extract(target); err != nil {
		return "", err
	}
	return target, nil
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
