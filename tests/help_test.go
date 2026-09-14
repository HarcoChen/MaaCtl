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

// runCLI runs the CLI in-process and returns its combined output.
func runCLI(args ...string) (string, error) {
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
	out, err := runCLI(args...)
	if err != nil {
		t.Fatalf("help %v: %v\n%s", args, err, out)
	}
	return out
}

func TestRootHelpIsCompactOverview(t *testing.T) {
	out := mustHelp(t, "-h")
	for _, want := range []string{
		"Inspect and validate a ProjectInterface",
		"Inspect devices and windows",
		"Load and inspect MaaFramework resources",
		"Run PI tasks, presets, or Pipeline nodes",
		"Inspect the client configuration",
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

// -v is the shorthand of --version at the root.
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

// The version command mirrors the flag.
func TestVersionCommand(t *testing.T) {
	out := mustHelp(t, "version")
	if !strings.Contains(out, "maactl version "+testVersion) {
		t.Fatalf("unexpected version output: %q", out)
	}
}

func TestHelpCommandMatchesDashH(t *testing.T) {
	for _, parts := range [][]string{
		{"run"},
		{"run", "task"},
		{"resource"},
		{"resource", "nodes"},
		{"pi"},
		{"pi", "tasks"},
		{"device", "adb"},
		{"config", "show"},
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

// Every former "planned" feature is implemented now, so no help page may still
// advertise one.
func TestNoPlannedFeaturesRemain(t *testing.T) {
	for _, args := range [][]string{
		{"-h"}, {"run", "-h"}, {"pi", "-h"}, {"resource", "-h"}, {"config", "-h"}, {"device", "-h"},
	} {
		out := mustHelp(t, args...)
		for _, unwanted := range []string{"(planned)", "Planned Flags:", "（计划中）", "计划中的选项："} {
			if strings.Contains(out, unwanted) {
				t.Errorf("%v help still advertises %q\n%s", args, unwanted, out)
			}
		}
	}
}

func TestRunHelpDocumentsWin32Flags(t *testing.T) {
	runHelp := mustHelp(t, "run", "-h")
	for _, flag := range []string{
		"--win32-handle", "--win32-class", "--win32-window",
		"--win32-screencap", "--win32-mouse", "--win32-keyboard",
		"--gamepad-type",
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

// The short flags must not be reused for a second meaning anywhere.
func TestShortFlagsAreUnambiguous(t *testing.T) {
	// The concrete guarantee: -v is --version, -p is --preset, -o is --override,
	// and -t/-n are the run shortcuts.
	out := mustHelp(t, "run", "-h")
	for _, want := range []string{"-p, --preset", "-o, --override", "-n, --node", "-t, --task", "-c, --controller", "-r, --resource"} {
		if !strings.Contains(out, want) {
			t.Errorf("run help is missing %q", want)
		}
	}
	for _, unwanted := range []string{"-p, --option", "-O, --override-file", "-v, --validate"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("run help should not contain %q\n%s", unwanted, out)
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

func TestHelpLanguageOverride(t *testing.T) {
	t.Setenv("MAACTL_LANG", "zh_CN")
	out := mustHelp(t, "-h")
	for _, want := range []string{
		"用法：",
		"命令：",
		"全局选项：",
		"示例：",
		"查看设备与窗口",
		"运行 PI task、preset 或 Pipeline 节点",
		"检查和验证 ProjectInterface",
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
	for _, want := range []string{"执行选项（与子命令共用）：", "事件输出", `（默认 "focus"）`, "配置项取值"} {
		if !strings.Contains(runHelp, want) {
			t.Errorf("Chinese run help is missing %q\n%s", want, runHelp)
		}
	}
	piHelp := mustHelp(t, "pi", "-h")
	if !strings.Contains(piHelp, "检查和验证 ProjectInterface") || !strings.Contains(piHelp, "列出任务") {
		t.Errorf("Chinese pi help is incomplete\n%s", piHelp)
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
	if strings.Contains(out, "配置项") || strings.Contains(out, "用法：") {
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
	}
}
