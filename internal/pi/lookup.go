package pi

import (
	"bytes"
	"fmt"
)

// FindController resolves a controller by name, defaulting to the only
// controller when name is empty.
func (p *Loaded) FindController(name string) (*Controller, error) {
	if name == "" {
		if len(p.Controller) == 1 {
			return &p.Controller[0], nil
		}
		return nil, fmt.Errorf("%d controllers found; specify --controller/-c: %s", len(p.Controller), JoinControllerNames(p.Controller))
	}
	for i := range p.Controller {
		if p.Controller[i].Name == name {
			return &p.Controller[i], nil
		}
	}
	return nil, fmt.Errorf("controller %q not found", name)
}

// FindResource resolves a resource by name that is compatible with ctrl,
// defaulting to the first compatible resource when name is empty.
func (p *Loaded) FindResource(name string, ctrl *Controller) (*Resource, error) {
	for i := range p.Resource {
		r := &p.Resource[i]
		if name != "" && r.Name != name {
			continue
		}
		if Compatible(r.Controller, ctrl.Name) {
			return r, nil
		}
	}
	if name == "" {
		return nil, fmt.Errorf("no resource compatible with controller %q", ctrl.Name)
	}
	return nil, fmt.Errorf("resource %q is missing or incompatible with controller %q", name, ctrl.Name)
}

// FindTask resolves a task by name and verifies controller and resource compatibility.
func (p *Loaded) FindTask(name string, ctrl *Controller, res *Resource) (*Task, error) {
	for i := range p.Task {
		if p.Task[i].Name == name {
			t := &p.Task[i]
			if !Compatible(t.Controller, ctrl.Name) || !Compatible(t.Resource, res.Name) {
				return nil, fmt.Errorf("task %q is incompatible with controller %q or resource %q", name, ctrl.Name, res.Name)
			}
			return t, nil
		}
	}
	return nil, fmt.Errorf("task %q not found", name)
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

// JoinControllerNames returns the comma-separated names of controllers.
func JoinControllerNames(items []Controller) string {
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
