package tests

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"maactl/internal/maafw/bundled"
)

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
		"MaaFramework.dll":          "framework",
		"MaaToolkit.dll":            "toolkit",
		"plugins/MaaPluginDemo.dll": "plugin",
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
		"MaaFramework.dll":          "framework",
		"MaaToolkit.dll":            "toolkit",
		"plugins/MaaPluginDemo.dll": "plugin",
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
	if err := os.Remove(filepath.Join(target, "MaaFramework.dll")); err != nil {
		t.Fatal(err)
	}
	written, err := bundled.ExtractZip(payload, target)
	if err != nil || written != 3 {
		t.Fatalf("repair wrote %d files, err %v; want 3", written, err)
	}
	if _, err := os.Stat(filepath.Join(target, "MaaFramework.dll")); err != nil {
		t.Errorf("main library was not restored: %v", err)
	}
}

func TestExtractZipRejectsBrokenContainers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bin")
	if _, err := bundled.ExtractZip([]byte("nope"), target); err == nil {
		t.Error("expected a corrupt container to fail")
	}
	incomplete := payloadContainer(t, map[string]string{"MaaToolkit.dll": "toolkit"})
	if _, err := bundled.ExtractZip(incomplete, target); err == nil {
		t.Error("expected a container without MaaFramework.dll to fail")
	}
}

func TestExtractZipKeepsEntriesInsideTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache", "bin")
	payload := payloadContainer(t, map[string]string{
		"MaaFramework.dll": "framework",
		"../escape.dll":    "evil",
	})
	if _, err := bundled.ExtractZip(payload, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "escape.dll")); err != nil {
		t.Errorf("sanitized entry was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "escape.dll")); err == nil {
		t.Error("entry escaped the extraction directory")
	}
}
