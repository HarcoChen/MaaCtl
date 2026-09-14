// Package clientconfig reads the client-side ProjectInterface configuration
// file (maa_pi_config.json) shared by the MaaFramework client ecosystem.
//
// The file is not part of the ProjectInterface protocol; it is the de-facto
// convention used by MaaPiCli / MFAA to remember the user's controller,
// resource, device, and option choices. maactl reads it for defaults and never
// writes it back.
package clientconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"maactl/internal/pi"
)

// FileName is the conventional config file name.
const FileName = "maa_pi_config.json"

// Adb holds the saved ADB connection details. Config keeps the client's opaque
// extras object (for example MuMu's emulator index) as raw JSON, because
// MaaFramework expects it back verbatim as a JSON string.
type Adb struct {
	AdbPath   string          `json:"adb_path,omitempty"`
	Address   string          `json:"address,omitempty"`
	Screencap string          `json:"screencap,omitempty"`
	Input     string          `json:"input,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

// ConfigJSON returns the ADB extras as the JSON string MaaFramework wants.
func (a Adb) ConfigJSON() string {
	if len(a.Config) == 0 || string(a.Config) == "null" {
		return ""
	}
	return string(a.Config)
}

// Win32 holds the saved Win32 window selection and methods.
type Win32 struct {
	Handle      string `json:"hwnd,omitempty"`
	ClassRegex  string `json:"class_regex,omitempty"`
	WindowRegex string `json:"window_regex,omitempty"`
	Screencap   string `json:"screencap,omitempty"`
	Mouse       string `json:"mouse,omitempty"`
	Keyboard    string `json:"keyboard,omitempty"`
}

// Task holds the saved state of one task.
type Task struct {
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled,omitempty"`
	Option  map[string]any `json:"option,omitempty"`
}

// Config is the parsed client configuration. Unknown fields (such as MFAA's
// `__key`) are ignored on purpose.
type Config struct {
	Controller string         `json:"controller,omitempty"`
	Resource   string         `json:"resource,omitempty"`
	Adb        Adb            `json:"adb,omitempty"`
	Win32      Win32          `json:"win32,omitempty"`
	Option     map[string]any `json:"option,omitempty"`
	Task       []Task         `json:"task,omitempty"`

	// Path is the file the configuration was read from ("" when absent).
	Path string `json:"-"`
}

// Load reads a client configuration file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read client config %s: %w", path, err)
	}
	var config Config
	if err := json.Unmarshal(pi.StripJSONC(b), &config); err != nil {
		return nil, fmt.Errorf("parse client config %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		config.Path = abs
	} else {
		config.Path = path
	}
	return &config, nil
}

// Discover looks for the client configuration next to the ProjectInterface and
// in the current working directory. A missing file is not an error: the caller
// gets an empty config plus the path that was searched. A file that exists but
// cannot be parsed is an error, so a broken config is never silently ignored.
func Discover(piPath string) (*Config, string, error) {
	candidates := []string{}
	if piPath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(piPath), "config", FileName))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "config", FileName))
	}
	fallback := ""
	for _, candidate := range candidates {
		if fallback == "" {
			fallback = candidate
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			config, err := Load(candidate)
			if err != nil {
				return nil, candidate, err
			}
			return config, candidate, nil
		}
	}
	return &Config{}, fallback, nil
}

// TaskByName returns the saved entry for a task.
func (c *Config) TaskByName(name string) *Task {
	if c == nil {
		return nil
	}
	for i := range c.Task {
		if c.Task[i].Name == name {
			return &c.Task[i]
		}
	}
	return nil
}

// TaskOptions returns the saved option values for a task.
func (c *Config) TaskOptions(name string) map[string]any {
	if task := c.TaskByName(name); task != nil {
		return task.Option
	}
	return nil
}

// GlobalOptions returns the config's global option values.
func (c *Config) GlobalOptions() map[string]any {
	if c == nil {
		return nil
	}
	return c.Option
}

// DefaultController returns the saved controller name.
func (c *Config) DefaultController() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Controller)
}

// DefaultResource returns the saved resource name.
func (c *Config) DefaultResource() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Resource)
}
