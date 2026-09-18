package tests

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"maactl/internal/maafw/pack"
)

// writeRuntime builds a MaaFramework runtime directory shaped like the bin/ of
// a release archive: the platform's libraries plus whatever else is passed in.
// The packer never inspects the content, so the files are plain bytes.
func writeRuntime(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		frameworkLibrary(): string("framework"),
		toolkitLibrary():   "toolkit",
	}
	for name, content := range extra {
		files[name] = content
	}
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func containerFiles(t *testing.T, container []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(container), int64(len(container)))
	if err != nil {
		t.Fatalf("read container: %v", err)
	}
	files := map[string]string{}
	for _, file := range reader.File {
		content, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(content); err != nil {
			t.Fatal(err)
		}
		content.Close()
		files[file.Name] = buf.String()
	}
	return files
}

// TestContainerPacksWhateverTheRuntimeHolds pins the packer's contract: it
// copies the directory, whatever MaaFramework version or platform those files
// belong to, and skips only hidden leftovers.
func TestContainerPacksWhateverTheRuntimeHolds(t *testing.T) {
	dir := writeRuntime(t, map[string]string{
		"plugins/MaaPluginDemo.dll": "plugin",
		"MaaPiCli.exe":              "cli",
		".hidden":                   "hidden",
	})
	container, count, err := pack.Container(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := containerFiles(t, container)
	want := []string{frameworkLibrary(), toolkitLibrary(), "MaaPiCli.exe", "plugins/MaaPluginDemo.dll"}
	if count != len(want) || len(files) != len(want) {
		t.Fatalf("packed %d files, count %d, want %v", len(files), count, want)
	}
	for _, name := range want {
		if _, ok := files[name]; !ok {
			t.Errorf("payload is missing %s: %v", name, files)
		}
	}
	if files["plugins/MaaPluginDemo.dll"] != "plugin" {
		t.Errorf("payload content changed: %v", files)
	}
}

// TestContainerRejectsEmptyRuntimes keeps the one assertion that is still
// worth making: packing a directory that holds nothing would embed a payload
// that cannot possibly load.
func TestContainerRejectsEmptyRuntimes(t *testing.T) {
	dotOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(dotOnly, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{
		"missing directory": filepath.Join(t.TempDir(), "nope"),
		"empty directory":   t.TempDir(),
		"only hidden files": dotOnly,
		"a file":            filepath.Join(dotOnly, ".gitkeep"),
	} {
		if _, _, err := pack.Container(dir); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
