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

// TestConfigShowMasksOptionValues pins that only an env reference is echoed
// back: the client config may hold secrets as plaintext, so everything else is
// masked in both the text and the JSON form, whose shape stays the same.
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
