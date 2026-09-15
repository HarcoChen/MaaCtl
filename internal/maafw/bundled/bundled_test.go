package bundled

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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

// brokenZipEntry returns a one-entry zip whose data no longer matches its
// checksum, so reading the entry fails after its bytes were handed out. That is
// what a copy interrupted halfway through looks like to writeFile.
func brokenZipEntry(t *testing.T, name, content string) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	data[offset] ^= 0xff
	reader, err = zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return reader.File[0]
}

// TestWriteFileKeepsTheDestinationIntactWhenTheCopyFails pins the atomic write:
// the data goes to a temporary file first, so a copy that fails halfway leaves
// the previous file in place and cleans the temporary one up. Without it the
// destination would already hold the new, incomplete data, which is how a cache
// directory ends up looking complete while holding a truncated library.
func TestWriteFileKeepsTheDestinationIntactWhenTheCopyFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "library.bin")
	if err := os.WriteFile(dest, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dest, brokenZipEntry(t, "library.bin", "new content")); err == nil {
		t.Fatal("expected reading a corrupt entry to fail")
	}
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous" {
		t.Errorf("destination = %q, want the previous content", content)
	}
	if _, err := os.Stat(dest + ".tmp"); err == nil {
		t.Error("the temporary file was not cleaned up")
	}
}
