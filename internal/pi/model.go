// Package pi models and loads ProjectInterface v2 (PI) files.
//
// The model targets ProjectInterface protocol v2.10.1. Fields that maactl does
// not act on yet (telemetry, focus.trace) are still decoded so that "unknown
// field" never means "silently dropped by accident".
package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ProjectInterface is a fully merged PI: the main interface.json plus every
// file reachable through `import`, merged with the protocol's rules.
type ProjectInterface struct {
	InterfaceVersion int               `json:"interface_version"`
	Languages        map[string]string `json:"languages,omitempty"`
	Name             string            `json:"name"`
	Label            string            `json:"label,omitempty"`
	Title            string            `json:"title,omitempty"`
	Icon             string            `json:"icon,omitempty"`
	MirrorChyanRID   string            `json:"mirrorchyan_rid,omitempty"`
	MirrorChyanMulti bool              `json:"mirrorchyan_multiplatform,omitempty"`
	GitHub           string            `json:"github,omitempty"`
	Version          string            `json:"version,omitempty"`
	Contact          string            `json:"contact,omitempty"`
	License          string            `json:"license,omitempty"`
	Welcome          string            `json:"welcome,omitempty"`
	Description      string            `json:"description,omitempty"`
	Telemetry        *Telemetry        `json:"telemetry,omitempty"`
	Controller       []Controller      `json:"controller"`
	Resource         []Resource        `json:"resource"`
	Group            []Group           `json:"group,omitempty"`
	Pretask          Pretasks          `json:"pretask,omitempty"`
	Agent            Agents            `json:"agent,omitempty"`
	Task             []Task            `json:"task"`
	Option           OptionMap         `json:"option,omitempty"`
	GlobalOption     []string          `json:"global_option,omitempty"`
	Setting          []Setting         `json:"setting,omitempty"`
	Import           []string          `json:"import,omitempty"`
	Preset           []Preset          `json:"preset,omitempty"`
}

// Telemetry is PI's anonymous telemetry configuration. maactl does not report
// telemetry; the field is decoded so `pi info` can tell the user it exists.
type Telemetry struct {
	Sentry *SentryTelemetry `json:"sentry,omitempty"`
}

// SentryTelemetry holds the Sentry-specific telemetry settings.
type SentryTelemetry struct {
	DSN                          string  `json:"dsn,omitempty"`
	Tracing                      *bool   `json:"tracing,omitempty"`
	TracesSampleRate             float64 `json:"traces_sample_rate,omitempty"`
	FailureAttachmentsSampleRate float64 `json:"failure_attachments_sample_rate,omitempty"`
	Environment                  string  `json:"environment,omitempty"`
}

// Controller describes one PI controller entry.
type Controller struct {
	Name               string          `json:"name"`
	Label              string          `json:"label,omitempty"`
	Description        string          `json:"description,omitempty"`
	Icon               string          `json:"icon,omitempty"`
	Type               string          `json:"type"`
	DisplayShortSide   *int            `json:"display_short_side,omitempty"`
	DisplayLongSide    *int            `json:"display_long_side,omitempty"`
	DisplayExpand      []int           `json:"display_expand,omitempty"`
	DisplayRaw         bool            `json:"display_raw,omitempty"`
	PermissionRequired bool            `json:"permission_required,omitempty"`
	AttachResourcePath []string        `json:"attach_resource_path,omitempty"`
	Option             []string        `json:"option,omitempty"`
	Adb                AdbConfig       `json:"adb,omitempty"`
	Win32              Win32Config     `json:"win32,omitempty"`
	MacOS              MacOSConfig     `json:"macos,omitempty"`
	PlayCover          PlayCoverConfig `json:"playcover,omitempty"`
	Gamepad            GamepadConfig   `json:"gamepad,omitempty"`
	Linux              LinuxConfig     `json:"linux,omitempty"`
}

// AdbConfig holds the optional ADB method overrides. ProjectInterface v2 leaves
// method selection to MaaFramework, but older files still carry these fields.
type AdbConfig struct {
	Screencap string `json:"screencap,omitempty"`
	Input     string `json:"input,omitempty"`
}

// Win32Config holds a Win32 controller's window selectors and method overrides.
type Win32Config struct {
	ClassRegex  string `json:"class_regex,omitempty"`
	WindowRegex string `json:"window_regex,omitempty"`
	Mouse       string `json:"mouse,omitempty"`
	Keyboard    string `json:"keyboard,omitempty"`
	Screencap   string `json:"screencap,omitempty"`
}

// MacOSConfig holds a MacOS controller's window selector and method overrides.
type MacOSConfig struct {
	TitleRegex string `json:"title_regex,omitempty"`
	Screencap  string `json:"screencap,omitempty"`
	Input      string `json:"input,omitempty"`
}

// PlayCoverConfig holds the PlayCover application identifier.
type PlayCoverConfig struct {
	UUID string `json:"uuid,omitempty"`
}

// GamepadConfig holds a virtual gamepad's window selectors and type.
type GamepadConfig struct {
	ClassRegex  string `json:"class_regex,omitempty"`
	WindowRegex string `json:"window_regex,omitempty"`
	GamepadType string `json:"gamepad_type,omitempty"`
	Screencap   string `json:"screencap,omitempty"`
}

