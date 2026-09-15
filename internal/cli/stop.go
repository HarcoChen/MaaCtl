package cli

import (
	"fmt"
	"os"
	"time"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// stopError marks unconfirmed stops so native objects are not destroyed while
// potentially in use. It does not define a separate process exit code.
type stopError struct{ message string }

func (e *stopError) Error() string { return e.message }

// waitExecution keeps listening for cancellation even for unbounded tasks.
// Each native Wait runs once. Stop completion and original task completion
// must both be observed before callers may destroy native objects.
func waitExecution(waitTask func() maa.Status, waitStop func() maa.Status, signals <-chan os.Signal, runLimit, stopLimit time.Duration) error {
	done := make(chan maa.Status, 1)
	go func(ch chan<- maa.Status) { ch <- waitTask() }(done)
	var deadline <-chan time.Time
	if runLimit > 0 {
		timer := time.NewTimer(runLimit)
		defer timer.Stop()
		deadline = timer.C
	}
	cause := "scheduled stop (--stop-after)"
	cancelled := false
	select {
	case <-signals:
		cause = "task cancelled"
		cancelled = true
	case <-deadline:
	case status := <-done:
		if !status.Success() {
			return fmt.Errorf("task finished with %s", status)
		}
		return nil
	}
	stopped := make(chan maa.Status, 1)
	go func(ch chan<- maa.Status) { ch <- waitStop() }(stopped)
	timer := time.NewTimer(stopLimit)
	defer timer.Stop()
	var stopStatus, taskStatus maa.Status
	for done != nil || stopped != nil {
		select {
		case taskStatus = <-done:
			done = nil
		case stopStatus = <-stopped:
			stopped = nil
		case <-timer.C:
			return &stopError{fmt.Sprintf("%s; stop did not complete within %s", cause, stopLimit)}
		}
	}
	if !taskStatus.Done() {
		return &stopError{fmt.Sprintf("%s; original task returned nonterminal status: %s", cause, taskStatus)}
	}
	if !stopStatus.Success() {
		return &stopError{fmt.Sprintf("%s; stop job failed: %s", cause, stopStatus)}
	}
	if cancelled {
		return fmt.Errorf("task cancelled")
	}
	return nil
}
