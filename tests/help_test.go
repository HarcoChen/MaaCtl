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

// runCLI runs the CLI in-process and returns its combined output. Arguments go
// through the same alias normalization as the real binary.
func runCLI(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := cli.NewRootCommand(testVersion)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(cli.NormalizeArgs(args))
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
		"pi, if, interface",
		"resource, res",
		"device, dev",
		"run, r",
		"config, cfg",
		"Global:",
		"Example:",
		`Use "maactl <command> -h" for command details.`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("root help is missing %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		"-a, --adb-address string",
		"Target:",
		"maactl run task <task-name>",
		"maactl resource inspect",
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

// The version command mirrors the flag and has a short alias.
func TestVersionCommand(t *testing.T) {
	out := mustHelp(t, "version")
	if !strings.Contains(out, "maactl version "+testVersion) {
		t.Fatalf("unexpected version output: %q", out)
	}
	if alias := mustHelp(t, "ver"); alias != out {
		t.Errorf("version alias output differs:\n%s\n%s", alias, out)
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

// Run flags are grouped by purpose, and a flag is never listed twice on one page.
func TestRunHelpGroupsFlagsOnce(t *testing.T) {
	runHelp := mustHelp(t, "run", "-h")
	for _, section := range []string{"Shortcuts:", "Target:", "Options:", "Resources:", "Output:", "Control:", "Global:"} {
		if !strings.Contains(runHelp, section) {
			t.Errorf("run help is missing the %s section\n%s", section, runHelp)
		}
	}
	taskHelp := mustHelp(t, "run", "task", "-h")
	for _, flag := range []string{"-a, --adb-address", "-opt, --option", "-pa, --path", "-e, --events", "-dr, --dry-run", "-f, -if, --interface"} {
		if n := strings.Count(taskHelp, flag); n != 1 {
			t.Errorf("flag %s listed %d times in run task help\n%s", flag, n, taskHelp)
		}
	}
}

// Flags keep declaration order inside a group, so related flags stay adjacent
// instead of being sorted alphabetically.
func TestRunHelpKeepsDeclarationOrder(t *testing.T) {
	out := mustHelp(t, "run", "-h")
	index := func(flag string) int { return strings.Index(out, flag) }
	if index("-r, --resource") > index("-c, --controller") || index("-c, --controller") > index("-a, --adb-address") {
		t.Errorf("target flags are not in declaration order:\n%s", out)
	}
	if index("-wh, --win32-handle") > index("-wc, --win32-class") {
		t.Errorf("win32 flags are not in declaration order:\n%s", out)
	}
	if index("-opt, --option") > index("-p, --preset") {
		t.Errorf("option flags are not in declaration order:\n%s", out)
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
			t.Errorf("win32 flag %s missing from run task help\n%s", flag, taskHelp)
		}
	}
}

// Short flavors are consistent: single letters where free, 2-3 letters otherwise,
// and no letter means two different things.
func TestShortFlagsAreConsistent(t *testing.T) {
	out := mustHelp(t, "run", "-h")
	for _, want := range []string{
		"-r, --resource", "-c, --controller", "-a, --adb-address",
		"-t, --task", "-n, --node", "-p, --preset", "-o, --override",
		"-e, --events", "-x, --explain", "-opt, --option", "-of, --option-file",
		"-ovf, --override-file", "-pa, --path", "-ol, --overlay", "-to, --timeout",
		"-sa, --stop-after", "-dr, --dry-run", "-al, --agent-log", "-k, -coe, --continue-on-error",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("run help is missing %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"-O, --override-file", "-v, --validate"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("run help should not contain %q\n%s", unwanted, out)
		}
	}
	// Global flags accept both the single letter and the mnemonic alias.
	global := mustHelp(t, "-h")
	for _, want := range []string{"-f, -if, --interface", "-l, -lib, --lib-dir", "-j, --json", "-cfg, --config", "-lg, --lang"} {
		if !strings.Contains(global, want) {
			t.Errorf("global help is missing %q\n%s", want, global)
		}
	}
}

// Every command in the tree exposes a short alias, and aliases are unique among
// siblings.
func TestEveryCommandHasAShortAlias(t *testing.T) {
	expectations := map[string][]string{
		"maactl":          {"pi, if, interface", "resource, res", "device, dev", "run, r", "config, cfg", "version, ver"},
		"maactl pi":       {"info, i", "validate, v", "controllers, c", "tasks, t", "groups, g", "options, o", "presets, p", "settings, s"},
		"maactl resource": {"list, l", "inspect, i", "nodes, n", "hash, h"},
		"maactl device":   {"adb, a", "win32, w"},
		"maactl run":      {"task, t <task-name>", "preset, p <preset-name>", "node, n <node-name>"},
		"maactl config":   {"path, p", "show, s"},
	}
	for command, wants := range expectations {
		args := strings.Fields(strings.TrimPrefix(command, "maactl"))
		args = append(args, "-h")
		out := mustHelp(t, args...)
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("%s help is missing %q\n%s", command, want, out)
			}
		}
	}
}

func TestResourceHelpAttributesResourceFlag(t *testing.T) {
	parent := mustHelp(t, "resource", "-h")
	if !strings.Contains(parent, "-r, --resource") {
		t.Fatalf("the resource group should list its own flags:\n%s", parent)
	}
	group := parent[strings.Index(parent, "Global:"):]
	if strings.Contains(group, "-r, --resource") {
		t.Errorf("resource flag is mislabelled as global:\n%s", parent)
	}
	child := mustHelp(t, "resource", "nodes", "-h")
	if !strings.Contains(child, "-r, --resource") {
		t.Fatalf("resource nodes is missing the inherited resource flag:\n%s", child)
	}
	global := child[strings.Index(child, "Global:"):]
	if strings.Contains(global, "-r, --resource") {
		t.Errorf("resource flag is mislabelled as global:\n%s", child)
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
		"列出设备与窗口",
		"运行 task、preset 或节点",
		"检查 ProjectInterface",
		`使用 "maactl <command> -h" 查看命令细节。`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Chinese help is missing %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Usage:", "Commands:", "Global:", "Example:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("Chinese help leaked English section title %q\n%s", unwanted, out)
		}
	}
}

