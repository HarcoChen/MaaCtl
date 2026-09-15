package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadPI writes body to a fresh interface.json and loads it.
func loadPI(t *testing.T, body string) *Loaded {
	t.Helper()
	path := filepath.Join(t.TempDir(), "interface.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return project
}

// selection returns the resolved selection for name.
func selection(t *testing.T, resolution *Resolution, name string) Selection {
	t.Helper()
	for _, sel := range resolution.Selections {
		if sel.Name == name {
			return sel
		}
	}
	t.Fatalf("option %q was not resolved: %+v", name, resolution.Selections)
	return Selection{}
}

// TestResolveOptionStandalone covers the pretask path: ResolveOption resolves an
// option the task's layers never reference, returns nil when it does not apply,
// and honours an explicit value.
func TestResolveOptionStandalone(t *testing.T) {
	project := loadPI(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [
			{"name": "base", "path": ["resource"]},
			{"name": "official", "path": ["resource2"]}
		],
		"option": {
			"模式": {"type": "select", "default_case": "普通", "cases": [{"name": "普通"}, {"name": "急速"}]},
			"仅Official": {"type": "switch", "resource": ["official"], "default_case": "Yes", "cases": [{"name": "Yes"}, {"name": "No"}]},
			"使用药": {"type": "switch", "default_case": "Yes", "cases": [{"name": "Yes"}, {"name": "No"}]}
		}
	}`)
	req := Request{ControllerName: "Android", ResourceName: "base"}

	value, err := project.ResolveOption("模式", req)
	if err != nil || value != "普通" {
		t.Errorf("default select = %#v, %v; want 普通", value, err)
	}

	withValue := req
	withValue.CLI = map[string]any{"模式": "急速"}
	if value, err = project.ResolveOption("模式", withValue); err != nil || value != "急速" {
		t.Errorf("cli select = %#v, %v; want 急速", value, err)
	}

	// A nested-only option is not referenced by any layer but must still resolve
	// on its own, which is why ResolveOption exists.
	if value, err = project.ResolveOption("使用药", req); err != nil || value != "Yes" {
		t.Errorf("nested option = %#v, %v; want Yes", value, err)
	}

	// An option restricted to another resource does not apply.
	if value, err = project.ResolveOption("仅Official", req); err != nil || value != nil {
		t.Errorf("inapplicable option = %#v, %v; want nil, nil", value, err)
	}
}

// TestResolveReportsSourceDefault pins the documented "default" source value: an
// option resolved from its own default_case has no caller layer behind it.
func TestResolveReportsSourceDefault(t *testing.T) {
	project := loadPI(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"global_option": ["模式"],
		"option": {"模式": {"type": "select", "default_case": "普通", "cases": [{"name": "普通"}, {"name": "急速"}]}}
	}`)
	resolution, err := project.Resolve(Request{ControllerName: "Android", ResourceName: "base"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	sel := selection(t, resolution, "模式")
	if sel.Source != SourceDefault {
		t.Errorf("source = %q, want %q (the documented JSON value)", sel.Source, SourceDefault)
	}
	if SourceDefault != "default" {
		t.Errorf("SourceDefault = %q, want default", SourceDefault)
	}
}

// TestResolveRejectsCyclicOption checks the optionPath cycle guard on
// case.option self references.
func TestResolveRejectsCyclicOption(t *testing.T) {
	project := loadPI(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"global_option": ["A"],
		"option": {"A": {"type": "select", "default_case": "a", "cases": [{"name": "a", "option": ["A"]}]}}
	}`)
	_, err := project.Resolve(Request{ControllerName: "Android", ResourceName: "base"})
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("err = %v, want a cyclic reference error", err)
	}
}

// TestResolveRejectsUnsupportedOptionType covers the resolver's default branch.
func TestResolveRejectsUnsupportedOptionType(t *testing.T) {
	project := loadPI(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"global_option": ["X"],
		"option": {"X": {"type": "nope"}}
	}`)
	_, err := project.Resolve(Request{ControllerName: "Android", ResourceName: "base"})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("err = %v, want an unsupported type error", err)
	}
}

// TestPickCaseRejectsUnknownCase covers the "is not a case" branch for select.
func TestPickCaseRejectsUnknownCase(t *testing.T) {
	r := &resolver{}
	option := &Option{Name: "模式", Type: OptionTypeSelect, Cases: []OptionCase{{Name: "a"}, {Name: "b"}}}
	_, err := r.pickCase(option, "zzz", true)
	if err == nil || !strings.Contains(err.Error(), "is not a case") {
		t.Fatalf("err = %v, want an is-not-a-case error", err)
	}
}

// TestOptionCasesRejectsUnknownCase covers the same branch for checkbox.
func TestOptionCasesRejectsUnknownCase(t *testing.T) {
	option := &Option{Name: "功能", Type: OptionTypeCheckbox, Cases: []OptionCase{{Name: "a"}, {Name: "b"}}}
	_, err := optionCases(option, []any{"a", "zzz"}, true)
	if err == nil || !strings.Contains(err.Error(), "is not a case") {
		t.Fatalf("err = %v, want an is-not-a-case error", err)
	}
}

// TestConvertInputErrors covers the two failure modes of pipeline_type coercion.
func TestConvertInputErrors(t *testing.T) {
	if _, err := convertInput("maybe", "bool"); err == nil || !strings.Contains(err.Error(), "expected a boolean") {
		t.Errorf("bool err = %v, want expected a boolean", err)
	}
	if _, err := convertInput("1.5", "float"); err == nil || !strings.Contains(err.Error(), "unsupported pipeline_type") {
		t.Errorf("float err = %v, want unsupported pipeline_type", err)
	}
}

// TestVerifyFieldRejectsInvalidPattern covers the regexp compile failure.
func TestVerifyFieldRejectsInvalidPattern(t *testing.T) {
	err := verifyField(&Option{Name: "关卡"}, OptionInput{Name: "章节号", Verify: "("}, "4")
	if err == nil || !strings.Contains(err.Error(), "invalid verify pattern") {
		t.Fatalf("err = %v, want an invalid verify pattern error", err)
	}
}

// TestResolveFieldValueReportsMissingEnv covers the env reference whose variable
// is not set.
func TestResolveFieldValueReportsMissingEnv(t *testing.T) {
	r := &resolver{lookupEnv: func(string) (string, bool) { return "", false }}
	_, err := r.resolveFieldValue(
		&Option{Name: "账号"},
		OptionInput{Name: "token"},
		map[string]any{"env": "MAACTL_TEST_MISSING_TOKEN"},
		SourceOptionFile,
	)
	if err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("err = %v, want an is-not-set error", err)
	}
}

// TestHotkeyValuesRequiresDefault covers a hotkey field with neither a value nor
// a default.
func TestHotkeyValuesRequiresDefault(t *testing.T) {
	r := &resolver{}
	_, err := r.hotkeyValues(&Option{Name: "改键", Hotkeys: []OptionHotkey{{Name: "Skill"}}}, nil, false)
	if err == nil || !strings.Contains(err.Error(), "has no default") {
		t.Fatalf("err = %v, want a has-no-default error", err)
	}
}
