package pi

import (
	"fmt"
	"os"
	"path/filepath"
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
	l.validateNames(r)
	l.validateControllers(r, opts)
	l.validateResources(r, opts)
	l.validateGroups(r)
	l.validateTasks(r)
	l.validateOptions(r)
	l.validatePresets(r)
	l.validateSettings(r)
	l.validatePretasks(r)
	l.validateLanguages(r, opts)
	return r
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
	controllerNames := make([]string, len(l.Controller))
	for i := range l.Controller {
		controllerNames[i] = l.Controller[i].Name
	}
	check("controller", controllerNames)
	resourceNames := make([]string, len(l.Resource))
	for i := range l.Resource {
		resourceNames[i] = l.Resource[i].Name
	}
	check("resource", resourceNames)
	taskNames := make([]string, len(l.Task))
	for i := range l.Task {
		taskNames[i] = l.Task[i].Name
	}
	check("task", taskNames)
	groupNames := make([]string, len(l.Group))
	for i := range l.Group {
		groupNames[i] = l.Group[i].Name
	}
	check("group", groupNames)
	presetNames := make([]string, len(l.Preset))
	for i := range l.Preset {
		presetNames[i] = l.Preset[i].Name
	}
	check("preset", presetNames)
	settingNames := make([]string, len(l.Setting))
	for i := range l.Setting {
		settingNames[i] = l.Setting[i].Name
	}
	check("setting", settingNames)
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
		for j, name := range ctrl.Option {
			if _, ok := l.Option.Get(name); !ok {
				r.errorf(fmt.Sprintf("%s.option[%d]", path, j), "references option %q, which is not defined in `option`", name)
			}
		}
		if !opts.SkipFiles {
			for j, p := range ctrl.AttachResourcePath {
				if _, err := os.Stat(filepath.Join(l.Dir, filepath.FromSlash(p))); err != nil {
					r.warnf(fmt.Sprintf("%s.attach_resource_path[%d]", path, j), "path %q does not exist", p)
				}
			}
		}
	}
}

// validateResources checks resource paths, controller references, and options.
func (l *Loaded) validateResources(r *Report, opts ValidateOptions) {
	knownControllers := map[string]bool{}
	for i := range l.Controller {
		knownControllers[l.Controller[i].Name] = true
	}
	for i := range l.Resource {
		res := &l.Resource[i]
		path := fmt.Sprintf("resource[%d]", i)
		if len(res.Path) == 0 {
			r.errorf(path+".path", "at least one resource path is required")
		}
		if !opts.SkipFiles {
			for j, p := range res.Path {
				if _, err := os.Stat(filepath.Join(l.Dir, filepath.FromSlash(p))); err != nil {
					r.warnf(fmt.Sprintf("%s.path[%d]", path, j), "path %q does not exist", p)
				}
			}
		}
		for j, name := range res.Controller {
			if !knownControllers[name] {
				r.errorf(fmt.Sprintf("%s.controller[%d]", path, j), "references controller %q, which is not defined", name)
			}
		}
		for j, name := range res.Option {
			if _, ok := l.Option.Get(name); !ok {
				r.errorf(fmt.Sprintf("%s.option[%d]", path, j), "references option %q, which is not defined in `option`", name)
			}
		}
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
func (l *Loaded) validateTasks(r *Report) {
	knownControllers := map[string]bool{}
	for i := range l.Controller {
		knownControllers[l.Controller[i].Name] = true
	}
	knownResources := map[string]bool{}
	for i := range l.Resource {
		knownResources[l.Resource[i].Name] = true
	}
	knownGroups := map[string]bool{}
	for i := range l.Group {
		knownGroups[l.Group[i].Name] = true
	}
	for i := range l.Task {
		task := &l.Task[i]
		path := fmt.Sprintf("task[%d]", i)
		if strings.TrimSpace(task.Entry) == "" {
			r.errorf(path+".entry", "entry node is required")
		}
		for j, name := range task.Resource {
			if !knownResources[name] {
				r.errorf(fmt.Sprintf("%s.resource[%d]", path, j), "references resource %q, which is not defined", name)
			}
		}
		for j, name := range task.Controller {
			if !knownControllers[name] {
				r.errorf(fmt.Sprintf("%s.controller[%d]", path, j), "references controller %q, which is not defined", name)
			}
		}
		for j, name := range task.Group {
			if !knownGroups[name] {
				r.warnf(fmt.Sprintf("%s.group[%d]", path, j), "references group %q, which is not declared at the top level", name)
			}
		}
		for j, name := range task.Option {
			if _, ok := l.Option.Get(name); !ok {
				r.errorf(fmt.Sprintf("%s.option[%d]", path, j), "references option %q, which is not defined in `option`", name)
			}
		}
	}
}

// validateOptions checks every option definition and its case children.
func (l *Loaded) validateOptions(r *Report) {
	for _, name := range l.Option.Names() {
		option, _ := l.Option.Get(name)
		path := fmt.Sprintf("option.%s", name)
		for j, ctrl := range option.Controller {
			if !l.hasController(ctrl) {
				r.warnf(fmt.Sprintf("%s.controller[%d]", path, j), "references controller %q, which is not defined", ctrl)
			}
		}
		for j, res := range option.Resource {
			if !l.hasResource(res) {
				r.warnf(fmt.Sprintf("%s.resource[%d]", path, j), "references resource %q, which is not defined", res)
			}
		}
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
				for k, child := range item.Option {
					if _, ok := l.Option.Get(child); !ok {
						r.errorf(fmt.Sprintf("%s.option[%d]", casePath, k), "references option %q, which is not defined in `option`", child)
					}
				}
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
	for i, name := range l.GlobalOption {
		if _, ok := l.Option.Get(name); !ok {
			r.errorf(fmt.Sprintf("global_option[%d]", i), "references option %q, which is not defined in `option`", name)
		}
	}
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
				if _, ok := l.Option.Get(optionName); !ok {
					r.errorf(path+".option."+optionName, "references option %q, which is not defined in `option`", optionName)
				}
			}
		}
	}
}

