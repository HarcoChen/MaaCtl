// Package pack turns a MaaFramework runtime directory into the payload embedded
// by bundled maactl builds.
//
// The runtime directory is what a MaaFramework release archive unpacks into
// maafw/bin: the shared libraries maactl loads at run time, plus the control
// units and plugins they need. The packer packs exactly the files it finds
// there and cares about nothing else—not the version, not the platform, not
// which MaaFramework release the files came from. Producing a correct runtime
// directory is the job of whoever fills maafw/ (CI downloads the release of the
// platform it builds for), and the runtime itself is the authority on which
// version it is.
package pack

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Dir is where a MaaFramework release archive is unpacked, relative to the
// project root: the runtime libraries live in its bin/ subdirectory. CI fills
// this directory before packing, so the packer itself never downloads anything.
var Dir = filepath.Join("maafw", "bin")

// Container packs every file of dir into a zip stream that bundled builds embed
// and extract at run time, reporting how many files it packed.
func Container(dir string) ([]byte, int, error) {
	files, err := runtimeFiles(dir)
	if err != nil {
		return nil, 0, err
	}
	var out bytes.Buffer
	dst := zip.NewWriter(&out)
	for _, file := range files {
		if err := writeEntry(dst, dir, file); err != nil {
			return nil, 0, err
		}
	}
	if err := dst.Close(); err != nil {
		return nil, 0, fmt.Errorf("build payload: %w", err)
	}
	return out.Bytes(), len(files), nil
}

// runtimeFiles lists the regular files of a runtime directory as names relative
// to it, with forward slashes so the container looks the same on every
// platform. Hidden files are skipped: they are editor and version-control
// leftovers, never part of a release.
func runtimeFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("MaaFramework runtime directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	var files []string
	err = filepath.WalkDir(dir, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		relative = path.Clean(filepath.ToSlash(relative))
		if relative == "." || strings.HasPrefix(path.Base(relative), ".") {
			return nil
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s holds no files; unpack a MaaFramework release archive there first", dir)
	}
	slices.Sort(files)
	return files, nil
}

// writeEntry copies one runtime file into the container under its relative
// name. Entries are re-compressed because Go's embed stores the payload
// verbatim inside the executable, where size matters.
func writeEntry(dst *zip.Writer, dir, name string) error {
	source, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	header := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: info.ModTime(),
	}
	writer, err := dst.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	if _, err := io.Copy(writer, source); err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	return nil
}
