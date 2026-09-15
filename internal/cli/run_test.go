package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maactl/internal/clientconfig"
	"maactl/internal/pi"
)

// TestParseOptionFlag covers the shell-friendly --option grammar.
func TestParseOptionFlag(t *testing.T) {
	cases := []struct {
		raw       string
		wantName  string
		wantValue any
	}{
		{"复现次数=x3", "复现次数", "x3"},
		{"刷完全部体力=No", "刷完全部体力", "No"},
		{"战斗划火柴=a,b", "战斗划火柴", "a,b"},
		{`战斗划火柴=["a","b"]`, "战斗划火柴", []any{"a", "b"}},
		{"自定义关卡.章节号=4", "自定义关卡", map[string]any{"章节号": "4"}},
		{`账号={"env":"TOKEN"}`, "账号", map[string]any{"env": "TOKEN"}},
	}
	for _, tc := range cases {
		name, value, err := parseOptionFlag(tc.raw)
		if err != nil {
			t.Errorf("%s: %v", tc.raw, err)
			continue
		}
		if name != tc.wantName {
			t.Errorf("%s: name = %q, want %q", tc.raw, name, tc.wantName)
		}
		if !deepEqual(value, tc.wantValue) {
			t.Errorf("%s: value = %#v, want %#v", tc.raw, value, tc.wantValue)
		}
	}
	for _, bad := range []string{"", "novalue", "=x", `x=[1,`} {
		if _, _, err := parseOptionFlag(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// deepEqual compares the small JSON-ish values used above.
func deepEqual(a, b any) bool {
	switch left := a.(type) {
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i := range left {
			if !deepEqual(left[i], right[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			if !deepEqual(value, right[key]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// TestParseCLIOptionsMergesFieldFlags checks repeated --option flags for the
// same input option merge into one field map.
func TestParseCLIOptionsMergesFieldFlags(t *testing.T) {
	values, err := parseCLIOptions([]string{"关卡.章节号=4", "关卡.难度=Hard"})
	if err != nil {
		t.Fatal(err)
	}
	fields, ok := values["关卡"].(map[string]any)
	if !ok || fields["章节号"] != "4" || fields["难度"] != "Hard" {
		t.Fatalf("values = %#v", values)
	}
}

// fixtureProject loads a PI with an i18n label and one controller/resource.
func fixtureProject(t *testing.T) *pi.Loaded {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "interface.json")
	body := `{
		"interface_version": 2,
		"name": "demo",
		"version": "v9",
		"languages": {"zh_cn": "zh.json"},
		"controller": [{"name": "Android", "label": "$安卓", "type": "Adb"}],
		"resource": [{"name": "base", "label": "$基础", "path": ["resource"]}],
		"task": [{"name": "t", "entry": "E"}]
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zh.json"), []byte(`{"安卓":"安卓设备","基础":"基础资源"}`), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := pi.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

// TestAgentEnvBuildsProtocolVariables checks the PI_* environment (protocol
// v2.5.0), including i18n-resolved controller/resource JSON.
func TestAgentEnvBuildsProtocolVariables(t *testing.T) {
	project := fixtureProject(t)
	prepared := &preparedRun{
		global:     &GlobalOptions{Lang: "zh_cn"},
		project:    project,
		config:     &clientconfig.Config{},
		controller: &project.Controller[0],
		resource:   &project.Resource[0],
		task:       project.LookupTask("t"),
	}
	env, err := prepared.agentEnv()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"PI_INTERFACE_VERSION=" + pi.ProtocolVersion,
		"PI_CLIENT_NAME=" + pi.ClientName,
		"PI_CLIENT_VERSION=" + clientVersion,
		"PI_CLIENT_LANGUAGE=zh_cn",
		"PI_VERSION=v9",
		`"label":"安卓设备"`,
		`"label":"基础资源"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("agent env is missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "$安卓") {
		t.Errorf("agent env must carry resolved labels:\n%s", joined)
	}
}

// TestExitCodesAreStable pins the mapping scripts depend on.
func TestExitCodesAreStable(t *testing.T) {
	var exit *ExitError
	if !errors.As(withExitCode(ExitTask, errors.New("boom")), &exit) || exit.Code != ExitTask {
		t.Fatal("withExitCode should attach the code")
	}
	// An error that already carries a code is not re-wrapped.
	wrapped := withExitCode(ExitUsage, exitErrorf(ExitResource, "resource"))
	if !errors.As(wrapped, &exit) || exit.Code != ExitResource {
		t.Fatalf("existing code was overwritten: %v", wrapped)
	}
	if withExitCode(ExitTask, nil) != nil {
		t.Fatal("nil errors must stay nil")
	}
}

// writeProject writes body to a fresh interface.json and loads it.
func writeProject(t *testing.T, body string) *pi.Loaded {
	t.Helper()
	path := filepath.Join(t.TempDir(), "interface.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := pi.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

// TestControllerFailureIsExitController pins exit code 4: an unsupported
// controller type is refused by createController before MaaFramework is touched,
// so this runs without a runtime.
func TestControllerFailureIsExitController(t *testing.T) {
	kind := unsupportedControllerType()
	if kind == "" {
		t.Fatalf("no unsupported controller type on %s", pi.PlatformName())
	}
	prepared := &preparedRun{
		global:     &GlobalOptions{},
		project:    fixtureProject(t),
		config:     &clientconfig.Config{},
		controller: &pi.Controller{Name: "other", Type: kind},
	}
	_, err := prepared.createController()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitController {
		t.Fatalf("err = %v, want exit code %d", err, ExitController)
	}
}

// TestPretaskFailureIsExitPretask pins exit code 5: a pretask whose executable
// cannot be started stops the run before the controller is created, so this
// runs offline.
func TestPretaskFailureIsExitPretask(t *testing.T) {
	project := writeProject(t, `{
		"interface_version": 2,
		"controller": [{"name": "Android", "type": "Adb"}],
		"resource": [{"name": "base", "path": ["resource"]}],
		"pretask": [{"exec": "maactl-definitely-missing-binary"}]
	}`)
	prepared := &preparedRun{
		global:     &GlobalOptions{},
		project:    project,
		config:     &clientconfig.Config{},
		controller: &project.Controller[0],
		resource:   &project.Resource[0],
		resolution: &pi.Resolution{},
	}
	err := prepared.runPretasks()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitPretask {
		t.Fatalf("err = %v, want exit code %d", err, ExitPretask)
	}
}
