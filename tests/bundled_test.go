package tests

import (
	"testing"

	"maactl/internal/maafw/bundled"
)

// TestBundledBuildCarriesPayload guards the release build: compiling with
// -tags bundled without packing the payload first would silently produce an
// executable that cannot use its own MaaFramework.
func TestBundledBuildCarriesPayload(t *testing.T) {
	if !bundled.Compiled() {
		t.Skip("build without -tags bundled")
	}
	if !bundled.Available() {
		t.Fatal("built with -tags bundled but no payload; run \"go run ./tools/packmaafw\" first")
	}
	if bundled.CacheID() == "" {
		t.Error("a payload must name the cache directory it is extracted into")
	}
}

// TestUnbundledBuildReportsNoLibraries guards the lite build: plain builds must
// not claim to carry MaaFramework.
func TestUnbundledBuildReportsNoLibraries(t *testing.T) {
	if bundled.Compiled() {
		t.Skip("build with -tags bundled")
	}
	if bundled.Available() {
		t.Error("an unbundled build must not report embedded libraries")
	}
}
