package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProtocolVersion is the ProjectInterface semantic version whose behavior this
// client implements. It is reported to agents through PI_INTERFACE_VERSION and
// used by `pi info`; it is distinct from the numeric `interface_version`.
const ProtocolVersion = "v2.10.1"

// ClientName is the identifier reported to agents through PI_CLIENT_NAME.
const ClientName = "MaaCtl"

// Load reads a ProjectInterface file or a project directory containing
// interface.json, resolves its imports, and merges them with the protocol's
// rules. Relative paths inside the PI resolve against the main file's directory.
func Load(path string) (*Loaded, error) {
	main, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	files, err := collect(main)
	if err != nil {
		return nil, err
	}
	merged := mergeFiles(files)
	if merged.InterfaceVersion != 2 {
		return nil, fmt.Errorf("%s: interface_version must be 2, got %d", main, merged.InterfaceVersion)
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.path
	}
	return &Loaded{
		ProjectInterface: *merged,
		Path:             main,
		Dir:              filepath.Dir(main),
		Files:            names,
	}, nil
}

// resolvePath turns a PI file or directory argument into an absolute file path.
// An empty argument means the current working directory's interface.json.
func resolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = filepath.Join(cwd, "interface.json")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("read interface: %w", err)
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "interface.json")
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("read interface: %w", err)
		}
	}
	return filepath.Clean(abs), nil
}

// resolveRelative resolves a PI-relative path against base. Windows-style
// backslashes are normalized first so a project written on Windows loads and
// validates identically on Linux and macOS; it must stay the single helper the
// loader, the translator, and the validator all use.
func resolveRelative(base, p string) string {
	return filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(p, `\`, "/")))
}

// fileEntry is one parsed PI file together with its absolute path.
type fileEntry struct {
	path string
	pi   *ProjectInterface
}

// collect returns the main file first, then every imported file in the order
// the protocol defines: for each import, the file itself before its own imports.
// Cyclic imports are reported instead of being silently truncated.
func collect(main string) ([]fileEntry, error) {
	var (
		entries  []fileEntry
		seen     = map[string]bool{}
		visiting = map[string]bool{}
	)
	var walk func(path string) error
	walk = func(path string) error {
		path = filepath.Clean(path)
		if visiting[path] {
			return fmt.Errorf("cyclic PI import: %s", path)
		}
		if seen[path] {
			// Already merged through another branch; imports are idempotent.
			return nil
		}
		parsed, err := parseFile(path)
		if err != nil {
			return err
		}
		seen[path] = true
		visiting[path] = true
		entries = append(entries, fileEntry{path: path, pi: parsed})
		base := filepath.Dir(path)
		for _, imported := range parsed.Import {
			if strings.TrimSpace(imported) == "" {
				continue
			}
			target := resolveRelative(base, imported)
			if err := walk(target); err != nil {
				return err
			}
		}
		delete(visiting, path)
		return nil
	}
	if err := walk(main); err != nil {
		return nil, err
	}
	return entries, nil
}

// parseFile reads one PI file through the JSONC preprocessor.
func parseFile(path string) (*ProjectInterface, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read PI file %s: %w", path, err)
	}
	var current ProjectInterface
	if err := json.Unmarshal(StripJSONC(b), &current); err != nil {
		return nil, fmt.Errorf("parse PI file %s: %w", path, err)
	}
	return &current, nil
}

// mergeFiles applies the protocol's import merge rules to the collected files.
//
//   - The main file provides every top-level field, including controllers and
//     resources; `import` cannot contribute those (the protocol only lists
//     task/option/preset/group/pretask/global_option/setting as importable).
//   - `task` / `preset` / `setting` / `pretask` are appended in file order.
//   - `group` and `global_option` are appended with first-occurrence dedup.
//   - `option` merges by key; a later definition wins but keeps its position.
func mergeFiles(files []fileEntry) *ProjectInterface {
	if len(files) == 0 {
		return &ProjectInterface{}
	}
	merged := *files[0].pi
	if merged.Option.items == nil {
		merged.Option = OptionMap{}
	}
	for _, file := range files[1:] {
		child := file.pi
		merged.Task = append(merged.Task, child.Task...)
		merged.Preset = append(merged.Preset, child.Preset...)
		merged.Setting = append(merged.Setting, child.Setting...)
		merged.Pretask = append(merged.Pretask, child.Pretask...)
		merged.Group = appendUniqueByName(merged.Group, child.Group)
		merged.GlobalOption = appendUniqueStrings(merged.GlobalOption, child.GlobalOption)
		merged.Option.Merge(&child.Option)
	}
	return &merged
}

// appendUniqueByName appends groups whose name is not present yet, keeping the
// first definition (protocol: group 按 name 去重，保留先出现的项).
func appendUniqueByName(dst, extra []Group) []Group {
	for _, item := range extra {
		duplicate := false
		for _, existing := range dst {
			if existing.Name == item.Name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, item)
		}
	}
	return dst
}

// appendUniqueStrings appends the values not present yet, keeping first
// occurrences (protocol: global_option 按 option 键名去重).
func appendUniqueStrings(dst, extra []string) []string {
	for _, item := range extra {
		duplicate := false
		for _, existing := range dst {
			if existing == item {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, item)
		}
	}
	return dst
}

// StripJSONC removes // and /* */ comments while preserving comment-like text
// inside JSON strings. An unterminated block comment is left in place so the
// JSON parser reports the real problem instead of a truncated document.
func StripJSONC(in []byte) []byte {
	in = bytes.TrimPrefix(in, []byte("\xEF\xBB\xBF"))
	var out bytes.Buffer
	out.Grow(len(in))
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
			for {
				if i >= len(in) {
					// Unterminated block comment: give the input back untouched.
					return in
				}
				if in[i] == '*' && i+1 < len(in) && in[i+1] == '/' {
					break
				}
				// Keep one newline per comment line so later error line numbers
				// still point at the real location.
				if in[i] == '\n' {
					out.WriteByte('\n')
				}
				i++
			}
			i++ // step onto '/'; the loop's post statement moves past it
			continue
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}
