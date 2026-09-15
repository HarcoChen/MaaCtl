package cli

import (
	"bytes"
	"errors"
	"testing"
)

// brokenResource is a loaded resource whose metadata cannot be read, which is
// how a corrupted MaaFramework resource behaves. It implements resourceReader
// so the failure handling of the resource queries is testable without loading
// MaaFramework.
type brokenResource struct{ err error }

func (b brokenResource) GetHash() (string, error) { return "", b.err }

func (b brokenResource) GetNodeList() ([]string, error) { return nil, b.err }

// TestOutputResourceInspectFailsOnUnreadableMetadata pins that `resource
// inspect` reports a resource whose metadata cannot be read with the resource
// exit code instead of printing an empty hash and a zero node count.
func TestOutputResourceInspectFailsOnUnreadableMetadata(t *testing.T) {
	var out bytes.Buffer
	err := outputResourceInspect(&out, &GlobalOptions{}, brokenResource{err: errors.New("broken resource")},
		&resourcePlan{}, []string{"/tmp/resource"})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitResource {
		t.Fatalf("outputResourceInspect = %v, want an ExitResource error", err)
	}
	if out.Len() != 0 {
		t.Errorf("a failed inspect must not report a hash or a node count:\n%s", out.String())
	}
}

// TestOutputResourceInspectJSONFailsBeforeWriting pins that the JSON form fails
// the same way, so a broken resource never produces a valid document.
func TestOutputResourceInspectJSONFailsBeforeWriting(t *testing.T) {
	var out bytes.Buffer
	err := outputResourceInspect(&out, &GlobalOptions{JSON: true}, brokenResource{err: errors.New("broken resource")},
		&resourcePlan{}, nil)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitResource {
		t.Fatalf("outputResourceInspect = %v, want an ExitResource error", err)
	}
	if out.Len() != 0 {
		t.Errorf("a failed inspect must not write JSON:\n%s", out.String())
	}
}
