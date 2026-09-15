package tests

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/maafw/pack"
	"maactl/internal/platform"
)

// writeRuntime builds a MaaFramework runtime directory shaped like the bin/ of
// a release archive: the platform's libraries plus whatever else is passed in.
func writeRuntime(t *testing.T, target platform.Target, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		target.FrameworkLibrary(): string(fakeLibrary(target)),
		target.ToolkitLibrary():   "toolkit",
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

func TestContainerKeepsTheWholeRuntime(t *testing.T) {
	target := platform.Target{OS: platform.Windows, Arch: platform.AMD64}
	dir := writeRuntime(t, target, map[string]string{
		"plugins/MaaPluginDemo.dll": "plugin",
		"MaaPiCli.exe":              "cli",
		".hidden":                   "hidden",
	})
	container, count, err := pack.Container(dir, target)
	if err != nil {
		t.Fatal(err)
	}
	files := containerFiles(t, container)
	want := []string{"MaaFramework.dll", "MaaToolkit.dll", "MaaPiCli.exe", "plugins/MaaPluginDemo.dll"}
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

func TestContainerRejectsIncompleteRuntimes(t *testing.T) {
	target := platform.Target{OS: platform.Linux, Arch: platform.AMD64}
	empty := t.TempDir()
	missingToolkit := writeRuntime(t, target, nil)
	if err := os.Remove(filepath.Join(missingToolkit, target.ToolkitLibrary())); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{
		"missing directory": filepath.Join(t.TempDir(), "nope"),
		"empty directory":   empty,
		"no toolkit":        missingToolkit,
	} {
		if _, _, err := pack.Container(dir, target); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestInspectReadsThePlatformFromTheRuntime(t *testing.T) {
	for _, target := range platform.Supported {
		dir := writeRuntime(t, target, nil)
		detected, err := pack.Inspect(dir)
		if err != nil {
			t.Fatalf("%s: %v", target.ID(), err)
		}
		if detected != target {
			t.Errorf("Inspect = %s, want %s", detected.ID(), target.ID())
		}
	}
}

func TestInspectRejectsRuntimesItCannotUse(t *testing.T) {
	empty := t.TempDir()
	if _, err := pack.Inspect(empty); err == nil {
		t.Error("expected an empty directory to fail")
	}
	if _, err := pack.Inspect(filepath.Join(empty, "missing")); err == nil {
		t.Error("expected a missing directory to fail")
	}
	// A runtime whose header cannot be read still reports its operating system,
	// but leaves the architecture open so the caller can decide.
	dir := t.TempDir()
	for _, name := range []string{"libMaaFramework.so", "libMaaToolkit.so"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("nope"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	detected, err := pack.Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if detected.OS != platform.Linux || detected.Arch != "" {
		t.Errorf("Inspect = %q, want the Linux runtime without an architecture", detected.ID())
	}
	// Packing it for a platform whose libraries are not there must fail.
	if _, _, err := pack.Container(dir, platform.Target{OS: platform.Linux, Arch: platform.AArch64}); err != nil {
		t.Errorf("packing a Linux runtime for linux-aarch64: %v", err)
	}
}

func TestInspectReportsMixedRuntimes(t *testing.T) {
	dir := writeRuntime(t, platform.Target{OS: platform.MacOS, Arch: platform.AArch64}, map[string]string{
		"MaaFramework.dll": "windows framework",
		"MaaToolkit.dll":   "windows toolkit",
	})
	if _, err := pack.Inspect(dir); err == nil || !strings.Contains(err.Error(), "several platforms") {
		t.Errorf("expected a mixed-runtime error, got %v", err)
	}
}
