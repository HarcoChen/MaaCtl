package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads a ProjectInterface file or a project directory containing
// interface.json, resolves its imports, and validates the interface version.
func Load(path string) (*Loaded, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(cwd, "interface.json")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read interface: %w", err)
	}
	if info.IsDir() {
		path = filepath.Join(path, "interface.json")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	pi, err := loadFile(abs, seen)
	if err != nil {
		return nil, err
	}
	if pi.InterfaceVersion != 2 {
		return nil, fmt.Errorf("%s: interface_version must be 2, got %d", abs, pi.InterfaceVersion)
	}
	return &Loaded{ProjectInterface: *pi, Path: abs, Dir: filepath.Dir(abs)}, nil
}

func loadFile(path string, seen map[string]bool) (*ProjectInterface, error) {
	path = filepath.Clean(path)
	if seen[path] {
		return nil, fmt.Errorf("cyclic PI import: %s", path)
	}
	seen[path] = true
	defer delete(seen, path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read PI file %s: %w", path, err)
	}
	var current ProjectInterface
	if err := json.Unmarshal(StripJSONC(b), &current); err != nil {
		return nil, fmt.Errorf("parse PI file %s: %w", path, err)
	}
	base := filepath.Dir(path)
	for _, imported := range current.Import {
		child, err := loadFile(filepath.Join(base, imported), seen)
		if err != nil {
			return nil, err
		}
		current.Task = append(current.Task, child.Task...)
		current.Controller = append(current.Controller, child.Controller...)
		current.Resource = append(current.Resource, child.Resource...)
	}
	return &current, nil
}

// StripJSONC removes // and /* */ comments while preserving comment-like text in JSON strings.
func StripJSONC(in []byte) []byte {
	var out bytes.Buffer
	inString, escaped := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inString {
			out.WriteByte(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(in) && in[i+1] == '/' {
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out.WriteByte('\n')
			}
			continue
		}
		if c == '/' && i+1 < len(in) && in[i+1] == '*' {
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
			continue
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}
