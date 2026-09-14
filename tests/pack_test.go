package tests

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"maactl/internal/maafw/pack"
)

// releaseArchive builds an in-memory archive shaped like an official
// MaaFramework release, whose runtime lives under bin/ and whose remaining
// directories must not end up in the payload.
func releaseArchive(t *testing.T, bin map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"README.md":            "# MaaFramework",
		"docs/1.1-Guide.md":    "docs",
		"include/MaaDef.h":     "header",
		"lib/MaaFramework.lib": "import library",
		"bin/":                 "",
		"bin/MaaFramework.dll": "framework",
		"bin/MaaToolkit.dll":   "toolkit",
	}
	for name, content := range bin {
		files[name] = content
	}
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

func TestContainerKeepsOnlyBinFiles(t *testing.T) {
	archive := releaseArchive(t, map[string]string{
		"bin/plugins/MaaPluginDemo.dll": "plugin",
		"bin/MaaPiCli.exe":              "cli",
		"bin/.hidden":                   "hidden",
	})
	container, count, err := pack.Container(archive)
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
	for name := range files {
		if strings.HasPrefix(name, "bin/") {
			t.Errorf("payload kept the bin/ prefix: %s", name)
		}
	}
	if files["MaaFramework.dll"] != "framework" || files["plugins/MaaPluginDemo.dll"] != "plugin" {
		t.Errorf("payload content changed: %v", files)
	}
}

func TestContainerRejectsIncompleteArchives(t *testing.T) {
	for name, archive := range map[string][]byte{
		"not a zip":       []byte("nope"),
		"no bin":          releaseArchiveNoBin(t),
		"no MaaToolkit":   releaseArchiveWithBin(t, map[string]string{"MaaFramework.dll": "framework"}),
		"no MaaFramework": releaseArchiveWithBin(t, map[string]string{"MaaToolkit.dll": "toolkit"}),
	} {
		if _, _, err := pack.Container(archive); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func releaseArchiveNoBin(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("docs/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("docs")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func releaseArchiveWithBin(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create("bin/" + name)
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

func TestVersionFromName(t *testing.T) {
	for name, want := range map[string]string{
		"MAA-win-x86_64-v5.13.0.zip":           "v5.13.0",
		"assets/MAA-linux-aarch64-v5.13.0.zip": "v5.13.0",
		"MAA-macos-x86_64-v5.14.0-beta.1.zip":  "v5.14.0-beta.1",
		"C:/tmp/MAA-win-aarch64-v6.0.1.zip":    "v6.0.1",
		"maafw-bin.zip":                        "",
		"MAA-win-x86_64.zip":                   "",
	} {
		if got := pack.VersionFromName(name); got != want {
			t.Errorf("VersionFromName(%q) = %q, want %q", name, got, want)
		}
	}
}