// LinuxConfig holds a Linux controller's method overrides.
type LinuxConfig struct {
	Screencap      string `json:"screencap,omitempty"`
	Input          string `json:"input,omitempty"`
	UseWin32VKCode bool   `json:"use_win32_vk_code,omitempty"`
	PipewireSource string `json:"pipewire_source,omitempty"`
}

// Resource describes one PI resource bundle.
type Resource struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Path        []string `json:"path"`
	Controller  []string `json:"controller,omitempty"`
	Option      []string `json:"option,omitempty"`
	Hash        string   `json:"hash,omitempty"`
}

// Group is a task grouping declared at the PI top level.
type Group struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	// DefaultExpand defaults to true when the field is absent.
	DefaultExpand *bool `json:"default_expand,omitempty"`
}

// Expands reports whether the group is expanded by default.
func (g Group) Expands() bool { return g.DefaultExpand == nil || *g.DefaultExpand }

// Task describes one PI task entry.
type Task struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Entry        string   `json:"entry"`
	DefaultCheck bool     `json:"default_check,omitempty"`
	Description  string   `json:"description,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Group        []string `json:"group,omitempty"`
	Resource     []string `json:"resource,omitempty"`
	Controller   []string `json:"controller,omitempty"`
	Override     any      `json:"pipeline_override,omitempty"`
	Option       []string `json:"option,omitempty"`
}

// Setting is one section of the PI settings page.
type Setting struct {
	Name          string   `json:"name"`
	Label         string   `json:"label,omitempty"`
	Description   string   `json:"description,omitempty"`
	Icon          string   `json:"icon,omitempty"`
	Option        []string `json:"option,omitempty"`
	DefaultExpand *bool    `json:"default_expand,omitempty"`
}

// Expands reports whether the section is expanded by default.
func (s Setting) Expands() bool { return s.DefaultExpand == nil || *s.DefaultExpand }

// Preset is a saved combination of task check states and option values.
type Preset struct {
	Name        string       `json:"name"`
	Label       string       `json:"label,omitempty"`
	Description string       `json:"description,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Task        []PresetTask `json:"task"`
}

// PresetTask is one task inside a preset. Enabled defaults to true; Option
// values follow the protocol's OptionValue shapes (string, []string, or an
// object for input/hotkey fields).
type PresetTask struct {
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled,omitempty"`
	Option  map[string]any `json:"option,omitempty"`
}

// EnabledOrDefault reports the task's check state inside a preset.
func (p PresetTask) EnabledOrDefault() bool { return p.Enabled == nil || *p.Enabled }

// OptionType is a PI option kind.
type OptionType string

// Supported PI option types.
const (
	OptionTypeSelect   OptionType = "select"
	OptionTypeCheckbox OptionType = "checkbox"
	OptionTypeInput    OptionType = "input"
	OptionTypeHotkey   OptionType = "hotkey"
	OptionTypeSwitch   OptionType = "switch"
)

