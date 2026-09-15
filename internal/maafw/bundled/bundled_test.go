package bundled

import (
	"strings"
	"testing"

	"maactl/internal/platform"
)

func TestCompatibleOnlyAcceptsTheHostPlatform(t *testing.T) {
	host := platform.Host().ID()
	if !compatible(payload{container: []byte("zip"), target: host}) {
		t.Errorf("a payload for %s must be usable on %s", host, host)
	}
	// A payload packed before the platform marker existed carries no target, and
	// is trusted so that upgrading maactl does not invalidate it.
	if !compatible(payload{container: []byte("zip")}) {
		t.Error("a payload without a platform marker must be accepted")
	}
	other := platform.Target{OS: platform.Linux, Arch: platform.AMD64}
	if other.ID() == host {
		other = platform.Target{OS: platform.Windows, Arch: platform.AArch64}
	}
	if compatible(payload{container: []byte("zip"), target: other.ID()}) {
		t.Errorf("a payload for %s must not be usable on %s", other.ID(), host)
	}
}

func TestCacheIDNamesThePlatformAndVersion(t *testing.T) {
	id := cacheID(payload{container: []byte("zip"), version: "v5.13.0", target: "win-x86_64"})
	if id != "win-x86_64-v5.13.0" {
		t.Errorf("cacheID = %q, want win-x86_64-v5.13.0", id)
	}
	// Without a version the payload hash keeps two builds apart, and the name
	// stays usable as a directory name on every platform.
	hashed := cacheID(payload{container: []byte("zip"), target: "linux-aarch64"})
	if !strings.HasPrefix(hashed, "linux-aarch64-") || len(hashed) != len("linux-aarch64-")+16 {
		t.Errorf("cacheID = %q, want the platform followed by a short hash", hashed)
	}
	if strings.ContainsAny(hashed, `/\:`) {
		t.Errorf("cacheID = %q is not path safe", hashed)
	}
	if cacheID(payload{}) != "" {
		t.Error("a build without a payload must not name a cache directory")
	}
}
