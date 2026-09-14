// Package pi models and loads ProjectInterface v2 files.
package pi

// ProjectInterface is the subset of ProjectInterface v2 required to select and
// run a task. Unknown PI fields are deliberately retained by json.Unmarshal's
// forward-compatible behavior.
type ProjectInterface struct {
	InterfaceVersion int          `json:"interface_version"`
	Name             string       `json:"name"`
	Label            string       `json:"label"`
	Controller       []Controller `json:"controller"`
	Resource         []Resource   `json:"resource"`
	Task             []Task       `json:"task"`
	Import           []string     `json:"import"`
	Agent            *Agent       `json:"agent,omitempty"`
}

// Agent describes the ProjectInterface agent child process.
type Agent struct {
	ChildExec string   `json:"child_exec"`
	ChildArgs []string `json:"child_args"`
}

// Controller describes a PI controller entry.
type Controller struct {
	Name  string    `json:"name"`
	Label string    `json:"label"`
	Type  string    `json:"type"`
	Adb   AdbConfig `json:"adb"`
}

// AdbConfig holds the ADB screencap and input method names from a controller.
type AdbConfig struct {
	Screencap string `json:"screencap"`
	Input     string `json:"input"`
}

// Resource describes a PI resource entry.
type Resource struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Path       []string `json:"path"`
	Controller []string `json:"controller"`
	Hash       string   `json:"hash"`
}

// Task describes a PI task entry.
type Task struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Entry      string   `json:"entry"`
	Controller []string `json:"controller"`
	Resource   []string `json:"resource"`
	Override   any      `json:"pipeline_override"`
}

// Loaded is a parsed ProjectInterface together with its resolved location.
type Loaded struct {
	ProjectInterface
	Path string
	Dir  string
}
