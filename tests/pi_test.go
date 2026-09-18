package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"maactl/internal/pi"
)

// TestLoadMergesImportsAndAllowsComments verifies the loader resolves imports
// relative to the parent file and tolerates JSONC comments.
func TestLoadMergesImportsAndAllowsComments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "interface.json")
	// Language comment (JSONC) plus a block comment must both be ignored.
	writeFile(t, main, `{
		"interface_version": 2, // required version
		"name": "demo",
		"import": ["tasks/extra.json"],
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource/base"], "controller": ["Android"]}]
	}`)
	writeFile(t, filepath.Join(dir, "tasks", "extra.json"), `{
		"interface_version": 2,
		/* imported task */
		"task": [{"name": "daily", "entry": "DailyEntry", "resource": ["base"]}]
	}`)

	project, err := pi.Load(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if project.Dir != dir || len(project.Controller) != 1 || len(project.Resource) != 1 || len(project.Task) != 1 {
		t.Fatalf("unexpected project: %+v", project)
	}
	ctrl, err := project.FindController("")
	if err != nil || ctrl.Name != "Android" {
		t.Fatalf("find controller: %v, %+v", err, ctrl)
	}
	res, err := project.FindResource("", ctrl)
	if err != nil || res.Name != "base" {
		t.Fatalf("find resource: %v, %+v", err, res)
	}
	task, err := project.FindTask("daily", ctrl, res)
	if err != nil || task.Entry != "DailyEntry" {
		t.Fatalf("find task: %v, %+v", err, task)
	}
	if _, err := project.FindTask("missing", ctrl, res); err == nil {
		t.Error("expected missing task to fail")
	}
}

// TestLoadWin32Controller verifies the win32 controller block is parsed, since
// it carries the window selectors and input methods used at run time.
func TestLoadWin32Controller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, path, `{
		"interface_version": 2,
		"controller": [{
			"name": "PC",
			"type": "Win32",
			"win32": {
				"class_regex": "UnityWndClass",
				"window_regex": "原神",
				"mouse": "SendMessage",
				"keyboard": "PostMessage",
				"screencap": "FramePool"
			}
		}],
		"resource": [{"name": "base", "path": ["resource/base"], "controller": ["PC"]}]
	}`)
	project, err := pi.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ctrl, err := project.FindController("")
	if err != nil {
		t.Fatalf("find controller: %v", err)
	}
	if ctrl.Type != "Win32" || ctrl.Win32.ClassRegex != "UnityWndClass" || ctrl.Win32.WindowRegex != "原神" {
		t.Fatalf("unexpected win32 window config: %+v", ctrl.Win32)
	}
	if ctrl.Win32.Mouse != "SendMessage" || ctrl.Win32.Keyboard != "PostMessage" || ctrl.Win32.Screencap != "FramePool" {
		t.Errorf("unexpected win32 method config: %+v", ctrl.Win32)
	}
}

// TestLoadGamepadController verifies the gamepad controller block is parsed,
// including the optional screencap window and the virtual gamepad type.
func TestLoadGamepadController(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, path, `{
		"interface_version": 2,
		"controller": [{
			"name": "Pad",
			"type": "Gamepad",
			"gamepad": {
				"class_regex": "UnityWndClass",
				"window_regex": "原神",
				"gamepad_type": "DualShock4",
				"screencap": "FramePool"
			}
		}]
	}`)
	project, err := pi.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ctrl, err := project.FindController("")
	if err != nil {
		t.Fatalf("find controller: %v", err)
	}
	if ctrl.Type != "Gamepad" || ctrl.Gamepad.GamepadType != "DualShock4" {
		t.Fatalf("unexpected gamepad config: %+v", ctrl.Gamepad)
	}
	if ctrl.Gamepad.ClassRegex != "UnityWndClass" || ctrl.Gamepad.WindowRegex != "原神" || ctrl.Gamepad.Screencap != "FramePool" {
		t.Errorf("unexpected gamepad window config: %+v", ctrl.Gamepad)
	}
}

func TestLoadRejectsWrongInterfaceVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, path, `{"interface_version": 1}`)
	if _, err := pi.Load(path); err == nil {
		t.Error("expected interface_version 1 to be rejected")
	}
}

// TestLoadAgentForms verifies the PI `agent` field is accepted as a single
// object, as an array of objects, and as absent, including the optional
// identifier of the single-agent form.
func TestLoadAgentForms(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantExec   []string
		identifier string
	}{
		{name: "absent", body: `{"interface_version": 2}`},
		{name: "null", body: `{"interface_version": 2, "agent": null}`},
		{
			name:       "object",
			body:       `{"interface_version": 2, "agent": {"child_exec": "python", "child_args": ["./agent/main.py"], "identifier": "maactl-demo"}}`,
			wantExec:   []string{"python"},
			identifier: "maactl-demo",
		},
		{
			name:     "array",
			body:     `{"interface_version": 2, "agent": [{"child_exec": "python"}, {"child_exec": "node"}]}`,
			wantExec: []string{"python", "node"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "interface.json")
			writeFile(t, path, tc.body)
			project, err := pi.Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(project.Agent) != len(tc.wantExec) {
				t.Fatalf("got %d agents, want %d: %+v", len(project.Agent), len(tc.wantExec), project.Agent)
			}
			for i, exec := range tc.wantExec {
				if project.Agent[i].ChildExec != exec {
					t.Errorf("agent %d child_exec = %q, want %q", i, project.Agent[i].ChildExec, exec)
				}
			}
			if len(tc.wantExec) == 1 && project.Agent[0].Identifier != tc.identifier {
				t.Errorf("identifier = %q, want %q", project.Agent[0].Identifier, tc.identifier)
			}
		})
	}
}

// TestStripJSONCStripsBOM verifies a UTF-8 BOM (written by PowerShell 5.1 and
// Notepad) does not break JSON parsing.
func TestStripJSONCStripsBOM(t *testing.T) {
	in := append([]byte("\xEF\xBB\xBF"), []byte(`{"interface_version": 2}`)...)
	var value map[string]any
	if err := json.Unmarshal(pi.StripJSONC(in), &value); err != nil {
		t.Fatalf("BOM-prefixed JSON: %v", err)
	}
	if value["interface_version"] != float64(2) {
		t.Errorf("decoded = %#v", value)
	}
}

// TestStripJSONCPreservesBlockCommentLines verifies a multi-line block comment
// keeps one newline per comment line so later JSON error line numbers do not
// shift upwards.
func TestStripJSONCPreservesBlockCommentLines(t *testing.T) {
	in := []byte("{\n/* line 2\nline 3 */\n\"a\": }\n")
	out := pi.StripJSONC(in)
	if got, want := bytes.Count(out, []byte("\n")), bytes.Count(in, []byte("\n")); got != want {
		t.Fatalf("newlines = %d, want %d:\n%s", got, want, out)
	}
	var value any
	err := json.Unmarshal(out, &value)
	if err == nil {
		t.Fatal("expected a JSON error")
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		line := 1 + bytes.Count(out[:syntax.Offset-1], []byte("\n"))
		if line != 4 {
			t.Errorf("error reported on line %d, want 4: %v", line, err)
		}
	}
}

// TestStripJSONCKeepsUnterminatedBlockComment verifies a block comment with no
// closing */ is returned untouched instead of silently swallowing the rest of
// the document.
func TestStripJSONCKeepsUnterminatedBlockComment(t *testing.T) {
	in := []byte(`{"a": 1, /* never closed`)
	out := pi.StripJSONC(in)
	if !bytes.Equal(out, in) {
		t.Fatalf("unterminated comment should pass through unchanged:\n%s", out)
	}
	var value any
	if err := json.Unmarshal(out, &value); err == nil {
		t.Error("an unterminated block comment must still fail JSON parsing")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