// Option is one PI configuration item definition.
type Option struct {
	Name        string         `json:"-"`
	Type        OptionType     `json:"type,omitempty"`
	Controller  []string       `json:"controller,omitempty"`
	Resource    []string       `json:"resource,omitempty"`
	Label       string         `json:"label,omitempty"`
	Description string         `json:"description,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Cases       []OptionCase   `json:"cases,omitempty"`
	Inputs      []OptionInput  `json:"inputs,omitempty"`
	Hotkeys     []OptionHotkey `json:"hotkeys,omitempty"`
	DefaultCase *DefaultCase   `json:"default_case,omitempty"`
	MinCount    *int           `json:"min_count,omitempty"`
	MaxCount    *int           `json:"max_count,omitempty"`
	Override    any            `json:"pipeline_override,omitempty"`
}

// Kind returns the effective option type; the protocol defaults to "select".
func (o *Option) Kind() OptionType {
	if o.Type == "" {
		return OptionTypeSelect
	}
	return o.Type
}

// CaseNames returns the case names in declaration order.
func (o *Option) CaseNames() []string {
	names := make([]string, len(o.Cases))
	for i := range o.Cases {
		names[i] = o.Cases[i].Name
	}
	return names
}

// FindCase returns the case with the given name.
func (o *Option) FindCase(name string) (*OptionCase, bool) {
	for i := range o.Cases {
		if o.Cases[i].Name == name {
			return &o.Cases[i], true
		}
	}
	return nil, false
}

// OptionCase is one selectable value of a select/checkbox/switch option.
type OptionCase struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Option      []string `json:"option,omitempty"`
	Override    any      `json:"pipeline_override,omitempty"`
}

// OptionInput is one input field of an input option.
type OptionInput struct {
	Name         string `json:"name"`
	Label        string `json:"label,omitempty"`
	Description  string `json:"description,omitempty"`
	Default      string `json:"default,omitempty"`
	PipelineType string `json:"pipeline_type,omitempty"`
	Verify       string `json:"verify,omitempty"`
	PatternMsg   string `json:"pattern_msg,omitempty"`
	Password     bool   `json:"password,omitempty"`
}

// OptionHotkey is one hotkey field of a hotkey option.
type OptionHotkey struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
}

// DefaultCase accepts either a single case name (select/switch) or a list of
// case names (checkbox). A nil *DefaultCase means the field was absent.
type DefaultCase struct {
	Scalar string
	List   []string
}

// String returns the scalar default, or "" when the default is a list or absent.
func (d *DefaultCase) String() string {
	if d == nil {
		return ""
	}
	return d.Scalar
}

// Slice returns the list default, or nil when the default is a scalar or absent.
func (d *DefaultCase) Slice() []string {
	if d == nil {
		return nil
	}
	return d.List
}

// UnmarshalJSON accepts a string or an array of strings.
func (d *DefaultCase) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*d = DefaultCase{}
		return nil
	}
	if trimmed[0] == '[' {
		var list []string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*d = DefaultCase{List: list}
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*d = DefaultCase{Scalar: single}
	return nil
}

// MarshalJSON emits the same shape the file used.
func (d DefaultCase) MarshalJSON() ([]byte, error) {
	if d.List != nil {
		return json.Marshal(d.List)
	}
	return json.Marshal(d.Scalar)
}

// OptionMap is an insertion-ordered map of option definitions. Order matters:
// the protocol uses the `option` member order for UI display, and `pi options`
// preserves it.
type OptionMap struct {
	names []string
	items map[string]*Option
}

// NewOptionMap returns an empty ordered option map.
func NewOptionMap() *OptionMap { return &OptionMap{items: map[string]*Option{}} }

// Len returns the number of option definitions.
func (m *OptionMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.names)
}

// Names returns the option names in declaration order.
func (m *OptionMap) Names() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.names...)
}

// Get returns the option with the given name.
func (m *OptionMap) Get(name string) (*Option, bool) {
	if m == nil || m.items == nil {
		return nil, false
	}
	item, ok := m.items[name]
	return item, ok
}

// Set inserts or replaces an option, keeping the position of an existing key.
func (m *OptionMap) Set(name string, item *Option) {
	if m.items == nil {
		m.items = map[string]*Option{}
	}
	item.Name = name
	if _, exists := m.items[name]; !exists {
		m.names = append(m.names, name)
	}
	m.items[name] = item
}

// Merge merges another option map: later keys win, first position is kept.
func (m *OptionMap) Merge(other *OptionMap) {
	if other == nil {
		return
	}
	for _, name := range other.names {
		m.Set(name, other.items[name])
	}
}

// UnmarshalJSON decodes an option object while preserving member order.
func (m *OptionMap) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("option: expected an object, got %s", string(trimmed))
	}
	m.items = map[string]*Option{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("option: expected a string key")
		}
		var item Option
		if err := dec.Decode(&item); err != nil {
			return fmt.Errorf("option %q: %w", name, err)
		}
		m.Set(name, &item)
	}
	_, err = dec.Token() // closing brace
	return err
}

// MarshalJSON emits the option entries in declaration order.
func (m OptionMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, name := range m.names {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := json.Marshal(m.items[name])
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Agent describes one ProjectInterface agent: the child process that runs an
// AgentServer and the identifier used for its socket.
type Agent struct {
	ChildExec  string   `json:"child_exec"`
	ChildArgs  []string `json:"child_args,omitempty"`
	Identifier string   `json:"identifier,omitempty"`
}

// Agents holds the PI `agent` field, which the protocol allows as either a
// single object or an array of objects.
type Agents []Agent

// UnmarshalJSON accepts both the single-object and the array form of `agent`.
func (a *Agents) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*a = nil
		return nil
	}
	if trimmed[0] == '[' {
		var list []Agent
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*a = list
		return nil
	}
	var single Agent
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*a = Agents{single}
	return nil
}

// Pretask describes one pre-controller program run before the controller
// connects.
type Pretask struct {
	Resource    []string `json:"resource,omitempty"`
	Controller  []string `json:"controller,omitempty"`
	Exec        string   `json:"exec"`
	Args        []string `json:"args,omitempty"`
	Name        string   `json:"name,omitempty"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Option      []string `json:"option,omitempty"`
}

// Identifier returns the pretask's stable name, falling back to exec.
func (p Pretask) Identifier() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Exec
}

// Pretasks holds the PI `pretask` field, accepted as a single object or array.
type Pretasks []Pretask

// UnmarshalJSON accepts both the single-object and the array form of `pretask`.
func (p *Pretasks) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*p = nil
		return nil
	}
	if trimmed[0] == '[' {
		var list []Pretask
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*p = list
		return nil
	}
	var single Pretask
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*p = Pretasks{single}
	return nil
}

// Loaded is a parsed and merged ProjectInterface together with its location.
type Loaded struct {
	ProjectInterface
	// Path is the absolute path of the main interface.json.
	Path string
	// Dir is the directory containing Path; relative PI paths resolve here.
	Dir string
	// Files lists every PI file that contributed, in merge order.
	Files []string
	// translators caches the per-language translators built by Translator().
	translators map[string]*Translator
}
