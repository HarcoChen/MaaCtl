package pi

// OptionPlanEntry is one option in the display tree produced by OptionPlan.
type OptionPlanEntry struct {
	// Layer is the PI field that references the option.
	Layer Layer
	// Name is the option key.
	Name string
	// Parent is the option whose case references this one ("" at top level).
	Parent string
	// Depth is the nesting depth.
	Depth int
	// Option is the definition (nil when the name is not defined).
	Option *Option
	// Active reports whether the option applies to the selected
	// controller/resource.
	Active bool
	// Reason explains an inactive entry.
	Reason string
}

// OptionPlan walks global_option, resource.option, controller.option, and
// task.option in merge order and returns the option tree they activate. It
// performs no value resolution, so it works for introspection on projects whose
// options have no defaults yet.
//
// With all=false, options that do not apply to the selected controller/resource
// are omitted entirely. With all=true they are included and marked inactive so
// `pi options --all` can explain the filtering.
func (l *Loaded) OptionPlan(controllerName, resName string, task *Task, all bool) []OptionPlanEntry {
	type layerOptions struct {
		layer Layer
		names []string
	}
	layers := []layerOptions{{LayerGlobal, l.GlobalOption}}
	for i := range l.Resource {
		if l.Resource[i].Name == resName {
			layers = append(layers, layerOptions{LayerResource, l.Resource[i].Option})
		}
	}
	for i := range l.Controller {
		if l.Controller[i].Name == controllerName {
			layers = append(layers, layerOptions{LayerController, l.Controller[i].Option})
		}
	}
	if task != nil {
		layers = append(layers, layerOptions{LayerTask, task.Option})
	}

	var entries []OptionPlanEntry
	for _, layer := range layers {
		for _, name := range layer.names {
			entries = append(entries, l.planOption(name, layer.layer, "", controllerName, resName, nil, 0, true, all)...)
		}
	}
	if all {
		entries = append(entries, l.planUnreferenced(controllerName, resName, task)...)
	}
	return entries
}

// planOption builds the tree for one referenced option.
func (l *Loaded) planOption(name string, layer Layer, parent, controllerName, resName string, path *optionPath, depth int, parentActive, all bool) []OptionPlanEntry {
	option, ok := l.Option.Get(name)
	if !ok {
		return []OptionPlanEntry{{Layer: layer, Name: name, Parent: parent, Depth: depth, Reason: "not defined in `option`"}}
	}
	active := parentActive && OptionActive(option, controllerName, resName)
	entry := OptionPlanEntry{Layer: layer, Name: name, Parent: parent, Depth: depth, Option: option, Active: active}
	if !active {
		switch {
		case !parentActive:
			entry.Reason = "its parent option does not apply"
		case !Compatible(option.Controller, controllerName):
			entry.Reason = "controller must be one of " + Join(option.Controller)
		case !Compatible(option.Resource, resName):
			entry.Reason = "resource must be one of " + Join(option.Resource)
		}
	}
	if !active && !all {
		return nil
	}
	if path != nil && path.has(name) {
		entry.Active = false
		entry.Reason = "cyclic reference: " + path.chain(name)
		return []OptionPlanEntry{entry}
	}
	next := &optionPath{parent: path, name: name}
	entries := []OptionPlanEntry{entry}
	for i := range option.Cases {
		for _, child := range option.Cases[i].Option {
			entries = append(entries, l.planOption(child, layer, name, controllerName, resName, next, depth+1, active, all)...)
		}
	}
	return entries
}

// planUnreferenced appends the options no layer mentions, so `pi options --all`
// shows the full definition list.
func (l *Loaded) planUnreferenced(controllerName, resName string, task *Task) []OptionPlanEntry {
	referenced := map[string]bool{}
	for _, entry := range l.OptionPlan(controllerName, resName, task, false) {
		referenced[entry.Name] = true
	}
	var entries []OptionPlanEntry
	for _, name := range l.Option.Names() {
		if referenced[name] {
			continue
		}
		option, _ := l.Option.Get(name)
		active := OptionActive(option, controllerName, resName)
		entry := OptionPlanEntry{Layer: "option", Name: name, Option: option, Active: active}
		if !active {
			entry.Reason = "not referenced by any layer"
		}
		entries = append(entries, entry)
		for i := range option.Cases {
			for _, child := range option.Cases[i].Option {
				entries = append(entries, l.planOption(child, "option", name, controllerName, resName, nil, 1, active, true)...)
			}
		}
	}
	return entries
}
