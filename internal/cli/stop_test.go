package cli

import (
	"errors"
	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWaitExecution(t *testing.T) {
	for _, tt := range []struct {
		name                                  string
		cancel, timeout, blockTask, blockStop bool
		task, stop                            maa.Status
		want                                  string
		stopFailed                            bool
	}{
		{name: "success", task: maa.StatusSuccess},
		{name: "task failure", task: maa.StatusFailure, want: "task finished"},
		{name: "cancel unbounded", cancel: true, task: maa.StatusFailure, stop: maa.StatusSuccess, want: "task cancelled"},
		{name: "timeout", timeout: true, task: maa.StatusFailure, stop: maa.StatusSuccess, want: ""},
		{name: "stop failure", cancel: true, task: maa.StatusFailure, stop: maa.StatusFailure, want: "stop", stopFailed: true},
		{name: "original job stuck", cancel: true, blockTask: true, stop: maa.StatusSuccess, want: "stop", stopFailed: true},
		{name: "stop job stuck", cancel: true, blockStop: true, task: maa.StatusFailure, want: "stop", stopFailed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signals := make(chan os.Signal, 1)
			taskRelease := make(chan struct{})
			stopRelease := make(chan struct{})
			defer close(stopRelease)
			defer close(taskRelease)
			startedStop := make(chan struct{})
			if tt.cancel {
				signals <- os.Interrupt
			}
			limit := time.Duration(0)
			if tt.timeout {
				limit = time.Millisecond
			}
			err := waitExecution(func() maa.Status {
				if tt.cancel || tt.timeout {
					<-startedStop
				}
				if tt.blockTask {
					<-taskRelease
				}
				return tt.task
			}, func() maa.Status {
				close(startedStop)
				if tt.blockStop {
					<-stopRelease
				}
				return tt.stop
			}, signals, limit, 20*time.Millisecond)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q, got %v", tt.want, err)
			}
			var stopErr *stopError
			if errors.As(err, &stopErr) != tt.stopFailed {
				t.Fatalf("unexpected stop failure classification: %v", err)
			}

		})
	}
}

func TestStopWaitsForOriginalTask(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	release := make(chan struct{})
	stopDone := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- waitExecution(func() maa.Status { <-release; return maa.StatusFailure }, func() maa.Status { close(stopDone); return maa.StatusSuccess }, signals, 0, time.Second)
	}()
	<-stopDone
	select {
	case err := <-result:
		t.Fatalf("returned before original job ended: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "task cancelled") {
		t.Fatalf("got %v", err)
	}
}
