// Package bundled exposes the MaaFramework runtime that bundled maactl builds
// carry inside the executable.
//
// The payload directory is filled by tools/packmaafw at build time and read
// back here at run time: the container archive is unpacked into a cache
// directory, which is then handed to MaaFramework as its library directory.
// The payload is a plain copy of a MaaFramework runtime directory—maactl keeps
// no MaaFramework version of its own, it asks the loaded libraries.
//
// Embedding is opt-in through the "bundled" build tag, so the same source tree
// produces both a self-contained executable and a small one that loads
// MaaFramework from disk:
//
//	go build -tags bundled -o maactl.exe      ./cmd/maactl
//	go build               -o maactl-lite.exe ./cmd/maactl
package bundled

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"maactl/internal/platform"
)

// payloadDir is the embedded directory that tools/packmaafw populates. It
// always contains a README so that the directory is embeddable even when no
// payload was built in.
const payloadDir = "payload"

// mainLibrary is the library extracted last, so its presence proves the cache
// holds a complete extraction. It is the one library MaaFramework cannot be
// loaded without, whatever the platform spells it as.
func mainLibrary() string {
	return platform.Host().FrameworkLibrary()
}

// current reads the embedded payload once per process.
var current = sync.OnceValue(load)

type payload struct {
	container []byte
}

// Compiled reports whether this build was compiled to carry MaaFramework.
// The payload itself may still be missing, which Available reports.
func Compiled() bool {
	return compiled
}

// Available reports whether this build carries MaaFramework libraries.
func Available() bool {
	return len(current().container) > 0
}

// CacheID identifies the embedded payload so that libraries extracted from a
// different payload are never reused. It is derived from the payload bytes, so
// it changes exactly when the packed runtime does—no version has to be tracked
// for that.
func CacheID() string {
	return cacheID(current())
}

// cacheID derives the cache directory name of one payload: the host platform
// for readability, plus a digest of the payload that keeps two runtimes apart.
func cacheID(have payload) string {
	if len(have.container) == 0 {
		return ""
	}
	sum := sha256.Sum256(have.container)
	return platform.Host().ID() + "-" + hex.EncodeToString(sum[:8])
}

// Extract writes the embedded libraries into target and reports how many files
// were written. It does nothing when target already holds an extraction.
func Extract(target string) (int, error) {
	have := current()
	if len(have.container) == 0 {
		return 0, fmt.Errorf("this maactl build does not carry MaaFramework libraries")
	}
	return ExtractZip(have.container, target)
}

// ExtractZip extracts a container produced by tools/packmaafw into target,
// creating parent directories as needed. It reports how many files were written
// and writes nothing when target already holds an extraction.
func ExtractZip(container []byte, target string) (int, error) {
	mainName := mainLibrary()
	if fileExists(filepath.Join(target, mainName)) {
		return 0, nil
	}
	reader, err := zip.NewReader(bytes.NewReader(container), int64(len(container)))
	if err != nil {
		return 0, fmt.Errorf("read embedded MaaFramework payload: %w", err)
	}
	if !hasEntry(reader, mainName) {
		return 0, fmt.Errorf("embedded MaaFramework payload has no %s", mainName)
	}
	written := 0
	// Every other file is written before the main library, so an extraction
	// interrupted halfway is recognised as incomplete and redone on the next run.
	for _, main := range []bool{false, true} {
		for _, file := range reader.File {
			if strings.HasSuffix(file.Name, "/") || (file.Name == mainName) != main {
				continue
			}
			name, err := targetPath(target, file.Name)
			if err != nil {
				return written, err
			}
			if err := writeFile(name, file); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}

// readPayload collects the container from an embedded payload tree. Anything
// but the container archive is ignored: the payload carries no metadata, so
// there is no version or platform file to read or to keep in sync.
func readPayload(fsys fs.FS) payload {
	entries, err := fs.ReadDir(fsys, payloadDir)
	if err != nil {
		return payload{}
	}
	var result payload
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		if data, err := fs.ReadFile(fsys, path.Join(payloadDir, entry.Name())); err == nil {
			result.container = data
			break
		}
	}
	return result
}

// targetPath resolves an archive entry name below target, rejecting absolute
// paths and names that would escape target.
func targetPath(target, name string) (string, error) {
	relative := strings.TrimPrefix(path.Clean("/"+name), "/")
	if relative == "" || relative == "." {
		return "", fmt.Errorf("invalid payload entry %q", name)
	}
	return filepath.Join(target, filepath.FromSlash(relative)), nil
}

// writeFile writes one archive entry atomically: the data goes to a temporary
// file next to the destination and is renamed into place only once it is
// complete. An interrupted extraction therefore never leaves a half-written
// file behind, which matters most for the main library that doubles as the
// completeness marker of the cache directory.
func writeFile(name string, file *zip.File) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	reader, err := file.Open()
	if err != nil {
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	defer reader.Close()
	temp := name + ".tmp"
	writer, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, reader); err != nil {
		writer.Close()
		os.Remove(temp)
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	if err := writer.Close(); err != nil {
		os.Remove(temp)
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	// Renaming inside one directory is atomic, so the destination is either
	// the previous file or the complete new one.
	if err := os.Rename(temp, name); err != nil {
		os.Remove(temp)
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	return nil
}

func hasEntry(reader *zip.Reader, name string) bool {
	for _, file := range reader.File {
		if file.Name == name {
			return true
		}
	}
	return false
}

// fileExists reports whether name is an existing regular file.
func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}
