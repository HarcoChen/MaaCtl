package tests

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/pi"
)

// sampleProject loads the MaaFramework reference interface.json, which exercises
// every option kind and the preset format.
func sampleProject(t *testing.T) *pi.Loaded {
	t.Helper()
	project, err := pi.Load(filepath.Join("..", "maafw", "sample", "interface.json"))
	if err != nil {
		t.Fatalf("load sample: %v", err)
	}
	return project
}

// TestSampleInterfaceOptionResolution resolves a task with a checkbox, a
// select, and a switch, and checks both the audit trail and the merged override.
func TestSampleInterfaceOptionResolution(t *testing.T) {
	project := sampleProject(t)
	resolution, err := project.Resolve(pi.Request{
		ControllerName: "Android",
		ResourceName:   "Official",
		Task:           project.LookupTask("常规作战"),
		CLI: map[string]any{
			"作战关卡":   "4-20 厄险（双头形骨架）",
			"复现次数":   "x4",
			"刷完全部体力": "Yes",
			"战斗划火柴": []string{
				"普通划火柴",
				"连续划火柴",
			},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	want := map[string]map[string]any{
		"EnterTheShow":    {"next": "MainChapter_4"},
		"TargetStageName": {"expected": "20"},
		"StageDifficulty": {"next": "StageDifficulty_Hard"},
		"SetReplaysTimes": {"expected": "4"},
		"AllIn":           {"enabled": true},
		"NormalMatch":     {"enabled": true},
		"ComboMatch":      {"enabled": true},
		// One value per option name: the checkbox selection applies to every
		// layer that references 战斗划火柴, so the unselected case's node stays
		// untouched even though global_option references the option too.
		"ChargedMatch":    nil,
		"AutoDodge":       {"enabled": false},
		"UseSanityPotion": nil, // nested option is not activated
	}
	for node, fields := range want {
		got, ok := resolution.Override[node]
		if fields == nil {
			if ok {
				t.Errorf("node %s should not be overridden: %+v", node, got)
			}
			continue
		}
		object, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("node %s = %#v", node, got)
		}
		for field, value := range fields {
			if object[field] != value {
				t.Errorf("node %s field %s = %#v, want %#v", node, field, object[field], value)
			}
		}
	}

	if selection, ok := findSelection(resolution, "刷完全部体力"); !ok || selection.Source != pi.SourceCLI || len(selection.Cases) != 1 || selection.Cases[0] != "Yes" {
		t.Errorf("刷完全部体力 selection = %+v (found %v)", selection, ok)
	}
	// The option value is exposed in the protocol's OptionValue shape.
	if value, ok := resolution.Value("战斗划火柴"); !ok {
		t.Errorf("战斗划火柴 missing from resolution")
	} else if list, ok := value.([]string); !ok || len(list) != 2 {
		t.Errorf("战斗划火柴 value = %#v", value)
	}
}

// TestSampleInterfaceApplicabilityFilter checks that an option restricted to
// another resource produces no override and is reported as skipped.
func TestSampleInterfaceApplicabilityFilter(t *testing.T) {
	project := sampleProject(t)
	resolution, err := project.Resolve(pi.Request{
		ControllerName: "Android",
		ResourceName:   "Bilibili",
		Task:           project.LookupTask("常规作战"),
		CLI: map[string]any{
			"复现次数":   "x1",
			"刷完全部体力": "No",
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, ok := resolution.Override["EnterTheShow"]; ok {
		t.Errorf("作战关卡 is Official-only and must not contribute: %+v", resolution.Override["EnterTheShow"])
	}
	found := false
	for _, skipped := range resolution.Skipped {
		if skipped.Name == "作战关卡" {
			found = true
		}
	}
	if !found {
		t.Errorf("作战关卡 should be reported as skipped: %+v", resolution.Skipped)
	}
}

// TestPresetResolution applies the sample's "ALL IN" preset.
func TestPresetResolution(t *testing.T) {
	project := sampleProject(t)
	preset, err := project.FindPreset("ALL IN")
	if err != nil {
		t.Fatal(err)
	}
	entry := preset.Task[0]
	if entry.Name != "常规作战" {
		t.Fatalf("unexpected preset entry: %+v", entry)
	}
	resolution, err := project.Resolve(pi.Request{
		ControllerName: "Android",
		ResourceName:   "Official",
		Task:           project.LookupTask(entry.Name),
		PresetOptions:  entry.Option,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if object, ok := resolution.Override["AllIn"].(map[string]any); !ok || object["enabled"] != true {
		t.Errorf("AllIn = %#v", resolution.Override["AllIn"])
	}
	if selection, ok := findSelection(resolution, "复现次数"); !ok || selection.Source != pi.SourcePreset || selection.Cases[0] != "x4" {
		t.Errorf("复现次数 selection = %+v (found %v)", selection, ok)
	}
}

// TestValueSourcePrecedence pins the config < preset < option-file < cli order.
func TestValueSourcePrecedence(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["模式"]}],
		"option": {"模式": {"type": "select", "cases": [
			{"name": "默认"}, {"name": "配置"}, {"name": "预设"}, {"name": "文件"}, {"name": "命令行"}
		]}}
	}`)

	request := pi.Request{ControllerName: "Android", ResourceName: "base", Task: task}
	if _, err := project.Resolve(request); err == nil {
		t.Fatal("an option with no default_case and no value should fail")
	}

	request.ConfigGlobal = map[string]any{"模式": "配置"}
	resolution := resolveOK(t, project, request)
	assertCaseSource(t, resolution, "模式", "配置", pi.SourceConfig)

	request.PresetOptions = map[string]any{"模式": "预设"}
	resolution = resolveOK(t, project, request)
	assertCaseSource(t, resolution, "模式", "预设", pi.SourcePreset)

	request.OptionFile = map[string]any{"模式": "文件"}
	resolution = resolveOK(t, project, request)
	assertCaseSource(t, resolution, "模式", "文件", pi.SourceOptionFile)

	request.CLI = map[string]any{"模式": "命令行"}
	resolution = resolveOK(t, project, request)
	assertCaseSource(t, resolution, "模式", "命令行", pi.SourceCLI)
}

// TestOptionMissingValueFails makes sure a select without default_case and
// without a caller value is an error rather than a silent empty choice.
func TestOptionMissingValueFails(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["模式"]}],
		"option": {"模式": {"type": "select", "cases": [{"name": "A"}]}}
	}`)
	if _, err := project.Resolve(pi.Request{ControllerName: "Android", ResourceName: "base", Task: task}); err == nil {
		t.Fatal("expected a missing select value to fail")
	}
}

// TestCheckboxOrderAndCounts verifies definition-order merging plus the
// min_count / max_count constraints.
func TestCheckboxOrderAndCounts(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["功能"]}],
		"option": {"功能": {
			"type": "checkbox", "min_count": 1, "max_count": 2,
			"cases": [
				{"name": "a", "pipeline_override": {"Log": {"order": ["a"]}}},
				{"name": "b", "pipeline_override": {"Log": {"order": ["b"]}}},
				{"name": "c", "pipeline_override": {"Log": {"order": ["c"]}}}
			]
		}}
	}`)

	// Requested in reverse order on purpose: the protocol merges by case order.
	resolution := resolveOK(t, project, pi.Request{
		ControllerName: "Android", ResourceName: "base", Task: task,
		CLI: map[string]any{"功能": []string{"c", "a"}},
	})
	order, _ := resolution.Override["Log"].(map[string]any)["order"].([]any)
	if len(order) != 1 || order[0] != "c" {
		// Each case replaces the whole `order` array; the last in case order wins.
		t.Errorf("merged Log.order = %#v", order)
	}
	if selection, ok := findSelection(resolution, "功能"); !ok || strings.Join(selection.Cases, ",") != "a,c" {
		t.Errorf("checkbox cases = %+v", selection.Cases)
	}

	if _, err := project.Resolve(pi.Request{ControllerName: "Android", ResourceName: "base", Task: task, CLI: map[string]any{"功能": []string{}}}); err == nil {
		t.Error("expected min_count=1 to reject an empty selection")
	}
	if _, err := project.Resolve(pi.Request{ControllerName: "Android", ResourceName: "base", Task: task, CLI: map[string]any{"功能": []string{"a", "b", "c"}}}); err == nil {
		t.Error("expected max_count=2 to reject three selections")
	}
}

// TestInputTemplateTypes checks that whole-string placeholders keep the type
// declared by pipeline_type.
func TestInputTemplateTypes(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["关卡"]}],
		"option": {"关卡": {
			"type": "input",
			"inputs": [
				{"name": "章节号", "default": "4"},
				{"name": "超时", "default": "20000", "pipeline_type": "int"},
				{"name": "启用", "default": "true", "pipeline_type": "bool"},
				{"name": "校验", "default": "123", "verify": "^\\d+$", "pattern_msg": "要数字"}
			],
			"pipeline_override": {
				"Node": {
					"next": "MainChapter_{章节号}",
					"timeout": "{超时}",
					"enabled": "{启用}",
					"label": "关卡 {章节号}（{超时}）"
				}
			}
		}}
	}`)
	resolution := resolveOK(t, project, pi.Request{ControllerName: "Android", ResourceName: "base", Task: task})
	node, _ := resolution.Override["Node"].(map[string]any)
	if node["next"] != "MainChapter_4" {
		t.Errorf("next = %#v", node["next"])
	}
	if node["timeout"] != int64(20000) {
		t.Errorf("timeout = %#v (%T)", node["timeout"], node["timeout"])
	}
	if node["enabled"] != true {
		t.Errorf("enabled = %#v (%T)", node["enabled"], node["enabled"])
	}
	if node["label"] != "关卡 4（20000）" {
		t.Errorf("label = %#v", node["label"])
	}

	// verify + pattern_msg drive the error text.
	_, err := project.Resolve(pi.Request{ControllerName: "Android", ResourceName: "base", Task: task, CLI: map[string]any{"关卡": map[string]any{"校验": "xyz"}}})
	if err == nil || !strings.Contains(err.Error(), "要数字") {
		t.Errorf("verify error = %v", err)
	}
}