// validateSettings checks setting sections and their option lists.
func (l *Loaded) validateSettings(r *Report) {
	for i := range l.Setting {
		for j, name := range l.Setting[i].Option {
			if _, ok := l.Option.Get(name); !ok {
				r.errorf(fmt.Sprintf("setting[%d].option[%d]", i, j), "references option %q, which is not defined in `option`", name)
			}
		}
	}
}

// validatePretasks checks pretask identifiers and option references.
func (l *Loaded) validatePretasks(r *Report) {
	for i := range l.Pretask {
		pretask := &l.Pretask[i]
		if strings.TrimSpace(pretask.Exec) == "" {
			r.errorf(fmt.Sprintf("pretask[%d].exec", i), "exec is required")
		}
		for j, name := range pretask.Option {
			if _, ok := l.Option.Get(name); !ok {
				r.errorf(fmt.Sprintf("pretask[%d].option[%d]", i, j), "references option %q, which is not defined in `option`", name)
			}
		}
		for j, name := range pretask.Resource {
			if !l.hasResource(name) {
				r.warnf(fmt.Sprintf("pretask[%d].resource[%d]", i, j), "references resource %q, which is not defined", name)
			}
		}
		for j, name := range pretask.Controller {
			if !l.hasController(name) {
				r.warnf(fmt.Sprintf("pretask[%d].controller[%d]", i, j), "references controller %q, which is not defined", name)
			}
		}
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
		if _, err := os.Stat(filepath.Join(l.Dir, filepath.FromSlash(file))); err != nil {
			r.warnf("languages."+code, "language file %q does not exist", file)
		}
	}
}

// hasController reports whether a controller name is declared.
func (l *Loaded) hasController(name string) bool {
	for i := range l.Controller {
		if l.Controller[i].Name == name {
			return true
		}
	}
	return false
}

// hasResource reports whether a resource name is declared.
func (l *Loaded) hasResource(name string) bool {
	for i := range l.Resource {
		if l.Resource[i].Name == name {
			return true
		}
	}
	return false
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
