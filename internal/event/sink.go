package event

import (
	"encoding/json"
	"fmt"
	"sync"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
)

// Event output modes accepted by --events.
const (
	ModeFocus = "focus"
	ModeAll   = "all"
	ModeOff   = "off"
)

// ParseMode validates an --events value.
func ParseMode(value string) (string, error) {
	switch value {
	case ModeFocus, ModeAll, ModeOff:
		return value, nil
	}
	return "", fmt.Errorf("invalid --events value %q; use focus, all, or off", value)
}

// Sink receives MaaFramework tasker and context callbacks and prints them
// according to the selected event mode.
type Sink struct {
	JSON bool
	Mode string
	mu   sync.Mutex
}

// Output renders one sink event. In focus mode only Pipeline focus texts are
// printed; in all mode every event and its detail is printed.
func (s *Sink) Output(kind string, status maa.EventStatus, detail any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(detail)
	var values map[string]any
	_ = json.Unmarshal(b, &values)
	message := kind + "." + EventName(status)
	focus := RenderFocus(message, values)
	if s.Mode == ModeOff || (s.Mode == ModeFocus && focus == "") {
		return
	}
	if s.JSON {
		out := map[string]any{"focus": focus}
		if s.Mode == ModeAll {
			out["event"] = message
			out["status"] = status
			out["detail"] = values
		}
		b, _ = json.Marshal(out)
		fmt.Println(string(b))
		return
	}
	if s.Mode == ModeFocus {
		fmt.Printf("%s\n", focus)
		return
	}
	if focus != "" {
		fmt.Printf("%s\n", focus)
	}
	fmt.Printf("%s %s\n", message, b)
}

// EventName returns the MaaFramework status name used in event messages.
func EventName(event maa.EventStatus) string {
	switch event {
	case maa.EventStatusStarting:
		return "Starting"
	case maa.EventStatusSucceeded:
		return "Succeeded"
	case maa.EventStatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// TaskerSink forwards tasker-level callbacks to a Sink.
type TaskerSink struct{ Sink *Sink }

func (s *TaskerSink) OnResourceLoading(_ *maa.Tasker, e maa.EventStatus, d maa.ResourceLoadingDetail) {
	s.Sink.Output("Resource.Loading", e, d)
}
func (s *TaskerSink) OnControllerAction(_ *maa.Tasker, e maa.EventStatus, d maa.ControllerActionDetail) {
	s.Sink.Output("Controller.Action", e, d)
}
func (s *TaskerSink) OnTaskerTask(_ *maa.Tasker, e maa.EventStatus, d maa.TaskerTaskDetail) {
	s.Sink.Output("Tasker.Task", e, d)
}
func (s *TaskerSink) OnNodePipelineNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodePipelineNodeDetail) {
	s.Sink.Output("Node.PipelineNode", e, d)
}
func (s *TaskerSink) OnNodeRecognitionNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodeRecognitionNodeDetail) {
	s.Sink.Output("Node.RecognitionNode", e, d)
}
func (s *TaskerSink) OnNodeActionNode(_ *maa.Tasker, e maa.EventStatus, d maa.NodeActionNodeDetail) {
	s.Sink.Output("Node.ActionNode", e, d)
}
func (s *TaskerSink) OnTaskNextList(_ *maa.Tasker, e maa.EventStatus, d maa.NodeNextListDetail) {
	s.Sink.Output("Node.NextList", e, d)
}
func (s *TaskerSink) OnTaskRecognition(_ *maa.Tasker, e maa.EventStatus, d maa.NodeRecognitionDetail) {
	s.Sink.Output("Node.Recognition", e, d)
}
func (s *TaskerSink) OnTaskAction(_ *maa.Tasker, e maa.EventStatus, d maa.NodeActionDetail) {
	s.Sink.Output("Node.Action", e, d)
}
func (s *TaskerSink) OnUnknownEvent(_ *maa.Tasker, msg, details string) {
	s.Sink.unknown(msg, details)
}

// ContextSink receives node-level callbacks, including the focus field.
// It forwards their original event category and status to the same Sink.
type ContextSink struct{ Sink *Sink }

func (s *ContextSink) OnResourceLoading(_ *maa.Context, e maa.EventStatus, d maa.ResourceLoadingDetail) {
	s.Sink.Output("Resource.Loading", e, d)
}
func (s *ContextSink) OnControllerAction(_ *maa.Context, e maa.EventStatus, d maa.ControllerActionDetail) {
	s.Sink.Output("Controller.Action", e, d)
}
func (s *ContextSink) OnTaskerTask(_ *maa.Context, e maa.EventStatus, d maa.TaskerTaskDetail) {
	s.Sink.Output("Tasker.Task", e, d)
}
func (s *ContextSink) OnNodePipelineNode(_ *maa.Context, e maa.EventStatus, d maa.NodePipelineNodeDetail) {
	s.Sink.Output("Node.PipelineNode", e, d)
}
func (s *ContextSink) OnNodeRecognitionNode(_ *maa.Context, e maa.EventStatus, d maa.NodeRecognitionNodeDetail) {
	s.Sink.Output("Node.RecognitionNode", e, d)
}
func (s *ContextSink) OnNodeActionNode(_ *maa.Context, e maa.EventStatus, d maa.NodeActionNodeDetail) {
	s.Sink.Output("Node.ActionNode", e, d)
}
func (s *ContextSink) OnTaskNextList(_ *maa.Context, e maa.EventStatus, d maa.NodeNextListDetail) {
	s.Sink.Output("Node.NextList", e, d)
}
func (s *ContextSink) OnTaskRecognition(_ *maa.Context, e maa.EventStatus, d maa.NodeRecognitionDetail) {
	s.Sink.Output("Node.Recognition", e, d)
}
func (s *ContextSink) OnTaskAction(_ *maa.Context, e maa.EventStatus, d maa.NodeActionDetail) {
	s.Sink.Output("Node.Action", e, d)
}
func (s *ContextSink) OnUnknownEvent(_ *maa.Context, msg, details string) {
	s.Sink.unknown(msg, details)
}

func (s *Sink) unknown(msg, details string) {
	if s.Mode != ModeAll {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Printf("%s %s\n", msg, details)
}
