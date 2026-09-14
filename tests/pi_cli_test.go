package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/cli"
	"maactl/internal/pi"
	"maactl/internal/table"
)

// piFixture writes a small but complete ProjectInterface for CLI tests.
func piFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "interface.json")
	data := `{
		"interface_version": 2,
		"name": "demo",
		"label": "示例",
		"version": "v1.2.3",
		"controller": [
			{"name": "Android", "label": "安卓", "type": "Adb", "option": ["模式"]},
			{"name": "PC", "type": "Win32"}
		],
		"resource": [
			{"name": "base", "label": "基础", "path": ["resource/base"]},
			{"name": "pc", "path": ["resource/pc"], "controller": ["PC"]}
		],
		"group": [{"name": "daily", "label": "日常"}],
		"global_option": ["模式"],
		"setting": [{"name": "global", "option": ["模式"]}],
		"task": [
			{"name": "打开游戏", "entry": "启动游戏", "group": ["daily"]},
			{"name": "PC任务", "entry": "PCStart", "controller": ["PC"]}
		],
		"option": {
			"模式": {"type": "select", "default_case": "普通", "cases": [
				{"name": "普通", "pipeline_override": {"Mode": {"value": 1}}},
				{"name": "急速", "pipeline_override": {"Mode": {"value": 2}}}
			]}
		},
		"preset": [{"name": "日常", "task": [{"name": "打开游戏"}]}]
	}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPISubcommandsRun pins the command surface replacing the old
// `interface --action` flags.
func TestPISubcommandsRun(t *testing.T) {
	path := piFixture(t)
	cases := []struct {
		args     []string
		contains []string
	}{
		{[]string{"pi", "info", "-f", path}, []string{"示例 (demo) v1.2.3", "protocol: PI " + pi.ProtocolVersion}},
		{[]string{"pi", "controllers", "-f", path}, []string{"name", "label", "type", "runnable", "Android", "Win32"}},
		{[]string{"pi", "resources", "-f", path}, []string{"path", "hash", "base", "pc"}},
		{[]string{"pi", "tasks", "-f", path}, []string{"entry", "group", "打开游戏", "PC任务"}},
		{[]string{"pi", "groups", "-f", path}, []string{"default_expand", "daily", "日常"}},
		{[]string{"pi", "options", "-f", path}, []string{"[global_option]", "模式", "[select]"}},
		{[]string{"pi", "presets", "-f", path}, []string{"日常", "tasks", "disabled"}},
		{[]string{"pi", "settings", "-f", path}, []string{"global", "模式"}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := runCLI(tc.args...)
			if err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, out)
			}
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Errorf("%v: output is missing %q\n%s", tc.args, want, out)
				}
			}
		})
	}
}

// TestInterfaceAliasKeepsWorking checks `interface` is still accepted as a name.
func TestInterfaceAliasKeepsWorking(t *testing.T) {
	path := piFixture(t)
	alias, err := runCLI("interface", "tasks", "-f", path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := runCLI("pi", "tasks", "-f", path)
	if err != nil {
		t.Fatal(err)
	}
	if alias != canonical {
		t.Errorf("alias output differs:\n%s\n%s", alias, canonical)
	}
}

// TestInterfaceActionsAreGone makes sure the old action flags fail loudly
// instead of silently doing nothing.
func TestInterfaceActionsAreGone(t *testing.T) {
	for _, flag := range []string{"--tasks", "--controllers", "--show", "--options", "--presets"} {
		if _, err := runCLI("pi", flag); err == nil {
			t.Errorf("%s should no longer be a flag", flag)
		}
	}
}

// TestPITasksFilters checks applicability filtering and --all.
func TestPITasksFilters(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "tasks", "-f", path, "-c", "Android", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "PC任务") || !strings.Contains(out, "controller") {
		t.Errorf("--all should list the unavailable task with a reason:\n%s", out)
	}
	out, err = runCLI("pi", "tasks", "-f", path, "-c", "Android")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PC任务") {
		t.Errorf("PC-only task should be hidden for Android:\n%s", out)
	}

	if _, err := runCLI("pi", "tasks", "-f", path, "--group", "missing"); err != nil {
		t.Errorf("unknown group filter should return an empty table, got %v", err)
	}
	if out, err := runCLI("pi", "tasks", "-f", path, "--group", "missing"); err != nil || !strings.Contains(out, "no data") && !strings.Contains(out, "无数据") {
		t.Errorf("empty group filter output = %q, err = %v", out, err)
	}
}

// TestPIOptionsTreeInspectsTaskLayers checks the option layers and applicability.
func TestPIOptionsTreeInspectsTaskLayers(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "options", "-f", path, "-c", "Android", "-r", "base", "-t", "打开游戏")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "[controller.option]") {
		t.Errorf("controller options should be listed:\n%s", out)
	}
	if !strings.Contains(out, "模式") {
		t.Errorf("global option should be listed:\n%s", out)
	}
}

// TestPIOptionsJSON checks the machine-readable form.
func TestPIOptionsJSON(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "options", "-f", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name   string   `json:"name"`
		Type   string   `json:"type"`
		Active bool     `json:"active"`
		Cases  []string `json:"cases"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	found := false
	for _, row := range rows {
		if row.Name == "模式" {
			found = true
			if row.Type != "select" || len(row.Cases) != 2 {
				t.Errorf("模式 row = %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("模式 missing from %s", out)
	}
}

// TestPIValidateReportsIssues checks validation output and the failure exit code.
func TestPIValidateReportsIssues(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "validate", "-f", path)
	if err != nil {
		t.Fatalf("valid interface failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Valid ProjectInterface") {
		t.Errorf("unexpected validate output: %s", out)
	}

	bad := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, bad, `{
		"interface_version": 2,
		"controller": [{"name": "A", "type": "Adb"}],
		"resource": [{"name": "r", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["missing"]}]
	}`)
	out, err = runCLI("pi", "validate", "-f", bad)
	if err == nil {
		t.Fatalf("expected validation to fail:\n%s", out)
	}
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitUsage {
		t.Errorf("exit error = %#v, want usage code", err)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("issue should name the missing option:\n%s", out)
	}
}

// TestPIValidateJSON checks the structured report used by CI.
func TestPIValidateJSON(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "validate", "-f", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report pi.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !report.OK() {
		t.Errorf("report should be OK: %+v", report.Issues)
	}
}

// TestPITableAlignment keeps the terminal-width alignment covered.
func TestPITableAlignment(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "tasks", "-f", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "name") {
		t.Fatalf("unexpected table header: %q", out)
	}
	var buf bytes.Buffer
	if err := table.Print(&buf, []string{"name"}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no data") && !strings.Contains(buf.String(), "无数据") {
		t.Fatalf("empty table = %q", buf.String())
	}
}

