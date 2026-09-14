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
	switch {
	case !bundled.Compiled():
		return "", errNoBundle
	case !bundled.Available():
		return "", fmt.Errorf("this build was compiled with -tags bundled but carries no MaaFramework payload; run \"go run ./tools/packmaafw\" before building")
	}
	root, err := os.UserCacheDir()
	if err != nil {
		root = os.TempDir()
	}
	target := filepath.Join(root, "maactl", "maafw", bundled.CacheID(), "bin")
	if _, err := bundled.Extract(target); err != nil {
		return "", err
	}
	return target, nil
}
