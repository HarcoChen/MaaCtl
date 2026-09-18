package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// stubRuntime replaces the runtime lookup for one test. Resolving and loading a
// real MaaFramework release is what makes the version line meaningful, but a
// unit test must not depend on one being unpacked.
func stubRuntime(t *testing.T, version, dir string, err error) {
	t.Helper()
	previous := runtimeVersion
	runtimeVersion = func(string, string) (string, string, error) { return version, dir, err }
	t.Cleanup(func() { runtimeVersion = previous })
}

// runVersion executes the command tree as the real binary would and returns
// stdout and stderr separately, because the version line must stay clean.
func runVersion(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := NewRootCommand("1.2.3")
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(NormalizeArgs(args))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), errOut.String()
}

// TestVersionLineReportsTheRuntimeVersion pins the user-visible contract: the
// MaaFramework version comes from the loaded runtime, not from the build.
func TestVersionLineReportsTheRuntimeVersion(t *testing.T) {
	stubRuntime(t, "v9.9.9", "/libs", nil)
	want := "maactl version 1.2.3 (MaaFramework v9.9.9)\n"
	for _, args := range [][]string{{"-v"}, {"--version"}, {"version"}, {"ver"}} {
		stdout, stderr := runVersion(t, args...)
		if stdout != want {
			t.Errorf("%v = %q, want %q", args, stdout, want)
		}
		if stderr != "" {
			t.Errorf("%v wrote to stderr: %q", args, stderr)
		}
	}
}

// TestVersionLineSurvivesAMissingRuntime keeps the version query usable on a
// machine without MaaFramework: it answers with maactl's own version, exits
// successfully, and stays quiet unless asked for details. `selfcheck` is the
// command that reports a broken runtime as a failure.
func TestVersionLineSurvivesAMissingRuntime(t *testing.T) {
	stubRuntime(t, "", "", errors.New("MaaFramework libraries not found"))
	want := "maactl version 1.2.3\n"
	stdout, stderr := runVersion(t, "-v")
	if stdout != want {
		t.Errorf("-v = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("-v wrote to stderr without --verbose: %q", stderr)
	}

	stdout, stderr = runVersion(t, "-v", "-vb")
	if stdout != want {
		t.Errorf("-v -vb = %q, want %q", stdout, want)
	}
	if !strings.Contains(stderr, "MaaFramework libraries not found") {
		t.Errorf("-v -vb did not explain the missing runtime: %q", stderr)
	}
}

// TestVersionLineWithoutAVersionString covers a runtime that loads but reports
// nothing: the line stays a valid version response instead of ending in an empty
// parenthesis.
func TestVersionLineWithoutAVersionString(t *testing.T) {
	stubRuntime(t, "  ", "/libs", nil)
	stdout, _ := runVersion(t, "-v")
	if stdout != "maactl version 1.2.3\n" {
		t.Errorf("-v = %q, want the runtime version left out", stdout)
	}
}
