package cli

import (
	"errors"
	"fmt"
)

// Stable exit codes. Scripts can rely on these to tell a bad ProjectInterface
// from a device problem or a failed task.
const (
	// ExitOK is returned when the command succeeded.
	ExitOK = 0
	// ExitInternal is a framework or programming error.
	ExitInternal = 1
	// ExitUsage covers argument errors, ambiguous selections, and PI validation.
	ExitUsage = 2
	// ExitResource covers resource loading and hash mismatches.
	ExitResource = 3
	// ExitController covers controller discovery and connection failures.
	ExitController = 4
	// ExitPretask covers pretask failures.
	ExitPretask = 5
	// ExitTask covers a task that finished unsuccessfully.
	ExitTask = 6
	// ExitTimeout covers --timeout expiry.
	ExitTimeout = 7
	// ExitInterrupted covers Ctrl+C and other interrupts.
	ExitInterrupted = 8
)

// ExitError carries the process exit code for a failure.
type ExitError struct {
	Code int
	Err  error
}

// Error implements error.
func (e *ExitError) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped error for errors.Is/As.
func (e *ExitError) Unwrap() error { return e.Err }

// exitErrorf builds an ExitError from a formatted message.
func exitErrorf(code int, format string, args ...any) error {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// withExitCode attaches a code to an error unless it already carries one.
func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	var existing *ExitError
	if errors.As(err, &existing) {
		return err
	}
	return &ExitError{Code: code, Err: err}
}
