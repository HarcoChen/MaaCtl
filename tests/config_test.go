package tests

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigShowWithoutInterfaceSucceeds pins that a missing ProjectInterface
// does not turn `config show` into an error. The check goes through
// errors.Is(err, fs.ErrNotExist), so the result is the same whatever wording the
// operating system uses for a missing file.
func TestConfigShowWithoutInterfaceSucceeds(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "project", "interface.json")
	out, err := runCLI("config", "show", "-f", missing)
	if err != nil {
		t.Fatalf("config show with a missing interface = %v\n%s", err, out)
	}
	if !strings.Contains(out, "path:") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

// TestConfigShowMasksOptionValues pins the no-ProjectInterface case: nothing is
// declared, so no value can be shown to be an ordinary one, and only an env
// reference survives. Both the text and the JSON form keep their shape.
func TestConfigShowMasksOptionValues(t *testing.T) {
	config := filepath.Join(t.TempDir(), "maa_pi_config.json")
	writeFile(t, config, `{
		"controller": "Android",
		"option": {"password": "hunter2", "port": 1234, "proxy": {"env": "HTTP_PROXY"}},
		"task": [{"name": "t", "enabled": true, "option": {"api_key": "s3cr3t-value"}}]
	}`)
	out, err := runCLI("config", "show", "-cfg", config)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, wanted := range []string{"option.password = ******", "option.proxy = map[env:HTTP_PROXY]", "task t: enabled=true"} {
		if !strings.Contains(out, wanted) {
			t.Errorf("output is missing %q:\n%s", wanted, out)
		}
	}
	for _, secret := range []string{"hunter2", "1234", "s3cr3t-value"} {
		if strings.Contains(out, secret) {
			t.Errorf("output echoes %q:\n%s", secret, out)
		}
	}

	jsonOut, err := runCLI("config", "show", "-cfg", config, "-j")
	if err != nil {
		t.Fatalf("%v\n%s", err, jsonOut)
	}
	var view struct {
		Controller string         `json:"controller"`
		Option     map[string]any `json:"option"`
		Tasks      []struct {
			Name    string         `json:"name"`
			Enabled *bool          `json:"enabled"`
			Option  map[string]any `json:"option"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &view); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, jsonOut)
	}
	if view.Controller != "Android" || len(view.Option) != 3 {
		t.Errorf("view = %+v", view)
	}
	if view.Option["password"] != "******" || view.Option["port"] != "******" {
		t.Errorf("scalar option values must be masked: %+v", view.Option)
	}
	if proxy, ok := view.Option["proxy"].(map[string]any); !ok || proxy["env"] != "HTTP_PROXY" {
		t.Errorf("an env reference must survive: %+v", view.Option["proxy"])
	}
	if len(view.Tasks) != 1 || view.Tasks[0].Name != "t" || view.Tasks[0].Option["api_key"] != "******" {
		t.Errorf("task view = %+v", view.Tasks)
	}
}

// TestConfigShowMasksOnlyDeclaredPasswords pins the scoped rule: a field the
// ProjectInterface declares as a password is hidden, while the ordinary option
// values next to it stay readable, because `config show` exists to explain the
// choices in effect.
func TestConfigShowMasksOnlyDeclaredPasswords(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "interface.json"), `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E"}],
		"option": {
			"账号": {"type": "input", "inputs": [
				{"name": "user", "default": "someone"},
				{"name": "token", "password": true}
			]},
			"模式": {"type": "select", "cases": [{"name": "fast"}]}
		}
	}`)
	config := filepath.Join(dir, "maa_pi_config.json")
	writeFile(t, config, `{"option": {"账号": {"user": "alice", "token": "hunter2"}, "模式": "fast"}}`)

	out, err := runCLI("config", "show", "-f", filepath.Join(dir, "interface.json"), "-cfg", config)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("the declared password leaked:\n%s", out)
	}
	if !strings.Contains(out, "token:******") {
		t.Errorf("the declared password must be masked:\n%s", out)
	}
	for _, wanted := range []string{"user:alice", "option.模式 = fast"} {
		if !strings.Contains(out, wanted) {
			t.Errorf("output is missing %q:\n%s", wanted, out)
		}
	}
}
