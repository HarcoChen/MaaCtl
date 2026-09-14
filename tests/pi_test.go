package tests

import (
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
