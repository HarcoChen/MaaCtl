package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/cli"
	"maactl/internal/pi"
	"maactl/internal/table"
)

// piFixtureBody is a small but complete ProjectInterface shared by the CLI
// tests.
const piFixtureBody = `{
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
			]},
			"未引用": {"type": "switch", "default_case": "No", "cases": [
				{"name": "Yes"}, {"name": "No"}
			]}
		},
		"preset": [{"name": "日常", "task": [{"name": "打开游戏"}]}]
	}`

// piFixture writes a small but complete ProjectInterface for CLI tests.
func piFixture(t *testing.T) string {
	t.Helper()
	return writeProject(t, piFixtureBody)
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
		{[]string{"resource", "list", "-f", path}, []string{"path", "hash", "base", "pc"}},
		{[]string{"pi", "tasks", "-f", path}, []string{"entry", "group", "打开游戏", "PC任务"}},
		{[]string{"pi", "groups", "-f", path}, []string{"default_expand", "daily", "日常"}},
		{[]string{"pi", "options", "-f", path}, []string{"[global_option]", "模式", "[select]"}},
		{[]string{"pi", "options", "-f", path, "--all"}, []string{"[option]", "未引用"}},
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

// TestPIOptionsAllListsUnreferencedOptionsOnce checks --all does not duplicate
// options that a layer already references.
func TestPIOptionsAllListsUnreferencedOptionsOnce(t *testing.T) {
	path := piFixture(t)
	out, err := runCLI("pi", "options", "-f", path, "-t", "打开游戏", "--all")
	if err != nil {
		t.Fatal(err)
	}
	layerSection := out[:strings.Index(out, "[option]")]
	definitionSection := out[strings.Index(out, "[option]"):]
	if !strings.Contains(layerSection, "模式") {
		t.Fatalf("the referenced option should appear in its layer:\n%s", out)
	}
	if strings.Contains(definitionSection, "模式") {
		t.Errorf("a referenced option must not appear under [option]:\n%s", out)
	}
	if !strings.Contains(definitionSection, "未引用") {
		t.Errorf("the unreferenced option should appear under [option]:\n%s", out)
	}
}

// TestPIOptionsAllKeepsReferencedOptionsUnique guards the `--all` listing: an
// option a layer references must never reappear under [option] with the bogus
// "not referenced by any layer" reason, even when it does not apply to the
// selected resource, and a self-referencing option must not gain a phantom
// extra level.
func TestPIOptionsAllKeepsReferencedOptionsUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interface.json")
	writeFile(t, path, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource/base"]}],
		"task": [{"name": "t", "entry": "E", "option": ["作战关卡"]}],
		"option": {
			"作战关卡": {"type": "select", "resource": ["Official"], "default_case": "普通", "cases": [{"name": "普通"}]},
			"未引用": {"type": "switch", "default_case": "No", "cases": [{"name": "Yes"}, {"name": "No"}]},
			"自引用": {"type": "select", "default_case": "a", "cases": [{"name": "a", "option": ["自引用"]}]}
		}
	}`)
	out, err := runCLI("pi", "options", "-f", path, "-t", "t", "--all", "-j")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var rows []struct {
		Layer  string `json:"layer"`
		Name   string `json:"name"`
		Depth  int    `json:"depth"`
		Active bool   `json:"active"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	counts := map[string]int{}
	maxDepth := map[string]int{}
	cycles := 0
	for _, row := range rows {
		counts[row.Name]++
		if row.Depth > maxDepth[row.Name] {
			maxDepth[row.Name] = row.Depth
		}
		if strings.Contains(row.Reason, "not referenced by any layer") {
			t.Errorf("option %q carries the wrong reason: %+v", row.Name, row)
		}
		if strings.Contains(row.Reason, "cyclic reference") {
			cycles++
		}
	}
	for name, want := range map[string]int{"作战关卡": 1, "未引用": 1, "自引用": 2} {
		if counts[name] != want {
			t.Errorf("option %q listed %d time(s), want %d:\n%s", name, counts[name], want, out)
		}
	}
	if maxDepth["自引用"] != 1 {
		t.Errorf("self-referencing option depth = %d, want 1:\n%s", maxDepth["自引用"], out)
	}
	if cycles != 1 {
		t.Errorf("cyclic-reference entries = %d, want 1:\n%s", cycles, out)
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

// TestPIQueryCommandsJSON pins the machine-readable shape of every read-only
// query command. Only `options` and `validate` were covered before, so a
// changed field name in any of these would slip through silently.
func TestPIQueryCommandsJSON(t *testing.T) {
	path := piFixture(t)

	decode := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := runCLI(args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}

	t.Run("info", func(t *testing.T) {
		var info struct {
			Name             string   `json:"name"`
			Label            string   `json:"label"`
			InterfaceVersion int      `json:"interface_version"`
			ProtocolVersion  string   `json:"protocol_version"`
			Controllers      int      `json:"controllers"`
			Resources        int      `json:"resources"`
			Tasks            int      `json:"tasks"`
			Groups           int      `json:"groups"`
			Options          int      `json:"options"`
			Presets          int      `json:"presets"`
			Settings         int      `json:"settings"`
			RunnableTypes    []string `json:"runnable_controller_types"`
			Telemetry        bool     `json:"telemetry"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "info", "-f", path, "-j")), &info); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if info.Name != "demo" || info.Label != "示例" || info.InterfaceVersion != 2 || info.ProtocolVersion != pi.ProtocolVersion {
			t.Errorf("info header = %+v", info)
		}
		if info.Controllers != 2 || info.Resources != 2 || info.Tasks != 2 || info.Groups != 1 || info.Options != 2 || info.Presets != 1 || info.Settings != 1 {
			t.Errorf("info counts = %+v", info)
		}
		if len(info.RunnableTypes) == 0 || info.Telemetry {
			t.Errorf("info runtime fields = %+v", info)
		}
	})

	t.Run("controllers", func(t *testing.T) {
		var rows []struct {
			Name      string `json:"name"`
			Label     string `json:"label"`
			Type      string `json:"type"`
			Runnable  bool   `json:"runnable"`
			Options   int    `json:"options"`
			Resources int    `json:"resources"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "controllers", "-f", path, "-j")), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(rows) != 2 || rows[0].Name != "Android" || rows[0].Label != "安卓" || rows[0].Type != "Adb" || rows[0].Options != 1 {
			t.Errorf("controller rows = %+v", rows)
		}
	})

	t.Run("tasks", func(t *testing.T) {
		var rows []struct {
			Name       string   `json:"name"`
			Label      string   `json:"label"`
			Entry      string   `json:"entry"`
			Group      []string `json:"group"`
			Options    int      `json:"options"`
			Compatible bool     `json:"compatible"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "tasks", "-f", path, "-j")), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(rows) != 2 || rows[0].Name != "打开游戏" || rows[0].Entry != "启动游戏" || len(rows[0].Group) != 1 || rows[0].Group[0] != "daily" || !rows[0].Compatible {
			t.Errorf("task rows = %+v", rows)
		}
	})

	t.Run("groups", func(t *testing.T) {
		var rows []struct {
			Name          string `json:"name"`
			Label         string `json:"label"`
			DefaultExpand bool   `json:"default_expand"`
			Tasks         int    `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "groups", "-f", path, "-j")), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		// A task with no group counts towards every group, since an empty
		// allow-list means "no restriction": daily holds 打开游戏 and PC任务.
		if len(rows) != 1 || rows[0].Name != "daily" || rows[0].Label != "日常" || rows[0].Tasks != 2 {
			t.Errorf("group rows = %+v", rows)
		}
	})

	t.Run("presets", func(t *testing.T) {
		var rows []struct {
			Name  string `json:"name"`
			Label string `json:"label"`
			Task  []struct {
				Name string `json:"name"`
			} `json:"task"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "presets", "-f", path, "-j")), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(rows) != 1 || rows[0].Name != "日常" || len(rows[0].Task) != 1 || rows[0].Task[0].Name != "打开游戏" {
			t.Errorf("preset rows = %+v", rows)
		}
	})

	t.Run("settings", func(t *testing.T) {
		var rows []struct {
			Name          string   `json:"name"`
			Label         string   `json:"label"`
			DefaultExpand bool     `json:"default_expand"`
			Option        []string `json:"option"`
		}
		if err := json.Unmarshal([]byte(decode(t, "pi", "settings", "-f", path, "-j")), &rows); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(rows) != 1 || rows[0].Name != "global" || len(rows[0].Option) != 1 || rows[0].Option[0] != "模式" {
			t.Errorf("setting rows = %+v", rows)
		}
	})
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
	global := out[strings.Index(out, "Global:"):]
	if !strings.Contains(global, "-j, --json") {
		t.Errorf("device adb should use the global json flag:\n%s", out)
	}
}

// TestResourceQueriesLiveTogether pins the grouping fix: everything about
// resources is under `resource`, and `pi` no longer has a resources command.
func TestResourceQueriesLiveTogether(t *testing.T) {
	if _, err := runCLI("pi", "resources"); err == nil {
		t.Error("`pi resources` should be gone; use `resource list`")
	}
	out := mustHelp(t, "resource", "-h")
	for _, want := range []string{"list, l", "inspect, i", "nodes, n", "hash, h"} {
		if !strings.Contains(out, want) {
			t.Errorf("resource help is missing %q\n%s", want, out)
		}
	}
}

// TestCommandAliasesEndToEnd exercises the mnemonic aliases through the CLI.
func TestCommandAliasesEndToEnd(t *testing.T) {
	path := piFixture(t)
	pairs := [][][]string{
		{{"pi", "t", "-f", path}, {"pi", "tasks", "-f", path}},
		{{"pi", "o", "-f", path}, {"pi", "options", "-f", path}},
		{{"resource", "l", "-f", path}, {"resource", "list", "-f", path}},
		{{"if", "i", "-f", path}, {"pi", "info", "-f", path}},
		{{"cfg", "p", "-f", path}, {"config", "path", "-f", path}},
	}
	for _, pair := range pairs {
		alias, err := runCLI(pair[0]...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", pair[0], err, alias)
		}
		canonical, err := runCLI(pair[1]...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", pair[1], err, canonical)
		}
		if alias != canonical {
			t.Errorf("%v and %v differ:\n%s\n%s", pair[0], pair[1], alias, canonical)
		}
	}
}

// TestFlagAliasesEndToEnd exercises multi-letter flag aliases.
func TestFlagAliasesEndToEnd(t *testing.T) {
	path := piFixture(t)
	alias, err := runCLI("pi", "t", "-if", path, "-c", "Android", "-all", "-j")
	if err != nil {
		t.Fatalf("%v\n%s", err, alias)
	}
	canonical, err := runCLI("pi", "t", "--interface", path, "--controller", "Android", "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if alias != canonical {
		t.Errorf("alias output differs:\n%s\n%s", alias, canonical)
	}
	if _, err := runCLI("pi", "v", "-if", path, "-st"); err == nil {
		t.Errorf("strict validate should fail on the fixture's missing paths")
	} else if !strings.Contains(err.Error(), "failed validation") {
		t.Errorf("strict validate error = %v", err)
	}
}

// TestLegacyDeviceCommandWarns checks the deprecated `adb devices` alias stays
// hidden from parent help but still warns on stderr before it does anything.
// The warning is printed before MaaFramework is touched, so this runs offline.
func TestLegacyDeviceCommandWarns(t *testing.T) {
	hidden := mustHelp(t, "adb", "-h")
	if strings.Contains(hidden, "adb devices") {
		t.Errorf("legacy command should be hidden from parent help:\n%s", hidden)
	}
	out, _ := runCLI("adb", "devices")
	if !strings.Contains(out, "deprecated") || !strings.Contains(out, "maactl device adb") {
		t.Errorf("legacy `adb devices` should warn on stderr:\n%s", out)
	}
}
