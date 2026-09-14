package tests

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"maactl/internal/cli"
)

// testVersion is injected into the command tree instead of the real build
// version so version assertions are stable.
const testVersion = "test"

// TestMain pins the help language so assertions are deterministic on any
// system locale; individual tests override MAACTL_LANG when needed.
func TestMain(m *testing.M) {
	_ = os.Setenv("MAACTL_LANG", "en")
	os.Exit(m.Run())
}

// interfaceOutput runs the CLI in-process and returns its combined output.
func interfaceOutput(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := cli.NewRootCommand(testVersion)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func mustHelp(t *testing.T, args ...string) string {
	t.Helper()
	out, err := interfaceOutput(args...)
	if err != nil {
		t.Fatalf("help %v: %v\n%s", args, err, out)
	}
	return out
}

func TestRootHelpIsCompactOverview(t *testing.T) {
	out := mustHelp(t, "-h")
	for _, want := range []string{
		"Inspect ADB devices",
		"Inspect Win32 desktop windows",
		"Inspect and validate a ProjectInterface",
		"Load and inspect PI resources",
		"Run PI tasks or Pipeline nodes",
		"Global Flags:",
		"Examples:",
		`Use "maactl <command> --help" for more information about a command.`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("root help is missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"-a, --adb-address string",
		"--stop-after duration",
		"Execution Flags",
		"maactl run task <task-name> [flags]",
		"maactl resource inspect [flags]",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("root help should not expand %q\n%s", unwanted, out)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	out := mustHelp(t, "--version")
	if !strings.Contains(out, "maactl version "+testVersion) {
		t.Fatalf("unexpected version output: %q", out)
	}
}

// -v is the shorthand of --version.
func TestVersionShorthand(t *testing.T) {
	short := mustHelp(t, "-v")
	long := mustHelp(t, "--version")
	if short != long {
		t.Errorf("-v = %q, --version = %q", short, long)
	}
	if !strings.Contains(short, "maactl version "+testVersion) {
		t.Errorf("unexpected version output: %q", short)
	}
}

func TestHelpCommandMatchesDashH(t *testing.T) {
	for _, parts := range [][]string{
		{"run"},
		{"run", "task"},
		{"resource"},
		{"resource", "nodes"},
		{"interface"},
		{"adb", "devices"},
	} {
		flagArgs := append(append([]string{}, parts...), "-h")
		commandArgs := append([]string{"help"}, parts...)
		withFlag := mustHelp(t, flagArgs...)
		withCommand := mustHelp(t, commandArgs...)
		if withFlag != withCommand {
			t.Errorf("help mismatch for %v:\nflag:\n%s\ncommand:\n%s", parts, withFlag, withCommand)
		}
	}
}

func TestRunHelpListsSharedFlagsOnce(t *testing.T) {
	runHelp := mustHelp(t, "run", "-h")
	if !strings.Contains(runHelp, "Execution Flags (shared with subcommands):") {
		t.Fatalf("run help is missing the shared execution flags section:\n%s", runHelp)
	}
	taskHelp := mustHelp(t, "run", "task", "-h")
	if !strings.Contains(taskHelp, `Execution Flags (inherited from "maactl run"):`) {
		t.Fatalf("run task help is missing the inherited execution flags section:\n%s", taskHelp)
	}
	for _, help := range []string{runHelp, taskHelp} {
		if n := strings.Count(help, "-a, --adb-address string"); n != 1 {
			t.Errorf("shared flag listed %d times:\n%s", n, help)
		}
	}
}

func TestPlannedFeaturesAreGrouped(t *testing.T) {
	runHelp := mustHelp(t, "run", "-h")
	idx := strings.Index(runHelp, "Planned Flags:")
	if idx < 0 {
		t.Fatalf("run help is missing the planned flags section:\n%s", runHelp)
	}
	planned := runHelp[idx:]
	for _, flag := range []string{"--dry-run", "--explain", "-p, --option", "--option-file", "--overlay"} {
		if !strings.Contains(planned, flag) {
			t.Errorf("planned flag %s missing from the planned section", flag)
		}
	}
	if strings.Contains(runHelp[:idx], "--dry-run") {
		t.Errorf("planned flag --dry-run should not appear outside the planned section")
	}
	if !strings.Contains(runHelp, "preset <name>") || !strings.Contains(runHelp, "Run the enabled tasks in a PI preset (planned)") {
		t.Errorf("planned command preset is not marked in run help")
	}

	interfaceHelp := mustHelp(t, "interface", "-h")
	idx = strings.Index(interfaceHelp, "Planned Flags:")
	if idx < 0 || !strings.Contains(interfaceHelp[idx:], "--options") || !strings.Contains(interfaceHelp[idx:], "--presets") {
		t.Fatalf("planned interface actions are not grouped:\n%s", interfaceHelp)
	}
	if !strings.Contains(interfaceHelp, "Actions:") {
		t.Errorf("interface help is missing the actions section:\n%s", interfaceHelp)
	}
}

func TestRunHelpDocumentsWin32Flags(t *testing.T) {
	runHelp := mustHelp(t, "run", "-h")
	for _, flag := range []string{
		"--win32-handle", "--win32-class", "--win32-window",
		"--win32-screencap", "--win32-mouse", "--win32-keyboard",
	} {
		if n := strings.Count(runHelp, flag); n != 1 {
			t.Errorf("win32 flag %s listed %d times in run help\n%s", flag, n, runHelp)
		}
	}
	taskHelp := mustHelp(t, "run", "task", "-h")
	for _, flag := range []string{"--win32-handle", "--win32-screencap"} {
		if !strings.Contains(taskHelp, flag) {
			t.Errorf("win32 flag %s missing from inherited run task help\n%s", flag, taskHelp)
		}
	}
}

func TestResourceHelpAttributesResourceFlag(t *testing.T) {
	parent := mustHelp(t, "resource", "-h")
	if !strings.Contains(parent, "-r, --resource string") {
		t.Fatalf("resource help is missing the resource flag:\n%s", parent)
	}
	global := parent[strings.Index(parent, "Global Flags:"):]
	if strings.Contains(global, "-r, --resource") {
		t.Errorf("resource flag is mislabelled as global:\n%s", parent)
	}
	child := mustHelp(t, "resource", "nodes", "-h")
	if !strings.Contains(child, `Inherited Flags (from "maactl resource"):`) {
		t.Fatalf("resource nodes does not attribute the resource flag:\n%s", child)
	}
}

func TestDevicesHelpUsesGlobalJSON(t *testing.T) {
	out := mustHelp(t, "adb", "devices", "-h")
	if n := strings.Count(out, "-j, --json"); n != 1 {
		t.Fatalf("json flag listed %d times:\n%s", n, out)
	}
	global := out[strings.Index(out, "Global Flags:"):]
	if !strings.Contains(global, "-j, --json") {
		t.Errorf("adb devices should use the global json flag:\n%s", out)
	}
}

func TestHelpLanguageOverride(t *testing.T) {
	t.Setenv("MAACTL_LANG", "zh_CN")
	out := mustHelp(t, "-h")
	for _, want := range []string{
		"用法：",
		"命令：",
		"全局选项：",
		"示例：",
		"查看 ADB 设备",
		"运行 PI task 或 Pipeline 节点",
		`使用 "maactl <command> --help" 查看某个命令的更多信息。`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Chinese help is missing %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Usage:", "Commands:", "Global Flags:", "Examples:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("Chinese help leaked English section title %q\n%s", unwanted, out)
		}
	}
}

func TestChineseHelpSections(t *testing.T) {
	t.Setenv("MAACTL_LANG", "zh-Hans")
	runHelp := mustHelp(t, "run", "-h")
	for _, want := range []string{"执行选项（与子命令共用）：", "计划中的选项：", "（计划中）", "事件输出", `（默认 "focus"）`} {
		if !strings.Contains(runHelp, want) {
			t.Errorf("Chinese run help is missing %q\n%s", want, runHelp)
		}
	}
	iface := mustHelp(t, "interface", "-h")
	if !strings.Contains(iface, "操作：") || !strings.Contains(iface, "计划中的选项：") || !strings.Contains(iface, "列出任务") {
		t.Errorf("Chinese interface help is incomplete\n%s", iface)
	}
	nodes := mustHelp(t, "resource", "nodes", "-h")
	if !strings.Contains(nodes, `继承选项（来自 "maactl resource"）：`) {
		t.Errorf("Chinese inherited flag section missing\n%s", nodes)
	}
	task := mustHelp(t, "run", "task", "-h")
	if !strings.Contains(task, `执行选项（继承自 "maactl run"）：`) {
		t.Errorf("Chinese inherited execution section missing\n%s", task)
	}
	completion := mustHelp(t, "completion", "-h")
	if !strings.Contains(completion, "为指定的 shell 生成自动补全脚本") || !strings.Contains(completion, "为 bash 生成自动补全脚本") {
		t.Errorf("Chinese completion help is incomplete\n%s", completion)
	}
}

func TestEnglishHelpStaysEnglish(t *testing.T) {
	t.Setenv("MAACTL_LANG", "en_US.UTF-8")
	out := mustHelp(t, "run", "-h")
	if !strings.Contains(out, "Execution Flags (shared with subcommands):") {
		t.Errorf("English run help missing section\n%s", out)
	}
	if strings.Contains(out, "计划中的选项") || strings.Contains(out, "用法：") {
		t.Errorf("English help leaked Chinese section titles\n%s", out)
	}
}

// Shared run flags must parse both before and after the subcommand name.
func TestRunFlagsParseAroundSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"run", "task", "demo", "-r", "base"},
		{"run", "-r", "base", "task", "demo"},
		{"run", "-t", "demo", "-r", "base"},
	} {
		root := cli.NewRootCommand(testVersion)
		target, rest, err := root.Find(args)
		if err != nil {
			t.Fatalf("%v: find: %v", args, err)
		}
		if err := target.ParseFlags(rest); err != nil {
			t.Fatalf("%v: parse: %v", args, err)
		}
		if got, err := target.Flags().GetString("resource"); err != nil || got != "base" {
			t.Errorf("%v: resource = %q, err = %v", args, got, err)
		}
		if target.Name() == "task" {
			if target.LocalFlags().Lookup("task") != nil || target.LocalFlags().Lookup("node") != nil {
				t.Errorf("%v: run shortcut flags leaked into %s", args, target.Name())
			}
		}
	}
}
