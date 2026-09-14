package pi

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
)

// FindController resolves a controller by name, defaulting to the only
// controller when name is empty.
func (l *Loaded) FindController(name string) (*Controller, error) {
	if name == "" {
		if len(l.Controller) == 1 {
			return &l.Controller[0], nil
		}
		if len(l.Controller) == 0 {
			return nil, fmt.Errorf("the ProjectInterface declares no controller")
		}
		return nil, fmt.Errorf("%d controllers found; specify --controller/-c: %s", len(l.Controller), JoinControllerNames(l.Controller))
	}
	for i := range l.Controller {
		if l.Controller[i].Name == name {
			return &l.Controller[i], nil
		}
	}
	return nil, fmt.Errorf("controller %q not found; available: %s", name, JoinControllerNames(l.Controller))
}

// FindResource resolves a resource by name that is compatible with ctrl,
// defaulting to the first compatible resource when name is empty.
func (l *Loaded) FindResource(name string, ctrl *Controller) (*Resource, error) {
	if ctrl == nil {
		if name == "" {
			if len(l.Resource) == 0 {
				return nil, fmt.Errorf("the ProjectInterface declares no resource")
			}
			return &l.Resource[0], nil
		}
		for i := range l.Resource {
			if l.Resource[i].Name == name {
				return &l.Resource[i], nil
			}
		}
		return nil, fmt.Errorf("resource %q not found; available: %s", name, JoinResourceNames(l.Resource))
	}
	for i := range l.Resource {
		r := &l.Resource[i]
		if name != "" && r.Name != name {
			continue
		}
		if Compatible(r.Controller, ctrl.Name) {
			return r, nil
		}
	}
	if name == "" {
		return nil, fmt.Errorf("no resource is compatible with controller %q", ctrl.Name)
	}
	return nil, fmt.Errorf("resource %q is missing or incompatible with controller %q", name, ctrl.Name)
}

// FindTask resolves a task by name and verifies controller and resource
// compatibility.
func (l *Loaded) FindTask(name string, ctrl *Controller, res *Resource) (*Task, error) {
	task := l.LookupTask(name)
	if task == nil {
		return nil, fmt.Errorf("task %q not found; available: %s", name, JoinTaskNames(l.Task))
	}
	if reasons := l.TaskReasons(task, ctrl, res); len(reasons) > 0 {
		return nil, fmt.Errorf("task %q is unavailable here: %s", name, strings.Join(reasons, "; "))
	}
	return task, nil
}

// LookupTask finds a task by name, then by its label in the active language,
// then by a case-insensitive name match. It does not check applicability, so
// callers can report why a task is unavailable instead of "not found".
func (l *Loaded) LookupTask(name string, lang ...string) *Task {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for i := range l.Task {
		if l.Task[i].Name == name {
			return &l.Task[i]
		}
	}
	translator := l.Translator(firstOrEmpty(lang))
	for i := range l.Task {
		if translator.Resolve(l.Task[i].Label) == name {
			return &l.Task[i]
		}
	}
	lower := strings.ToLower(name)
	for i := range l.Task {
		if strings.ToLower(l.Task[i].Name) == lower {
			return &l.Task[i]
		}
	}
	return nil
}

// FindPreset resolves a preset by name or label.
func (l *Loaded) FindPreset(name string, lang ...string) (*Preset, error) {
	name = strings.TrimSpace(name)
	for i := range l.Preset {
		if l.Preset[i].Name == name {
			return &l.Preset[i], nil
		}
	}
	translator := l.Translator(firstOrEmpty(lang))
	for i := range l.Preset {
		if translator.Resolve(l.Preset[i].Label) == name {
			return &l.Preset[i], nil
		}
	}
	names := make([]string, len(l.Preset))
	for i := range l.Preset {
		names[i] = l.Preset[i].Name
	}
	return nil, fmt.Errorf("preset %q not found; available: %s", name, Join(names))
}

// FindGroup resolves a group by name.
func (l *Loaded) FindGroup(name string) (*Group, error) {
	for i := range l.Group {
		if l.Group[i].Name == name {
			return &l.Group[i], nil
		}
	}
	names := make([]string, len(l.Group))
	for i := range l.Group {
		names[i] = l.Group[i].Name
	}
	return nil, fmt.Errorf("group %q not found; available: %s", name, Join(names))
}

// TaskReasons lists why a task cannot run with the given controller/resource.
// An empty result means the task is available.
func (l *Loaded) TaskReasons(task *Task, ctrl *Controller, res *Resource) []string {
	var reasons []string
	if ctrl != nil && !Compatible(task.Controller, ctrl.Name) {
		reasons = append(reasons, fmt.Sprintf("needs controller %s", Join(task.Controller)))
	}
	if res != nil && !Compatible(task.Resource, res.Name) {
		reasons = append(reasons, fmt.Sprintf("needs resource %s", Join(task.Resource)))
	}
	return reasons
}

// OptionActive reports whether an option participates for the given
// controller/resource, per the protocol's applicability filter.
func OptionActive(option *Option, ctrlName, resName string) bool {
	if option == nil {
		return false
	}
	return Compatible(option.Controller, ctrlName) && Compatible(option.Resource, resName)
}

// Compatible reports whether value is allowed by an allow-list. An empty list
// means "no restriction".
func Compatible(items []string, value string) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

// RunnableControllerTypes lists the controller types this build can create on
// the current platform. Types outside the list are parsed and listed, but fail
// fast with an explicit message instead of an opaque native error.
func RunnableControllerTypes() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"Adb", "Win32", "Gamepad"}
	case "darwin":
		return []string{"Adb", "MacOS", "PlayCover"}
	case "linux":
		return []string{"Adb", "Linux"}
	default:
		return []string{"Adb"}
	}
}

// ControllerRunnable reports whether this build can create ctrl.
func ControllerRunnable(ctrl *Controller) bool {
	if ctrl == nil {
		return false
	}
	for _, kind := range RunnableControllerTypes() {
		if strings.EqualFold(kind, ctrl.Type) {
			return true
		}
	}
	return false
}

// platformName names the current platform for validation messages.
func platformName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// JoinControllerNames returns the comma-separated names of controllers.
func JoinControllerNames(items []Controller) string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return Join(names)
}

// JoinResourceNames returns the comma-separated names of resources.
func JoinResourceNames(items []Resource) string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return Join(names)
}

// JoinTaskNames returns the comma-separated names of tasks.
func JoinTaskNames(items []Task) string {
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].Name
	}
	return Join(names)
}

// Join returns items separated by ", ".
func Join(items []string) string {
	var b bytes.Buffer
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(item)
	}
	return b.String()
}

// firstOrEmpty returns the first element of values, or "".
func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