// TestNestedOptionsActivateWithCase walks `case.option` recursion.
func TestNestedOptionsActivateWithCase(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["关卡"]}],
		"option": {
			"关卡": {
				"type": "select",
				"cases": [
					{"name": "3-9", "pipeline_override": {"Enter": {"next": "Ch3"}}, "option": ["使用药"]},
					{"name": "4-20", "pipeline_override": {"Enter": {"next": "Ch4"}}}
				]
			},
			"使用药": {"type": "switch", "default_case": "No", "cases": [
				{"name": "Yes", "pipeline_override": {"Ch3": {"enabled": true}}},
				{"name": "No", "pipeline_override": {"Ch3": {"enabled": false}}}
			]}
		}
	}`)
	resolution := resolveOK(t, project, pi.Request{
		ControllerName: "Android", ResourceName: "base", Task: task,
		CLI: map[string]any{"关卡": "3-9", "使用药": "Yes"},
	})
	if node, _ := resolution.Override["Enter"].(map[string]any); node["next"] != "Ch3" {
		t.Errorf("Enter = %#v", node)
	}
	if node, _ := resolution.Override["Ch3"].(map[string]any); node["enabled"] != true {
		t.Errorf("nested option did not apply: %#v", node)
	}
	if selection, ok := findSelection(resolution, "使用药"); !ok || selection.Parent != "关卡" || selection.Depth != 1 {
		t.Errorf("nested selection = %+v (found %v)", selection, ok)
	}

	// The nested option is not evaluated when its parent case is not selected.
	resolution = resolveOK(t, project, pi.Request{
		ControllerName: "Android", ResourceName: "base", Task: task,
		CLI: map[string]any{"关卡": "4-20"},
	})
	if _, ok := resolution.Override["Ch3"]; ok {
		t.Errorf("nested option of an unselected case must not apply")
	}
}

// TestHotkeyMapping checks hotkey placeholders and controller-specific codes.
func TestHotkeyMapping(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "PC", "type": "Win32"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["改键"]}],
		"option": {"改键": {
			"type": "hotkey",
			"hotkeys": [{"name": "Skill", "default": "E"}],
			"pipeline_override": {"__ClickKey": {"key": "{Skill.primary}"}, "__Mod": {"key": "{Skill.modifier1}"}}
		}}
	}`)
	resolution := resolveOK(t, project, pi.Request{
		ControllerName: "PC", ResourceName: "base", Task: task,
		CLI: map[string]any{"改键": map[string]any{"Skill": "Ctrl+A"}},
	})
	if node, _ := resolution.Override["__ClickKey"].(map[string]any); node["key"] != 0x41 {
		t.Errorf("primary key = %#v, want 0x41", node["key"])
	}
	if node, _ := resolution.Override["__Mod"].(map[string]any); node["key"] != 0x11 {
		t.Errorf("modifier key = %#v, want 0x11", node["key"])
	}
}

