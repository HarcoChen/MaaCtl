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

func TestCacheIDSeparatesPayloads(t *testing.T) {
	first := cacheID(payload{container: []byte("zip")})
	second := cacheID(payload{container: []byte("other zip")})
	if first == second {
		t.Fatal("two different payloads must not share a cache directory")
	}
	for _, id := range []string{first, second} {
		if !strings.HasPrefix(id, platform.Host().ID()+"-") {
			t.Errorf("cacheID = %q, want the host platform followed by a digest", id)
		}
		if strings.ContainsAny(id, `/:\`) {
			t.Errorf("cacheID = %q is not path safe", id)
		}
	}
	// The same payload always names the same directory, so the extraction is
	// reused instead of being redone on every run.
	if again := cacheID(payload{container: []byte("zip")}); again != first {
		t.Errorf("cacheID is not stable: %q then %q", first, again)
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
