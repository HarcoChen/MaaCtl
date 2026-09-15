// Package pack turns a MaaFramework runtime directory into the compact payload
// embedded by bundled maactl builds.
//
// The runtime directory is what a MaaFramework release archive unpacks into
// maafw/bin: the shared libraries maactl loads at run time, plus the control
// units and plugins they need. Everything else in the archive (docs, samples,
// headers, symbols) is never part of a payload.
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

	"maactl/internal/platform"
)

// Dir is where a MaaFramework release archive is unpacked, relative to the
// project root: the runtime libraries live in its bin/ subdirectory. CI fills
// this directory before packing, so the packer itself never downloads anything.
var Dir = filepath.Join("maafw", "bin")

// Inspect identifies the platform of the MaaFramework runtime in dir from the
// library file names, and reads the architecture out of the framework library.
// The returned Target has an empty Arch when the library header is not a
// recognizable executable format, which only matters for hand-made runtimes.
func Inspect(dir string) (platform.Target, error) {
	var systems []platform.OS
	for _, target := range platform.Supported {
		if !hasLibraries(dir, target) {
			continue
		}
		if !slices.Contains(systems, target.OS) {
			systems = append(systems, target.OS)
		}
	}
	switch len(systems) {
	case 1:
	case 0:
		return platform.Target{}, fmt.Errorf("%s holds no MaaFramework libraries", dir)
	default:
		names := make([]string, len(systems))
		for i, system := range systems {
			names[i] = string(system)
		}
		return platform.Target{}, fmt.Errorf("%s mixes runtimes of several platforms (%s)", dir, strings.Join(names, ", "))
	}
	target := platform.Target{OS: systems[0]}
	arches, err := platform.Architectures(filepath.Join(dir, target.FrameworkLibrary()))
	if err == nil && len(arches) > 0 {
		target.Arch = arches[0]
	}
	return target, nil
}

// Container packs the MaaFramework runtime in dir into a zip stream that
// bundled builds embed and extract at run time, reporting how many files it
// packed. The libraries of target must all be present, so a directory that
// belongs to another platform fails instead of producing an unusable payload.
func Container(dir string, target platform.Target) ([]byte, int, error) {
	files, err := runtimeFiles(dir)
	if err != nil {
		return nil, 0, err
	}
	if err := requireLibraries(dir, target); err != nil {
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
		return nil, fmt.Errorf("%s holds no files", dir)
	}
	slices.Sort(files)
	return files, nil
}

// hasLibraries reports whether every library of target is present in dir.
func hasLibraries(dir string, target platform.Target) bool {
	for _, library := range target.Libraries() {
		if info, err := os.Stat(filepath.Join(dir, library)); err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

// requireLibraries fails unless dir holds every library of target, naming what
// is missing so a half-extracted archive is obvious.
func requireLibraries(dir string, target platform.Target) error {
	var missing []string
	for _, library := range target.Libraries() {
		if info, err := os.Stat(filepath.Join(dir, library)); err != nil || info.IsDir() {
			missing = append(missing, library)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s is missing %s for %s; unpack the MAA-%s release archive into %s",
			dir, strings.Join(missing, ", "), target.ID(), target.ID(), filepath.Dir(dir))
	}
	return nil
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