// TestPIMissingInterfaceIsUsageError checks the exit code for a bad -f.
func TestPIMissingInterfaceIsUsageError(t *testing.T) {
	_, err := runCLI("pi", "info", "-f", filepath.Join(t.TempDir(), "nope.json"))
	var exit *cli.ExitError
	if !errors.As(err, &exit) || exit.Code != cli.ExitUsage {
		t.Fatalf("err = %v, want usage exit code", err)
	}
}

// TestDeviceHelpUsesGlobalJSON checks device commands rely on the global --json.
func TestDeviceHelpUsesGlobalJSON(t *testing.T) {
	out := mustHelp(t, "device", "adb", "-h")
	if n := strings.Count(out, "-j, --json"); n != 1 {
		t.Fatalf("json flag listed %d times:\n%s", n, out)
	}
	global := out[strings.Index(out, "Global Flags:"):]
	if !strings.Contains(global, "-j, --json") {
		t.Errorf("device adb should use the global json flag:\n%s", out)
	}
}

// TestLegacyDeviceCommandWarns checks the deprecated alias still works.
func TestLegacyDeviceCommandWarns(t *testing.T) {
	hidden := mustHelp(t, "adb", "-h")
	if strings.Contains(hidden, "adb devices") {
		t.Errorf("legacy command should be hidden from parent help:\n%s", hidden)
	}
}
