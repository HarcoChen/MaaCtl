// Package bundled exposes the MaaFramework runtime that bundled maactl builds
// carry inside the executable.
//
// The payload directory is filled by tools/packmaafw at build time and read
// back here at run time: the container archive is unpacked into a cache
// directory, which is then handed to MaaFramework as its library directory.
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
)

// payloadDir is the embedded directory that tools/packmaafw populates. It
// always contains a README so that the directory is embeddable even when no
// payload was built in.
const payloadDir = "payload"

// mainDLL is extracted last, so its presence proves the cache holds a complete
// extraction.
const mainDLL = "MaaFramework.dll"

// current reads the embedded payload once per process.
var current = sync.OnceValue(load)

type payload struct {
	container []byte
	version   string
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

// Version returns the MaaFramework version of the embedded libraries, or ""
// when the build is unbundled or the version is unknown.
func Version() string {
	return current().version
}

// CacheID identifies the embedded payload so that libraries extracted from a
// different payload are never reused.
func CacheID() string {
	if version := sanitize(current().version); version != "" {
		return version
	}
	sum := sha256.Sum256(current().container)
	return hex.EncodeToString(sum[:8])
}

// Extract writes the embedded libraries into target and reports how many files
// were written. It does nothing when target already holds an extraction.
func Extract(target string) (int, error) {
	container := current().container
	if len(container) == 0 {
		return 0, fmt.Errorf("this maactl build does not carry MaaFramework libraries")
	}
	return ExtractZip(container, target)
}

// ExtractZip extracts a container produced by tools/packmaafw into target,
// creating parent directories as needed. It reports how many files were written
// and writes nothing when target already holds an extraction.
func ExtractZip(container []byte, target string) (int, error) {
	if fileExists(filepath.Join(target, mainDLL)) {
		return 0, nil
	}
	reader, err := zip.NewReader(bytes.NewReader(container), int64(len(container)))
	if err != nil {
		return 0, fmt.Errorf("read embedded MaaFramework payload: %w", err)
	}
	if !hasEntry(reader, mainDLL) {
		return 0, fmt.Errorf("embedded MaaFramework payload has no %s", mainDLL)
	}
	written := 0
	// Every other file is written before mainDLL, so an extraction interrupted
	// halfway is recognised as incomplete and redone on the next run.
	for _, main := range []bool{false, true} {
		for _, file := range reader.File {
			if strings.HasSuffix(file.Name, "/") || (file.Name == mainDLL) != main {
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

// readPayload collects the container and version from an embedded payload tree.
func readPayload(fsys fs.FS) payload {
	entries, err := fs.ReadDir(fsys, payloadDir)
	if err != nil {
		return payload{}
	}
	var result payload
	for _, entry := range entries {
		name := path.Join(payloadDir, entry.Name())
		switch {
		case strings.HasSuffix(entry.Name(), ".zip"):
			if data, err := fs.ReadFile(fsys, name); err == nil {
				result.container = data
			}
		case entry.Name() == "version.txt":
			if data, err := fs.ReadFile(fsys, name); err == nil {
				result.version = strings.TrimSpace(string(data))
			}
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

func writeFile(name string, file *zip.File) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	reader, err := file.Open()
	if err != nil {
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	defer reader.Close()
	writer, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, reader); err != nil {
		writer.Close()
		return fmt.Errorf("extract %s: %w", file.Name, err)
	}
	if err := writer.Close(); err != nil {
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

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

// sanitize keeps only characters that are safe in a directory name.
func sanitize(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".-")
}