func TestChineseHelpSections(t *testing.T) {
	t.Setenv("MAACTL_LANG", "zh-Hans")
	runHelp := mustHelp(t, "run", "-h")
	for _, want := range []string{"快捷方式：", "目标选择：", "配置项与覆盖：", "资源：", "输出：", "运行控制：", "全局选项：", `（默认 "focus"）`} {
		if !strings.Contains(runHelp, want) {
			t.Errorf("Chinese run help is missing %q\n%s", want, runHelp)
		}
	}
	piHelp := mustHelp(t, "pi", "-h")
	if !strings.Contains(piHelp, "检查 ProjectInterface") || !strings.Contains(piHelp, "列出任务") {
		t.Errorf("Chinese pi help is incomplete\n%s", piHelp)
	}
	completion := mustHelp(t, "completion", "-h")
	if !strings.Contains(completion, "为指定的 shell 生成自动补全脚本") || !strings.Contains(completion, "为 bash 生成自动补全脚本") {
		t.Errorf("Chinese completion help is incomplete\n%s", completion)
	}
}

func TestEnglishHelpStaysEnglish(t *testing.T) {
	t.Setenv("MAACTL_LANG", "en_US.UTF-8")
	out := mustHelp(t, "run", "-h")
	if !strings.Contains(out, "Target:") || !strings.Contains(out, "Global:") {
		t.Errorf("English run help missing sections\n%s", out)
	}
	if strings.Contains(out, "配置项") || strings.Contains(out, "用法：") {
		t.Errorf("English help leaked Chinese section titles\n%s", out)
	}
}

// Shared run flags must parse both before and after the subcommand name, and
// multi-letter aliases must work in both positions.
func TestRunFlagsParseAroundSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"run", "task", "demo", "-r", "base"},
		{"run", "-r", "base", "task", "demo"},
		{"run", "-t", "demo", "-r", "base"},
		{"run", "-t", "demo", "-if", "D:\\proj"},
		{"run", "task", "demo", "-opt", "mode=fast"},
	} {
		root := cli.NewRootCommand(testVersion)
		target, rest, err := root.Find(cli.NormalizeArgs(args))
		if err != nil {
			t.Fatalf("%v: find: %v", args, err)
		}
		if err := target.ParseFlags(rest); err != nil {
			t.Fatalf("%v: parse: %v", args, err)
		}
		if strings.Contains(strings.Join(args, " "), "-r base") {
			if got, err := target.Flags().GetString("resource"); err != nil || got != "base" {
				t.Errorf("%v: resource = %q, err = %v", args, got, err)
			}
		}
		if strings.Contains(strings.Join(args, " "), "-if") {
			if got, err := target.Flags().GetString("interface"); err != nil || got != "D:\\proj" {
				t.Errorf("%v: interface = %q, err = %v", args, got, err)
			}
		}
		if strings.Contains(strings.Join(args, " "), "-opt") {
			if got, err := target.Flags().GetStringArray("option"); err != nil || len(got) != 1 || got[0] != "mode=fast" {
				t.Errorf("%v: option = %v, err = %v", args, got, err)
			}
		}
	}
}
