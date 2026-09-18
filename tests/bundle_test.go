package tests

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/maafw/bundled"
	"maactl/internal/platform"
)

// frameworkLibrary and toolkitLibrary are the names a payload carries on the
// platform the tests run on.
func frameworkLibrary() string { return platform.Host().FrameworkLibrary() }

func toolkitLibrary() string { return platform.Host().ToolkitLibrary() }

// payloadContainer builds an in-memory payload container as tools/packmaafw
// would produce it.
func payloadContainer(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func completePayload(t *testing.T) []byte {
	t.Helper()
	return payloadContainer(t, map[string]string{
		frameworkLibrary():   "framework",
		toolkitLibrary():     "toolkit",
		"plugins/plugin.bin": "plugin",
	})
}

func TestExtractZipWritesEveryEntry(t *testing.T) {
	target := filepath.Join(t.TempDir(), "cache", "bin")
	written, err := bundled.ExtractZip(completePayload(t), target)
	if err != nil {
		t.Fatal(err)
	}
	if written != 3 {
		t.Errorf("wrote %d files, want 3", written)
	}
	for name, want := range map[string]string{
		frameworkLibrary():   "framework",
		toolkitLibrary():     "toolkit",
		"plugins/plugin.bin": "plugin",
	} {
		content, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(content) != want {
			t.Errorf("%s = %q, want %q", name, content, want)
		}
	}
}

func TestExtractZipReusesAndRepairsTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin")
	payload := completePayload(t)
	if _, err := bundled.ExtractZip(payload, target); err != nil {
		t.Fatal(err)
	}
	// A complete extraction is reused instead of being written again.
	if written, err := bundled.ExtractZip(payload, target); err != nil || written != 0 {
		t.Errorf("second extraction wrote %d files, err %v; want 0", written, err)
	}
	// An interrupted extraction (no main library) is redone, because the main
	// library is always written last.
	if err := os.Remove(filepath.Join(target, frameworkLibrary())); err != nil {
		t.Fatal(err)
	}
	written, err := bundled.ExtractZip(payload, target)
	if err != nil || written != 3 {
		t.Fatalf("repair wrote %d files, err %v; want 3", written, err)
	}
	if _, err := os.Stat(filepath.Join(target, frameworkLibrary())); err != nil {
		t.Errorf("main library was not restored: %v", err)
	}
}

func TestExtractZipRejectsBrokenContainers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin")
	if _, err := bundled.ExtractZip([]byte("nope"), target); err == nil {
		t.Error("expected a corrupt container to fail")
	}
	incomplete := payloadContainer(t, map[string]string{toolkitLibrary(): "toolkit"})
	if _, err := bundled.ExtractZip(incomplete, target); err == nil {
		t.Error("expected a container without the framework library to fail")
	}
}

// TestExtractZipLeavesNoPartialExtractionWhenItFails pins the atomic write: the
// main library doubles as the completeness marker of the cache directory, so a
// failed extraction must not leave a half-written one behind, and no temporary
// file may survive it either.
func TestExtractZipLeavesNoPartialExtractionWhenItFails(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file where the payload wants a directory makes the extraction fail
	// halfway through.
	blocked := filepath.Join(target, "plugins")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bundled.ExtractZip(completePayload(t), target); err == nil {
		t.Fatal("expected extracting into a blocked directory to fail")
	}
	if _, err := os.Stat(filepath.Join(target, frameworkLibrary())); err == nil {
		t.Error("a failed extraction must not leave the main library behind")
	}
	// No half-written file is left around: a temporary file is renamed into
	// place only once it is complete.
	if err := filepath.WalkDir(filepath.Dir(target), func(name string, entry os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("%s was left behind", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The same payload extracts completely once the obstruction is gone, so a
	// failed run never poisons the cache.
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	written, err := bundled.ExtractZip(completePayload(t), target)
	if err != nil || written != 3 {
		t.Fatalf("repair wrote %d files, err %v; want 3", written, err)
	}
}

func TestExtractZipKeepsEntriesInsideTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache", "bin")
	payload := payloadContainer(t, map[string]string{
		frameworkLibrary(): "framework",
		"../escape.bin":    "evil",
	})
	if _, err := bundled.ExtractZip(payload, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "escape.bin")); err != nil {
		t.Errorf("sanitized entry was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "escape.bin")); err == nil {
		t.Error("entry escaped the extraction directory")
	}
}
