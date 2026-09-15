package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/pi"
)

// TestValidateSampleInterface checks that the protocol's reference file passes
// validation when filesystem checks are skipped.
func TestValidateSampleInterface(t *testing.T) {
	project, err := pi.Load(filepath.Join("..", "maafw", "sample", "interface.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := project.Validate(pi.ValidateOptions{SkipFiles: true})
	if report.Errors() != 0 {
		t.Fatalf("sample interface has errors: %+v", report.Issues)
	}
}

// TestValidateFindsProblems exercises the checks a real project trips over.
func TestValidateFindsProblems(t *testing.T) {
	project, _ := loadFixture(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}, {"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"], "option": ["不存在"]}],
		"task": [{"name": "t", "entry": "", "resource": ["missing"], "option": ["未定义"]}],
		"global_option": ["也没有"],
		"setting": [{"name": "s", "option": ["无"]}],
		"preset": [{"name": "p", "task": [{"name": "不存在"}]}],
		"option": {
			"A": {"type": "switch", "cases": [{"name": "Yes"}, {"name": "No"}, {"name": "Maybe"}]},
			"B": {"type": "select", "cases": [{"name": "x"}], "default_case": "y"},
			"C": {"type": "checkbox", "min_count": 3, "max_count": 1, "cases": [{"name": "x"}]},
			"D": {"type": "input", "inputs": [{"name": "pw", "password": true, "default": "oops"}]},
			"E": {"type": "select", "cases": [{"name": "x"}], "default_case": ["x"]}
		}
	}`)
	report := project.Validate(pi.ValidateOptions{SkipFiles: true})
	if report.Errors() == 0 {
		t.Fatal("expected validation errors")
	}
	joined := ""
	for _, issue := range report.Issues {
		joined += issue.Path + ": " + issue.Message + "\n"
	}
	for _, want := range []string{
		"duplicate name",
		"entry node is required",
		"references resource \"missing\"",
		"references option \"未定义\"",
		"global_option[0]",
		"setting[0].option[0]",
		"preset[0].task[0].name",
		"exactly two cases",
		"default case \"y\"",
		"min_count",
		"password field must not declare a default",
		"expects a single case name",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("validation is missing %q:\n%s", want, joined)
		}
	}
}

// TestValidateChecksFiles verifies that missing resource paths and language
// files are reported as warnings, and promoted by --strict.
func TestValidateChecksFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "interface.json")
	writeFile(t, path, `{
		"interface_version": 2,
		"languages": {"zh_cn": "missing.json"},
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E"}]
	}`)
	project, err := pi.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	report := project.Validate(pi.ValidateOptions{})
	if report.Warnings() == 0 {
		t.Fatalf("expected warnings: %+v", report.Issues)
	}
	if !report.OK() {
		t.Errorf("non-strict validation should tolerate warnings: %+v", report.Issues)
	}
	strict := project.Validate(pi.ValidateOptions{Strict: true})
	if strict.OK() {
		t.Errorf("strict validation should fail on warnings: %+v", strict.Issues)
	}
}

// TestValidateAcceptsWindowsStylePaths verifies the validator normalizes
// backslashes like the loader does, so a project written with Windows-style
// paths is not reported as missing when it actually loads.
func TestValidateAcceptsWindowsStylePaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "interface.json"), `{
		"interface_version": 2,
		"languages": {"zh_cn": "i18n\\zh.json"},
		"controller": [{"name": "Android", "type": "Adb", "attach_resource_path": ["resource\\extra"]}],
		"resource": [{"name": "base", "path": ["resource\\base"]}],
		"task": [{"name": "t", "entry": "E"}]
	}`)
	writeFile(t, filepath.Join(dir, "resource", "base", "keep.txt"), "")
	writeFile(t, filepath.Join(dir, "resource", "extra", "keep.txt"), "")
	writeFile(t, filepath.Join(dir, "i18n", "zh.json"), `{}`)

	project, err := pi.Load(filepath.Join(dir, "interface.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := project.Validate(pi.ValidateOptions{Strict: true})
	if !report.OK() {
		t.Fatalf("windows-style paths should validate: %+v", report.Issues)
	}
}
