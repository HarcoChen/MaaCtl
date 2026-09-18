package tests

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// runCLIRedirect runs the CLI while capturing the writes `run` makes directly to
// os.Stdout and os.Stderr. Its summaries and --explain output bypass the cobra
// command's own writers, so the usual runCLI buffer never sees them.
func runCLIRedirect(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalOut, originalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	stdoutCh := make(chan string, 1)
	stderrCh := make(chan string, 1)
	go func() { data, _ := io.ReadAll(stdoutReader); stdoutCh <- string(data) }()
	go func() { data, _ := io.ReadAll(stderrReader); stderrCh <- string(data) }()

	_, err = runCLI(args...)

	stdoutWriter.Close()
	stderrWriter.Close()
	os.Stdout, os.Stderr = originalOut, originalErr
	return <-stdoutCh, <-stderrCh, err
}

// TestRunDryRunJSONSummary pins the `run --dry-run --json` contract documented
// in docs/cli.md: the machine-readable summary keeps the field names scripts
// read. The path never creates a controller, so it runs offline.
func TestRunDryRunJSONSummary(t *testing.T) {
	path := piFixture(t)
	stdout, _, err := runCLIRedirect(t, "run", "task", "打开游戏", "-if", path, "-c", "Android", "-dr", "-j")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, stdout)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	for _, field := range []string{"interface", "resource", "resource_paths", "controller", "entry", "status", "dry_run"} {
		if _, ok := summary[field]; !ok {
			t.Errorf("dry-run summary is missing field %q: %v", field, summary)
		}
	}
	if summary["dry_run"] != true {
		t.Errorf("dry_run = %#v, want true", summary["dry_run"])
	}
	if summary["controller"] != "Android" || summary["entry"] != "启动游戏" {
		t.Errorf("summary target = %v", summary)
	}
	if summary["status"] != "DryRun" {
		t.Errorf("status = %#v, want DryRun", summary["status"])
	}
	paths, ok := summary["resource_paths"].([]any)
	if !ok || len(paths) == 0 {
		t.Errorf("resource_paths = %#v", summary["resource_paths"])
	}
}

// TestRunExplainPrintsEveryLayer checks that `--explain` reports the selections,
// each override layer, and the effective override on stderr. The shapes are
// asserted rather than the exact wording so a copy change does not break it.
func TestRunExplainPrintsEveryLayer(t *testing.T) {
	path := piFixture(t)
	_, stderr, err := runCLIRedirect(t, "run", "task", "打开游戏", "-if", path, "-c", "Android", "-x", "-dr")
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, stderr)
	}
	for _, want := range []string{"entry: 启动游戏", "option 模式 =", "override [", "effective override:"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("explain output is missing %q:\n%s", want, stderr)
		}
	}
}
