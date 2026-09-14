// Package pi models and loads ProjectInterface v2 files.
package pi

import (
	"bytes"
	"encoding/json"
)

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
	Agent            Agents       `json:"agent,omitempty"`
}

// Agent describes one ProjectInterface agent: the child process that runs an
// AgentServer and the identifier used for its socket.
type Agent struct {
	ChildExec  string   `json:"child_exec"`
	ChildArgs  []string `json:"child_args"`
	Identifier string   `json:"identifier"`
}

// Agents holds the PI `agent` field, which the protocol allows as either a
// single object or an array of objects. Multiple agents run as separate child
// processes connected to the same resource.
type Agents []Agent

// UnmarshalJSON accepts both the single-object and the array form of `agent`,
// and leaves null entries empty so callers can detect "no agent declared".
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
