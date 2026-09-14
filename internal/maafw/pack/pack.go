// Package pack turns an official MaaFramework release archive into the compact
// payload embedded by bundled maactl builds.
package pack

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

// BinDir is the directory of a MaaFramework release archive that holds the
// runtime libraries shipped with maactl. Everything else (docs, samples,
// headers, symbols) is intentionally dropped.
const BinDir = "bin"

// RequiredLibraries must exist in every payload, because maactl cannot load
// MaaFramework without them.
var RequiredLibraries = []string{"MaaFramework.dll", "MaaToolkit.dll"}

// versionPattern matches the release tag in names such as
// "MAA-win-x86_64-v5.13.0.zip".
var versionPattern = regexp.MustCompile(`v\d+(?:\.\d+){1,2}(?:[-+][0-9A-Za-z.\-]+)?`)

// VersionFromName extracts the MaaFramework version from an archive name and
// returns "" when the name carries none.
func VersionFromName(name string) string {
	base := path.Base(name)
	if ext := path.Ext(base); strings.EqualFold(ext, ".zip") {
		// The extension would otherwise look like part of a prerelease suffix.
		base = strings.TrimSuffix(base, ext)
	}
	return versionPattern.FindString(base)
}

// Container re-packs the bin/ directory of a MaaFramework release archive into
// a zip stream that bundled builds embed and extract at run time, reporting how
// many files it packed. Only regular files below bin/ are kept and the bin/
// prefix is dropped, so the container mirrors the directory layout
// MaaFramework expects next to its libraries.
func Container(archive []byte) ([]byte, int, error) {
	src, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, 0, fmt.Errorf("read MaaFramework archive: %w", err)
	}
	files, err := binFiles(src)
	if err != nil {
		return nil, 0, err
	}
	var out bytes.Buffer
	dst := zip.NewWriter(&out)
	for _, file := range files {
		if err := writeEntry(dst, file); err != nil {
			return nil, 0, err
		}
	}
	if err := dst.Close(); err != nil {
		return nil, 0, fmt.Errorf("build payload: %w", err)
	}
	return out.Bytes(), len(files), nil
}

// binFiles lists the regular files below bin/ with the prefix stripped and
// verifies that the required runtime libraries are present.
func binFiles(src *zip.Reader) ([]*zip.File, error) {
	prefix := BinDir + "/"
	var files []*zip.File
	found := make(map[string]bool, len(src.File))
	for _, file := range src.File {
		name := file.Name
		if strings.HasSuffix(name, "/") || !strings.HasPrefix(name, prefix) {
			continue
		}
		relative := strings.TrimPrefix(name, prefix)
		if relative == "" || strings.HasPrefix(path.Base(relative), ".") {
			continue
		}
		files = append(files, file)
		found[relative] = true
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("MaaFramework archive has no %s/ files", BinDir)
	}
	for _, required := range RequiredLibraries {
		if !found[required] {
			return nil, fmt.Errorf("MaaFramework archive is missing %s/%s", BinDir, required)
		}
	}
	return files, nil
}

// writeEntry copies one release entry into the container under its stripped
// name. Entries are re-compressed because Go's embed stores the payload
// verbatim inside the executable, where size matters.
func writeEntry(dst *zip.Writer, src *zip.File) error {
	name := strings.TrimPrefix(src.Name, BinDir+"/")
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: src.Modified}
	writer, err := dst.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	reader, err := src.Open()
	if err != nil {
		return fmt.Errorf("read %s: %w", src.Name, err)
	}
	defer reader.Close()
	if _, err := io.Copy(writer, reader); err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	return nil
}
