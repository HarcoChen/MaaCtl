package pi

import (
	"fmt"
	"os"
	"strings"
)

// IssueLevel separates hard errors from advisory warnings.
type IssueLevel string

// Issue levels.
const (
	LevelError   IssueLevel = "error"
	LevelWarning IssueLevel = "warning"
)

// Issue is one validation finding.
type Issue struct {
	Level   IssueLevel `json:"level"`
	Path    string     `json:"path"`
	Message string     `json:"message"`
}

// Report is the result of validating a ProjectInterface.
type Report struct {
	Issues []Issue `json:"issues"`
	Strict bool    `json:"strict"`
}

// Errors returns the number of error-level issues.
func (r *Report) Errors() int { return r.count(LevelError) }

// Warnings returns the number of warning-level issues.
func (r *Report) Warnings() int { return r.count(LevelWarning) }

// OK reports whether the ProjectInterface is usable: no errors, and in strict
// mode no warnings either. `--strict` promotes advisory findings to failures,
// which is what CI wants.
func (r *Report) OK() bool {
	if r.Errors() > 0 {
		return false
	}
	return !r.Strict || r.Warnings() == 0
}

func (r *Report) count(level IssueLevel) int {
	total := 0
	for _, issue := range r.Issues {
		if issue.Level == level {
			total++
		}
	}
	return total
}