// TestPasswordFields verifies env references, masking, and the CLI rejection.
func TestPasswordFields(t *testing.T) {
	project, task := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E", "option": ["账号"]}],
		"option": {"账号": {
			"type": "input",
			"inputs": [
				{"name": "user", "default": "someone"},
				{"name": "token", "password": true}
			],
			"pipeline_override": {"Login": {"custom_action_param": {"user": "{user}", "token": "{token}"}}}
		}}
	}`)
	t.Setenv("MAACTL_TEST_TOKEN", "s3cr3t")

	resolution := resolveOK(t, project, pi.Request{
		ControllerName: "Android", ResourceName: "base", Task: task,
		OptionFile: map[string]any{"账号": map[string]any{"token": map[string]any{"env": "MAACTL_TEST_TOKEN"}}},
	})
	param, _ := resolution.Override["Login"].(map[string]any)["custom_action_param"].(map[string]any)
	if param["token"] != "s3cr3t" || param["user"] != "someone" {
		t.Fatalf("override = %#v", param)
	}
	selection, ok := findSelection(resolution, "账号")
	if !ok || !selection.Secrets["token"] {
		t.Fatalf("secrets = %+v (found %v)", selection.Secrets, ok)
	}
	masked, _ := selection.MaskedValue().(map[string]string)
	if masked["token"] != "******" || masked["user"] != "someone" {
		t.Errorf("masked = %#v", masked)
	}

	// The downlink value stays in clear, but every display copy must be masked.
	maskedOverride, err := json.Marshal(resolution.MaskedOverride())
	if err != nil {
		t.Fatalf("marshal masked override: %v", err)
	}
	if strings.Contains(string(maskedOverride), "s3cr3t") {
		t.Errorf("MaskedOverride leaks the password: %s", maskedOverride)
	}
	if !strings.Contains(string(maskedOverride), "******") {
		t.Errorf("MaskedOverride should contain the mask: %s", maskedOverride)
	}
	for _, contribution := range resolution.Contributions {
		b, err := json.Marshal(contribution.MaskedOverride())
		if err != nil {
			t.Fatalf("marshal masked contribution: %v", err)
		}
		if strings.Contains(string(b), "s3cr3t") {
			t.Errorf("contribution %s leaks the password: %s", contribution.Label, b)
		}
	}

	// Passing a password with --option must be refused.
	if _, err := project.Resolve(pi.Request{
		ControllerName: "Android", ResourceName: "base", Task: task,
		CLI: map[string]any{"账号": map[string]any{"token": "plain"}},
	}); err == nil || !strings.Contains(err.Error(), "passwords cannot be passed") {
		t.Errorf("cli password error = %v", err)
	}

	// A missing password with no reference is reported.
	if _, err := project.Resolve(pi.Request{ControllerName: "Android", ResourceName: "base", Task: task}); err == nil {
		t.Error("expected a missing password to fail")
	}
}

// TestHotkeyKeyCodeSnapshot pins the adb and win32 key codes so a stray edit
// cannot silently drift again: Android DELETE is KEYCODE_FORWARD_DEL (112),
// while Android BACKSPACE is KEYCODE_DEL (67).
func TestHotkeyKeyCodeSnapshot(t *testing.T) {
	cases := []struct {
		key        string
		controller string
		primary    int
	}{
		{"Backspace", "Adb", 67},
		{"Delete", "Adb", 112},
		{"Del", "Adb", 112},
		{"Backspace", "Win32", 0x08},
		{"Delete", "Win32", 0x2E},
		{"Del", "Win32", 0x2E},
	}
	for _, tc := range cases {
		hotkey, err := pi.ParseHotkey(tc.key)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.key, err)
		}
		codes, err := hotkey.KeyCodes(tc.controller)
		if err != nil {
			t.Fatalf("%q on %s: %v", tc.key, tc.controller, err)
		}
		if codes.Primary != tc.primary {
			t.Errorf("%s %q = %#v, want primary %#x", tc.controller, tc.key, codes, tc.primary)
		}
	}
}

// TestOverrideMergesTopLevelFields pins MaaFramework's merge semantics: node
// objects merge, but a field present in both is replaced as a whole.
func TestOverrideMergesTopLevelFields(t *testing.T) {
	dst := pi.Pipeline{"task1": map[string]any{"enabled": false, "recognition": "DirectHit", "next": []any{"T1", "T2"}}}
	src := pi.Pipeline{"task1": map[string]any{"enabled": true, "action": "Click", "next": []any{"T3"}}}
	merged := pi.MergePipeline(dst, src)
	node, _ := merged["task1"].(map[string]any)
	if node["enabled"] != true || node["recognition"] != "DirectHit" || node["action"] != "Click" {
		t.Errorf("merged node = %#v", node)
	}
	next, _ := node["next"].([]any)
	if len(next) != 1 || next[0] != "T3" {
		t.Errorf("arrays must be replaced whole, got %#v", node["next"])
	}
}

// loadFixture writes a PI file and returns the loaded project with the single
// `t` task resolved.
func loadFixture(t *testing.T, body string) (*pi.Loaded, *pi.Task) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "interface.json")
	writeFile(t, path, body)
	project, err := pi.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	task := project.LookupTask("t")
	if task == nil {
		t.Fatalf("fixture has no task named t")
	}
	return project, task
}

// resolveOK resolves a request and fails the test on error.
func resolveOK(t *testing.T, project *pi.Loaded, request pi.Request) *pi.Resolution {
	t.Helper()
	resolution, err := project.Resolve(request)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return resolution
}

// findSelection returns the selection for an option name.
func findSelection(resolution *pi.Resolution, name string) (pi.Selection, bool) {
	for _, selection := range resolution.Selections {
		if selection.Name == name {
			return selection, true
		}
	}
	return pi.Selection{}, false
}

// assertCaseSource checks the chosen case and its reported value source.
func assertCaseSource(t *testing.T, resolution *pi.Resolution, name, wantCase string, wantSource pi.ValueSource) {
	t.Helper()
	selection, ok := findSelection(resolution, name)
	if !ok {
		t.Fatalf("option %s not resolved", name)
	}
	if len(selection.Cases) != 1 || selection.Cases[0] != wantCase || selection.Source != wantSource {
		t.Fatalf("option %s = %+v, want case %s from %s", name, selection, wantCase, wantSource)
	}
}
