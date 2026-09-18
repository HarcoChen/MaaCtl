package cli

import (
	"errors"
	"testing"
	"time"
)

// TestExecuteRejectsStopTimingFlags pins the --stop-after/--stop-timeout
// contract: a run whose stop timing cannot mean anything must fail as a usage
// error. The check runs before MaaFramework is initialized, so this test needs
// no runtime.
func TestExecuteRejectsStopTimingFlags(t *testing.T) {
	for name, opt := range map[string]runOptions{
		"--stop-after negative":   {stopAfter: -time.Second, stopTimeout: 8 * time.Second},
		"--stop-after zero":       {stopTimeout: 8 * time.Second},
		"--stop-timeout zero":     {stopTimeout: 0},
		"--stop-timeout negative": {stopTimeout: -time.Second},
	} {
		prepared := &preparedRun{global: &GlobalOptions{}, opt: opt}
		err := prepared.execute()
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitUsage {
			t.Errorf("%s: err = %v, want a usage exit error", name, err)
		}
	}
}

// TestStopExitCodes pins how an unconfirmed stop is reported, because those
// codes are documented for scripts: a --timeout stop keeps the timeout code, a
// --stop-after stop means the run failed, and a signal keeps the interrupt code.
func TestStopExitCodes(t *testing.T) {
	for _, tc := range []struct {
		cause stopCause
		want  int
	}{
		{stopCauseTimeout, ExitTimeout},
		{stopCauseStopAfter, ExitTask},
		{stopCauseNone, ExitInterrupted},
	} {
		if got := stopExitCode(tc.cause); got != tc.want {
			t.Errorf("stopExitCode(%v) = %d, want %d", tc.cause, got, tc.want)
		}
	}
}

// TestStopLimitPrefersTheEarlierFlag checks which flag is treated as the trigger
// when both are set: the earlier deadline is the one that fires first, and it
// decides what the run means.
func TestStopLimitPrefersTheEarlierFlag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opt   runOptions
		want  time.Duration
		cause stopCause
	}{
		{"neither", runOptions{stopTimeout: 8 * time.Second}, 0, stopCauseNone},
		{"stop-after only", runOptions{stopAfter: time.Minute, stopTimeout: 8 * time.Second}, time.Minute, stopCauseStopAfter},
		{"timeout only", runOptions{timeout: 30 * time.Second, stopTimeout: 8 * time.Second}, 30 * time.Second, stopCauseTimeout},
		{"timeout first", runOptions{timeout: time.Minute, stopAfter: 5 * time.Minute, stopTimeout: 8 * time.Second}, time.Minute, stopCauseTimeout},
		{"stop-after first", runOptions{timeout: 5 * time.Minute, stopAfter: time.Minute, stopTimeout: 8 * time.Second}, time.Minute, stopCauseStopAfter},
	} {
		prepared := &preparedRun{global: &GlobalOptions{}, opt: tc.opt}
		limit, cause := prepared.stopLimit()
		if limit != tc.want || cause != tc.cause {
			t.Errorf("%s: stopLimit() = (%s, %v), want (%s, %v)", tc.name, limit, cause, tc.want, tc.cause)
		}
	}
}
