package clientconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMaaPiConfigStyleFile parses the format MFAA / MaaPiCli write,
// including an object-valued adb.config and UI-only keys.
func TestLoadMaaPiConfigStyleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maa_pi_config.json")
	body := `{
		"resource": "base",
		"controller": "Android",
		"adb": {
			"adb_path": "D:/MuMu/adb.exe",
			"address": "127.0.0.1:16384",
			"screencap": "64",
			"input": "18446744073709551607",
			"config": {"extras": {"mumu": {"enable": true, "index": 0}}}
		},
		"option": {"模式": "急速"},
		"task": [
			{"name": "签到", "option": {"账号": {"token": {"env": "TOKEN"}}}, "enabled": true, "__key": "ignored"}
		]
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.DefaultController() != "Android" || config.DefaultResource() != "base" {
		t.Errorf("defaults = %q / %q", config.DefaultController(), config.DefaultResource())
	}
	if config.Adb.Address != "127.0.0.1:16384" || config.Adb.Screencap != "64" {
		t.Errorf("adb = %+v", config.Adb)
	}
	if got := config.Adb.ConfigJSON(); got == "" || !strings.Contains(got, "mumu") {
		t.Errorf("adb config = %q", got)
	}
	if config.GlobalOptions()["模式"] != "急速" {
		t.Errorf("global options = %+v", config.GlobalOptions())
	}
	options := config.TaskOptions("签到")
	if options == nil {
		t.Fatal("task options missing")
	}
	if entry := config.TaskByName("签到"); entry == nil || entry.Enabled == nil || !*entry.Enabled {
		t.Errorf("task entry = %+v", entry)
	}
	if config.TaskOptions("missing") != nil {
		t.Errorf("unknown task should have no options")
	}
}

// TestDiscoverPrefersProjectConfig checks the search order and the tolerant
// behaviour for a missing file.
func TestDiscoverPrefersProjectConfig(t *testing.T) {
	piDir := t.TempDir()
	configDir := filepath.Join(piDir, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, FileName)
	if err := os.WriteFile(path, []byte(`{"resource":"from-project"}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, found, err := Discover(filepath.Join(piDir, "interface.json"))
	if err != nil {
		t.Fatal(err)
	}
	if config.DefaultResource() != "from-project" {
		t.Errorf("resource = %q", config.DefaultResource())
	}
	if found != path {
		t.Errorf("path = %q, want %q", found, path)
	}

	emptyDir := t.TempDir()
	config, candidate, err := Discover(filepath.Join(emptyDir, "interface.json"))
	if err != nil {
		t.Fatalf("missing config should not be an error: %v", err)
	}
	if config.DefaultResource() != "" || candidate == "" {
		t.Errorf("config = %+v, candidate = %q", config, candidate)
	}
}

// TestDiscoverReportsBrokenConfig makes sure a malformed file is surfaced
// instead of silently dropping the user's saved choices.
func TestDiscoverReportsBrokenConfig(t *testing.T) {
	piDir := t.TempDir()
	configDir := filepath.Join(piDir, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, FileName), []byte(`{"controller": 5}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Discover(filepath.Join(piDir, "interface.json")); err == nil {
		t.Fatal("expected a parse error")
	}
}