func (r *Report) errorf(path, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{Level: LevelError, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (r *Report) warnf(path, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{Level: LevelWarning, Path: path, Message: fmt.Sprintf(format, args...)})
}

// reportf records an issue at the given level.
func (r *Report) reportf(level IssueLevel, path, format string, args ...any) {
	if level == LevelWarning {
		r.warnf(path, format, args...)
		return
	}
	r.errorf(path, format, args...)
}

// ValidateOptions controls how much Validate enforces.
type ValidateOptions struct {
	// Strict turns advisory findings (unrunnable controllers, missing resource
	// paths, missing language files) into errors.
	Strict bool
	// SkipFiles disables filesystem checks, for tests and for projects that are
	// only inspected through the model.
	SkipFiles bool
}

// Validate checks the merged ProjectInterface: name uniqueness, cross
// references, option constraints, and (unless SkipFiles) the paths that must
// exist on disk.
func (l *Loaded) Validate(opts ValidateOptions) *Report {
	r := &Report{Strict: opts.Strict}
	known := newNameIndex(l)
	l.validateNames(r)
	l.validateControllers(r, opts)
	l.validateResources(r, opts, known)
	l.validateGroups(r)
	l.validateTasks(r, known)
	l.validateOptions(r, known)
	l.validatePresets(r)
	l.validateSettings(r)
	l.validatePretasks(r, known)
	l.validateLanguages(r, opts)
	return r
}

// nameIndex caches the names declared at the ProjectInterface top level, so the
// reference checks do not rescan every list.
type nameIndex struct {
	controllers map[string]bool
	resources   map[string]bool
	groups      map[string]bool
}

// newNameIndex builds the name index of l.
func newNameIndex(l *Loaded) nameIndex {
	return nameIndex{
		controllers: nameSet(namesOf(l.Controller, func(c Controller) string { return c.Name })),
		resources:   nameSet(namesOf(l.Resource, func(r Resource) string { return r.Name })),
		groups:      nameSet(namesOf(l.Group, func(g Group) string { return g.Name })),
	}
}

// nameSet indexes names for membership tests.
func nameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// refs reports every name a declaration references that the ProjectInterface
// does not define, at the given issue level.
func refs(r *Report, level IssueLevel, pathPrefix, kind string, names []string, known map[string]bool) {
	for i, name := range names {
		if !known[name] {
			r.reportf(level, fmt.Sprintf("%s[%d]", pathPrefix, i), "references %s %q, which is not defined", kind, name)
		}
	}
}

// optionRef reports one option reference that the `option` section does not
// define, at the given path.
func (l *Loaded) optionRef(r *Report, path, name string) {
	if _, ok := l.Option.Get(name); !ok {
		r.errorf(path, "references option %q, which is not defined in `option`", name)
	}
}

// optionRefs reports every option reference a declaration makes, addressed by
// position, that the `option` section does not define.
func (l *Loaded) optionRefs(r *Report, pathPrefix string, names []string) {
	for i, name := range names {
		l.optionRef(r, fmt.Sprintf("%s[%d]", pathPrefix, i), name)
	}
}

// validateNames reports duplicate identifiers in every list keyed by name.
func (l *Loaded) validateNames(r *Report) {
	check := func(kind string, names []string) {
		seen := map[string]int{}
		for i, name := range names {
			if name == "" {
				r.errorf(fmt.Sprintf("%s[%d]", kind, i), "name must not be empty")
				continue
			}
			if first, ok := seen[name]; ok {
				r.errorf(fmt.Sprintf("%s[%d].name", kind, i), "duplicate name %q (first used by %s[%d])", name, kind, first)
				continue
			}
			seen[name] = i
		}
	}
	check("controller", namesOf(l.Controller, func(c Controller) string { return c.Name }))
	check("resource", namesOf(l.Resource, func(res Resource) string { return res.Name }))
	check("task", namesOf(l.Task, func(t Task) string { return t.Name }))
	check("group", namesOf(l.Group, func(g Group) string { return g.Name }))
	check("preset", namesOf(l.Preset, func(p Preset) string { return p.Name }))
	check("setting", namesOf(l.Setting, func(s Setting) string { return s.Name }))
}

// validateControllers checks controller types and platform support.
func (l *Loaded) validateControllers(r *Report, opts ValidateOptions) {
	for i := range l.Controller {
		ctrl := &l.Controller[i]
		path := fmt.Sprintf("controller[%d]", i)
		if strings.TrimSpace(ctrl.Type) == "" {
			r.errorf(path+".type", "type is required")
			continue
		}
		switch {
		case ControllerRunnable(ctrl):
		case isKnownControllerType(ctrl.Type):
			r.warnf(path+".type", "controller type %q cannot be created on this platform (%s); this build supports %s", ctrl.Type, platformName(), Join(RunnableControllerTypes()))
		default:
			r.errorf(path+".type", "unknown controller type %q", ctrl.Type)
		}
		l.optionRefs(r, path+".option", ctrl.Option)
		if !opts.SkipFiles {
			for j, p := range ctrl.AttachResourcePath {
				if _, err := os.Stat(resolveRelative(l.Dir, p)); err != nil {
					r.warnf(fmt.Sprintf("%s.attach_resource_path[%d]", path, j), "path %q does not exist", p)
				}
			}
		}
	}
}

// validateResources checks resource paths, controller references, and options.
func (l *Loaded) validateResources(r *Report, opts ValidateOptions, known nameIndex) {
	for i := range l.Resource {
		res := &l.Resource[i]
		path := fmt.Sprintf("resource[%d]", i)
		if len(res.Path) == 0 {
			r.errorf(path+".path", "at least one resource path is required")
		}
		if !opts.SkipFiles {
			for j, p := range res.Path {
				if _, err := os.Stat(resolveRelative(l.Dir, p)); err != nil {
					r.warnf(fmt.Sprintf("%s.path[%d]", path, j), "path %q does not exist", p)
				}
			}
		}
		refs(r, LevelError, path+".controller", "controller", res.Controller, known.controllers)
		l.optionRefs(r, path+".option", res.Option)
	}
}

// validateGroups checks group declarations.
func (l *Loaded) validateGroups(r *Report) {
	for i := range l.Group {
		if strings.TrimSpace(l.Group[i].Label) == "" && strings.TrimSpace(l.Group[i].Name) == "" {
			r.errorf(fmt.Sprintf("group[%d]", i), "name or label is required")
		}
	}
}

// validateTasks checks task references, groups, and entries.
func (l *Loaded) validateTasks(r *Report, known nameIndex) {
	for i := range l.Task {
		task := &l.Task[i]
		path := fmt.Sprintf("task[%d]", i)
		if strings.TrimSpace(task.Entry) == "" {
			r.errorf(path+".entry", "entry node is required")
		}
		refs(r, LevelError, path+".resource", "resource", task.Resource, known.resources)
		refs(r, LevelError, path+".controller", "controller", task.Controller, known.controllers)
		for j, name := range task.Group {
			if !known.groups[name] {
				r.warnf(fmt.Sprintf("%s.group[%d]", path, j), "references group %q, which is not declared at the top level", name)
			}
		}
		l.optionRefs(r, path+".option", task.Option)
	}
}

// validateOptions checks every option definition and its case children.
func (l *Loaded) validateOptions(r *Report, known nameIndex) {
	for _, name := range l.Option.Names() {
		option, _ := l.Option.Get(name)
		path := fmt.Sprintf("option.%s", name)
		refs(r, LevelWarning, path+".controller", "controller", option.Controller, known.controllers)
		refs(r, LevelWarning, path+".resource", "resource", option.Resource, known.resources)
		switch option.Kind() {
		case OptionTypeSelect, OptionTypeCheckbox, OptionTypeSwitch:
			if len(option.Cases) == 0 {
				r.errorf(path+".cases", "type %q requires at least one case", option.Kind())
			}
			seen := map[string]bool{}
			for j := range option.Cases {
				item := &option.Cases[j]
				casePath := fmt.Sprintf("%s.cases[%d]", path, j)
				if item.Name == "" {
					r.errorf(casePath+".name", "name is required")
				} else if seen[item.Name] {
					r.errorf(casePath+".name", "duplicate case name %q", item.Name)
				}
				seen[item.Name] = true
				l.optionRefs(r, casePath+".option", item.Option)
			}
			if option.Kind() == OptionTypeSwitch {
				l.validateSwitchCases(r, path, option)
			}
			if option.Kind() == OptionTypeCheckbox {
				l.validateCheckboxCounts(r, path, option)
			}
			if option.Kind() != OptionTypeCheckbox && option.DefaultCase != nil && option.DefaultCase.List != nil {
				r.errorf(path+".default_case", "type %q expects a single case name, not a list", option.Kind())
			}
			if option.Kind() == OptionTypeCheckbox && option.DefaultCase != nil && option.DefaultCase.Scalar != "" {
				r.errorf(path+".default_case", "type \"checkbox\" expects a list of case names")
			}
			if def := option.DefaultCase.String(); def != "" {
				if _, ok := option.FindCase(def); !ok {
					r.errorf(path+".default_case", "default case %q is not one of the declared cases", def)
				}
			}
			for _, def := range option.DefaultCase.Slice() {
				if _, ok := option.FindCase(def); !ok {
					r.errorf(path+".default_case", "default case %q is not one of the declared cases", def)
				}
			}
		case OptionTypeInput:
			if len(option.Inputs) == 0 {
				r.errorf(path+".inputs", "type \"input\" requires at least one input field")
			}
			for j := range option.Inputs {
				field := &option.Inputs[j]
				fieldPath := fmt.Sprintf("%s.inputs[%d]", path, j)
				if field.Name == "" {
					r.errorf(fieldPath+".name", "name is required")
				}
				if field.Password && field.Default != "" {
					r.errorf(fieldPath, "a password field must not declare a default")
				}
				switch field.PipelineType {
				case "", "string", "int", "bool":
				default:
					r.errorf(fieldPath+".pipeline_type", "unsupported pipeline_type %q", field.PipelineType)
				}
			}
		case OptionTypeHotkey:
			if len(option.Hotkeys) == 0 {
				r.errorf(path+".hotkeys", "type \"hotkey\" requires at least one hotkey field")
			}
			for j := range option.Hotkeys {
				fieldPath := fmt.Sprintf("%s.hotkeys[%d]", path, j)
				if option.Hotkeys[j].Name == "" {
					r.errorf(fieldPath+".name", "name is required")
				}
			}
		default:
			r.errorf(path+".type", "unsupported option type %q", option.Type)
		}
	}
	l.optionRefs(r, "global_option", l.GlobalOption)
}

// validateSwitchCases enforces the protocol's two-case Yes/No rule for switches.
func (l *Loaded) validateSwitchCases(r *Report, path string, option *Option) {
	if len(option.Cases) != 2 {
		r.errorf(path+".cases", "type \"switch\" requires exactly two cases, got %d", len(option.Cases))
		return
	}
	yes, no := false, false
	for _, item := range option.Cases {
		if normalized, ok := normalizeSwitch(item.Name); ok {
			if normalized == "yes" {
				yes = true
			} else {
				no = true
			}
		}
	}
	if !yes || !no {
		r.errorf(path+".cases", "type \"switch\" cases must be named Yes/No (or Y/N); got %s", Join(option.CaseNames()))
	}
}

// validateCheckboxCounts checks min_count / max_count consistency.
func (l *Loaded) validateCheckboxCounts(r *Report, path string, option *Option) {
	total := len(option.Cases)
	if min := option.MinCount; min != nil {
		if *min < 0 {
			r.errorf(path+".min_count", "must not be negative")
		}
		if *min > total {
			r.errorf(path+".min_count", "must not exceed the number of cases (%d)", total)
		}
	}
	if max := option.MaxCount; max != nil {
		if *max < 0 {
			r.errorf(path+".max_count", "must not be negative")
		}
		if *max > total {
			r.errorf(path+".max_count", "must not exceed the number of cases (%d)", total)
		}
	}
	if option.MinCount != nil && option.MaxCount != nil && *option.MinCount > *option.MaxCount {
		r.errorf(path, "min_count (%d) must not exceed max_count (%d)", *option.MinCount, *option.MaxCount)
	}
	if option.DefaultCase != nil {
		l.validateDefaultCount(r, path, option, len(option.DefaultCase.Slice()))
	}
}

func (l *Loaded) validateDefaultCount(r *Report, path string, option *Option, count int) {
	if option.MinCount != nil && count < *option.MinCount {
		r.errorf(path+".default_case", "selects %d case(s), fewer than min_count %d", count, *option.MinCount)
	}
	if option.MaxCount != nil && count > *option.MaxCount {
		r.errorf(path+".default_case", "selects %d case(s), more than max_count %d", count, *option.MaxCount)
	}
}

// validatePresets checks that preset tasks exist.
func (l *Loaded) validatePresets(r *Report) {
	for i := range l.Preset {
		preset := &l.Preset[i]
		for j := range preset.Task {
			entry := &preset.Task[j]
			path := fmt.Sprintf("preset[%d].task[%d]", i, j)
			if strings.TrimSpace(entry.Name) == "" {
				r.errorf(path+".name", "name is required")
				continue
			}
			if l.LookupTask(entry.Name) == nil {
				r.errorf(path+".name", "references task %q, which is not defined", entry.Name)
			}
			for optionName := range entry.Option {
				l.optionRef(r, path+".option."+optionName, optionName)
			}
		}
	}
}

// validateSettings checks setting sections and their option lists.
func (l *Loaded) validateSettings(r *Report) {
	for i := range l.Setting {
		l.optionRefs(r, fmt.Sprintf("setting[%d].option", i), l.Setting[i].Option)
	}
}

// validatePretasks checks pretask identifiers and option references.
func (l *Loaded) validatePretasks(r *Report, known nameIndex) {
	for i := range l.Pretask {
		pretask := &l.Pretask[i]
		if strings.TrimSpace(pretask.Exec) == "" {
			r.errorf(fmt.Sprintf("pretask[%d].exec", i), "exec is required")
		}
		l.optionRefs(r, fmt.Sprintf("pretask[%d].option", i), pretask.Option)
		refs(r, LevelWarning, fmt.Sprintf("pretask[%d].resource", i), "resource", pretask.Resource, known.resources)
		refs(r, LevelWarning, fmt.Sprintf("pretask[%d].controller", i), "controller", pretask.Controller, known.controllers)
	}
}

// validateLanguages reports declared language files that cannot be read.
func (l *Loaded) validateLanguages(r *Report, opts ValidateOptions) {
	if opts.SkipFiles {
		return
	}
	for _, code := range sortedLanguages(l.Languages) {
		file := l.Languages[code]
		if strings.TrimSpace(file) == "" {
			r.errorf("languages."+code, "language file path is empty")
			continue
		}
		if _, err := os.Stat(resolveRelative(l.Dir, file)); err != nil {
			r.warnf("languages."+code, "language file %q does not exist", file)
		}
	}
}

// isKnownControllerType reports whether name is one of the protocol's types.
func isKnownControllerType(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "adb", "win32", "macos", "playcover", "gamepad", "linux":
		return true
	default:
		return false
	}
}
