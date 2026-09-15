package tests

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"maactl/internal/pi"
)

// TestImportMergeFollowsProtocol pins the protocol's import merge rules:
// task/preset/setting are appended, group and global_option dedupe keeping the
// first occurrence, and option merges by key with later definitions winning.
func TestImportMergeFollowsProtocol(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "interface.json"), `{
		"interface_version": 2,
		"name": "demo",
		"import": ["a.json", "b.json"],
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"group": [{"name": "daily", "label": "日常"}],
		"global_option": ["A", "B"],
		"pretask": [{"exec": "main-pre"}],
		"task": [{"name": "main-task", "entry": "Main"}],
		"option": {"A": {"type": "switch", "default_case": "Yes"}},
		"preset": [{"name": "main-preset", "task": []}],
		"setting": [{"name": "main-setting"}]
	}`)
	writeFile(t, filepath.Join(dir, "a.json"), `{
		"task": [{"name": "a-task", "entry": "A"}],
		"pretask": {"exec": "a-pre"},
		"group": [{"name": "daily", "label": "重复"}, {"name": "battle"}],
		"global_option": ["B", "C"],
		"option": {
			"A": {"type": "switch", "default_case": "No"},
			"D": {"type": "checkbox", "default_case": ["x"]}
		},
		"preset": [{"name": "a-preset", "task": []}],
		"setting": [{"name": "a-setting"}],
		"controller": [{"name": "Ignored", "type": "Win32"}],
		"resource": [{"name": "ignored", "path": ["nope"]}]
	}`)
	writeFile(t, filepath.Join(dir, "b.json"), `{
		"import": ["c.json"],
		"task": [{"name": "b-task", "entry": "B"}],
		"setting": [{"name": "b-setting"}]
	}`)
	writeFile(t, filepath.Join(dir, "c.json"), `{
		"task": [{"name": "c-task", "entry": "C"}],
		"pretask": [{"exec": "c-pre"}]
	}`)

	project, err := pi.Load(filepath.Join(dir, "interface.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Controller/resource are main-file only, even when an import declares them.
	if len(project.Controller) != 1 || project.Controller[0].Name != "Android" {
		t.Errorf("controllers = %+v", project.Controller)
	}
	if len(project.Resource) != 1 || project.Resource[0].Name != "base" {
		t.Errorf("resources = %+v", project.Resource)
	}

	// task/setting append in file order: main, a, b, c (b's import).
	gotTasks := []string{project.Task[0].Name, project.Task[1].Name, project.Task[2].Name, project.Task[3].Name}
	wantTasks := []string{"main-task", "a-task", "b-task", "c-task"}
	for i := range wantTasks {
		if gotTasks[i] != wantTasks[i] {
			t.Fatalf("task order = %v, want %v", gotTasks, wantTasks)
		}
	}
	gotSettings := []string{project.Setting[0].Name, project.Setting[1].Name, project.Setting[2].Name}
	wantSettings := []string{"main-setting", "a-setting", "b-setting"}
	for i := range wantSettings {
		if gotSettings[i] != wantSettings[i] {
			t.Fatalf("setting order = %v, want %v", gotSettings, wantSettings)
		}
	}
	if len(project.Preset) != 2 || project.Preset[0].Name != "main-preset" || project.Preset[1].Name != "a-preset" {
		t.Errorf("presets = %+v", project.Preset)
	}

	// group/global_option dedupe keeps first occurrences.
	if len(project.Group) != 2 || project.Group[0].Label != "日常" || project.Group[1].Name != "battle" {
		t.Errorf("groups = %+v", project.Group)
	}
	if len(project.GlobalOption) != 3 || project.GlobalOption[2] != "C" {
		t.Errorf("global_option = %v", project.GlobalOption)
	}

	// pretask: main entries first, then imports in order.
	pretasks := []string{project.Pretask[0].Identifier(), project.Pretask[1].Identifier(), project.Pretask[2].Identifier()}
	if pretasks[0] != "main-pre" || pretasks[1] != "a-pre" || pretasks[2] != "c-pre" {
		t.Errorf("pretask order = %v", pretasks)
	}

	// option merges by key; a later definition wins but keeps its position.
	if names := project.Option.Names(); len(names) != 2 || names[0] != "A" || names[1] != "D" {
		t.Errorf("option order = %v", names)
	}
	if option, _ := project.Option.Get("A"); option.DefaultCase.String() != "No" {
		t.Errorf("option A should be overridden by a.json: %+v", option.DefaultCase)
	}
	if option, _ := project.Option.Get("D"); option.DefaultCase == nil || option.DefaultCase.Slice()[0] != "x" {
		t.Errorf("option D default_case list not parsed: %+v", option.DefaultCase)
	}
}

// TestOptionMapJSONPreservesOrder guards the declarative order the CLI displays.
func TestOptionMapJSONPreservesOrder(t *testing.T) {
	var m pi.OptionMap
	if err := json.Unmarshal([]byte(`{"z":{"type":"select"},"a":{"type":"switch"},"m":{"type":"input"}}`), &m); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"z":{"type":"select"},"a":{"type":"switch"},"m":{"type":"input"}}` {
		t.Fatalf("round trip = %s", b)
	}
}

// TestDefaultCaseForms verifies the scalar (select/switch) and list (checkbox)
// shapes, including the `present` distinction used to tell "unset" from "empty".
func TestDefaultCaseForms(t *testing.T) {
	var doc struct {
		Scalar *pi.DefaultCase `json:"scalar"`
		List   *pi.DefaultCase `json:"list"`
		Absent *pi.DefaultCase `json:"absent"`
	}
	if err := json.Unmarshal([]byte(`{"scalar":"x3","list":["a","b"]}`), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Scalar == nil || doc.Scalar.String() != "x3" {
		t.Errorf("scalar = %+v", doc.Scalar)
	}
	if doc.List == nil || len(doc.List.Slice()) != 2 {
		t.Errorf("list = %+v", doc.List)
	}
	if doc.Absent != nil {
		t.Errorf("absent should be nil: %+v", doc.Absent)
	}
}

// TestTranslatorResolvesLabels checks language negotiation, `$` resolution, and
// the JSON-wide resolution used for PI_* agent environment variables.
func TestTranslatorResolvesLabels(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "interface.json"), `{
		"interface_version": 2,
		"name": "demo",
		"languages": {"zh_cn": "zh.json", "en_us": "en.json"},
		"controller": [{"name": "Android", "label": "$安卓", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}]
	}`)
	writeFile(t, filepath.Join(dir, "zh.json"), `{"安卓": "安卓设备", "缺省": "默认"}`)
	writeFile(t, filepath.Join(dir, "en.json"), `{"安卓": "Android device"}`)

	project, err := pi.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	zh := project.Translator("zh_cn")
	if got := zh.Resolve("$安卓"); got != "安卓设备" {
		t.Errorf("zh resolve = %q", got)
	}
	// A primary-subtag request ("zh") must find the declared "zh_cn".
	if got := project.Translator("zh-ZH").Resolve("$安卓"); got != "安卓设备" {
		t.Errorf("zh-ZH resolve = %q", got)
	}
	if got := project.Translator("en").Resolve("$安卓"); got != "Android device" {
		t.Errorf("en resolve = %q", got)
	}
	// Unknown references lose the `$` instead of leaking it into output.
	if got := project.Translator("zh_cn").Resolve("$不存在"); got != "不存在" {
		t.Errorf("missing key = %q", got)
	}
	if got := project.Translator("zh_cn").Resolve("plain"); got != "plain" {
		t.Errorf("plain string = %q", got)
	}

	resolved, ok := zh.ResolveLabels(project.Controller[0]).(map[string]any)
	if !ok || resolved["label"] != "安卓设备" {
		t.Fatalf("ResolveLabels = %#v", resolved)
	}
}

// TestResolveLabelsStripsDollarWithoutLanguages verifies that a project without
// a `languages` declaration still gets `$`-free labels from ResolveLabels, the
// same way Translator.Resolve does, so PI_CONTROLLER / PI_RESOURCE never carry
// an unresolved reference.
func TestResolveLabelsStripsDollarWithoutLanguages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "interface.json"), `{
		"interface_version": 2,
		"controller": [{"name": "Android", "label": "$安卓", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"], "label": "$基础"}]
	}`)
	project, err := pi.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	translator := project.Translator("")
	controller, ok := translator.ResolveLabels(project.Controller[0]).(map[string]any)
	if !ok || controller["label"] != "安卓" {
		t.Fatalf("ResolveLabels without languages = %#v", controller)
	}
	resource, ok := translator.ResolveLabels(project.Resource[0]).(map[string]any)
	if !ok || resource["label"] != "基础" {
		t.Fatalf("ResolveLabels without languages = %#v", resource)
	}
}

// TestLoadMaaFrameworkSampleInterface makes sure the protocol's reference file
// (JSONC comments, every top-level field) parses into the full model.
func TestLoadMaaFrameworkSampleInterface(t *testing.T) {
	project, err := pi.Load(sampleInterface(t))
	if err != nil {
		t.Fatalf("load sample: %v", err)
	}
	if project.Name != "MyDemo3" || project.InterfaceVersion != 2 {
		t.Fatalf("unexpected header: %+v", project.ProjectInterface)
	}
	if len(project.Controller) != 3 || project.Controller[2].Type != "MacOS" {
		t.Errorf("controllers = %+v", project.Controller)
	}
	if len(project.Resource) != 2 || project.Resource[0].Hash != "" {
		t.Errorf("resources = %+v", project.Resource)
	}
	// The comment-heavy file must still yield the declared option order.
	wantOptions := []string{"作战关卡", "自定义关卡", "复现次数", "刷完全部体力", "使用理智药", "刷完xxx", "战斗划火柴", "战斗自动闪避"}
	gotOptions := project.Option.Names()
	if len(gotOptions) != len(wantOptions) {
		t.Fatalf("options = %v", gotOptions)
	}
	for i := range wantOptions {
		if gotOptions[i] != wantOptions[i] {
			t.Fatalf("option order = %v, want %v", gotOptions, wantOptions)
		}
	}
	if got := project.GlobalOption; len(got) != 2 || got[0] != "战斗划火柴" {
		t.Errorf("global_option = %v", got)
	}
	if len(project.Preset) != 2 || len(project.Preset[0].Task) != 5 {
		t.Errorf("presets = %+v", project.Preset)
	}
	if project.Preset[0].Task[4].EnabledOrDefault() {
		t.Errorf("the disabled task in preset 0 should stay disabled")
	}
	option, ok := project.Option.Get("自定义关卡")
	if !ok || option.Kind() != pi.OptionTypeInput || len(option.Inputs) != 3 || option.Inputs[0].PipelineType != "string" {
		t.Errorf("input option = %+v", option)
	}
	checkbox, ok := project.Option.Get("战斗划火柴")
	if !ok || checkbox.Kind() != pi.OptionTypeCheckbox || len(checkbox.DefaultCase.Slice()) != 2 {
		t.Errorf("checkbox option = %+v", checkbox)
	}
	if translator := project.Translator("zh_cn"); translator.Lang() != "zh_cn" {
		t.Errorf("language negotiation = %q", translator.Lang())
	}
}
