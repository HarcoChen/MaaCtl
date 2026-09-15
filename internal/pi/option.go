package pi

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Layer identifies which PI field contributed an active option.
type Layer string

// Option layers, in the protocol's merge order (lowest priority first).
const (
	LayerGlobal     Layer = "global_option"
	LayerResource   Layer = "resource.option"
	LayerController Layer = "controller.option"
	LayerTask       Layer = "task.option"
)

// ValueSource records where a concrete option value came from.
type ValueSource string

// Value sources, in increasing priority. "default" means the option's own
// default_case / inputs[].default / hotkeys[].default.
const (
	SourceDefault    ValueSource = "default"
	SourceConfig     ValueSource = "config"
	SourcePreset     ValueSource = "preset"
	SourceOptionFile ValueSource = "option-file"
	SourceCLI        ValueSource = "cli"
)

// Request describes one option resolution: which controller/resource/task is
// selected, plus every place an option value may come from.
type Request struct {
	// ControllerName and ResourceName drive applicability filtering. They may
	// be empty for introspection.
	ControllerName string
	ResourceName   string
	// Task supplies task.option, task.pipeline_override, and the preset entry
	// that applies to it. May be nil when only option layers are wanted.
	Task *Task
	// ConfigGlobal / ConfigTask are values saved by the user in the client
	// config file; ConfigTask applies to Task.
	ConfigGlobal map[string]any
	ConfigTask   map[string]any
	// PresetOptions is this task's entry inside the selected preset.
	PresetOptions map[string]any
	// OptionFile / CLI are the --option-file and --option values.
	OptionFile map[string]any
	CLI        map[string]any
	// ExtraOverride is the caller's final Pipeline override (--override).
	ExtraOverride Pipeline
	// LookupEnv resolves `{"env": "NAME"}` password references; nil means
	// os.LookupEnv.
	LookupEnv func(string) (string, bool)
}

// Selection is one resolved option together with the value that was used.
type Selection struct {
	Name string
	// Layer is the PI field that activated the option.
	Layer Layer
	// Source is where the value came from.
	Source ValueSource
	// Parent is the name of the option whose case activated this one.
	Parent string
	// Depth is the nesting depth (0 for top-level options).
	Depth int
	// Cases holds the selected case names. select/switch have one entry;
	// checkbox has one entry per selected case, in definition order.
	Cases []string
	// Inputs holds input/hotkey field values as plain strings (passwords in
	// clear; use MaskedInputs for anything that may be displayed).
	Inputs map[string]string
	// Secrets marks the Inputs entries that must never be printed.
	Secrets map[string]bool
}

// Value returns the selection in the protocol's OptionValue shape: a case name
// for select/switch, a case-name list for checkbox, and a field map for
// input/hotkey. It is the form pretask arguments and --explain use.
func (s Selection) Value() any {
	switch {
	case len(s.Inputs) > 0:
		out := make(map[string]string, len(s.Inputs))
		for key, value := range s.Inputs {
			out[key] = value
		}
		return out
	case len(s.Cases) > 0:
		if len(s.Cases) == 1 {
			return s.Cases[0]
		}
		return append([]string(nil), s.Cases...)
	default:
		return nil
	}
}

// MaskedInputs returns the input values with password fields replaced.
func (s Selection) MaskedInputs() map[string]string {
	if len(s.Inputs) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.Inputs))
	for key, value := range s.Inputs {
		if s.Secrets[key] {
			out[key] = "******"
			continue
		}
		out[key] = value
	}
	return out
}

// MaskedValue is Value with password fields replaced, safe for display.
func (s Selection) MaskedValue() any {
	if len(s.Secrets) == 0 {
		return s.Value()
	}
	return s.MaskedInputs()
}

// Contribution is one Pipeline override layer produced during resolution.
type Contribution struct {
	// Label identifies who contributed, e.g. "global_option 战斗划火柴=普通划火柴".
	Label string
	// Source is the value source behind the contribution.
	Source ValueSource
	// Override is the already-substituted Pipeline fragment. It keeps password
	// values in clear because it is what gets sent to the agent; use
	// MaskedOverride for anything that may be displayed.
	Override Pipeline
	// secrets holds the password literals resolved before this contribution so
	// MaskedOverride can hide them without touching Override.
	secrets []string
}

// MaskedOverride returns a copy of the contribution with every password value
// replaced by ******, safe for --explain. Override itself stays untouched.
func (c Contribution) MaskedOverride() Pipeline {
	return maskedPipeline(c.Override, c.secrets)
}

// Skipped records an option that was referenced but does not apply to the
// current controller/resource.
type Skipped struct {
	Name   string
	Layer  Layer
	Reason string
}

// Resolution is the outcome of evaluating every active option.
type Resolution struct {
	// Selections lists resolved options in evaluation order.
	Selections []Selection
	// Contributions lists the Pipeline fragments in merge order.
	Contributions []Contribution
	// Skipped lists referenced options that did not apply.
	Skipped []Skipped
	// Override is the merged result of every contribution. It keeps password
	// values in clear because it is what gets sent to the agent; use
	// MaskedOverride for anything that may be displayed or serialized.
	Override Pipeline
	// secrets holds every password literal resolved in this run.
	secrets []string
}

// MaskedOverride returns a copy of the merged override with every password
// value replaced by ******, safe for logging and JSON output.
func (r *Resolution) MaskedOverride() Pipeline {
	if r == nil {
		return nil
	}
	return maskedPipeline(r.Override, r.secrets)
}

// Value returns the selected option value for name, in OptionValue shape.
func (r *Resolution) Value(name string) (any, bool) {
	for _, selection := range r.Selections {
		if selection.Name == name {
			return selection.Value(), true
		}
	}
	return nil, false
}

// Values returns every resolved option value keyed by option name, in the shape
// the protocol uses for preset option maps and pretask arguments.
func (r *Resolution) Values() map[string]any {
	out := make(map[string]any, len(r.Selections))
	for _, selection := range r.Selections {
		out[selection.Name] = selection.Value()
	}
	return out
}

// Resolve evaluates global_option, resource.option, controller.option,
// task.option, the task's own pipeline_override, and the caller's extra
// override, in the protocol's order, producing both the audit trail
// (Selections/Contributions) and the final Pipeline override.
func (l *Loaded) Resolve(req Request) (*Resolution, error) {
	r := &resolver{
		project:   l,
		req:       req,
		override:  Pipeline{},
		lookupEnv: req.LookupEnv,
	}
	if r.lookupEnv == nil {
		r.lookupEnv = os.LookupEnv
	}
	if err := r.resolve(); err != nil {
		return nil, err
	}
	return &Resolution{
		Selections:    r.selections,
		Contributions: r.contributions,
		Skipped:       r.skipped,
		Override:      r.override,
		secrets:       r.secrets,
	}, nil
}

// ResolveOption evaluates a single option with the same layers and
// applicability rules as Resolve, without requiring it to be referenced by a
// layer. It is what pretask arguments use. It returns nil when the option does
// not apply to the selected controller/resource.
func (l *Loaded) ResolveOption(name string, req Request) (any, error) {
	r := &resolver{
		project:   l,
		req:       req,
		override:  Pipeline{},
		lookupEnv: req.LookupEnv,
	}
	if r.lookupEnv == nil {
		r.lookupEnv = os.LookupEnv
	}
	r.layers = r.valueLayers()
	if err := r.resolveOption(name, "standalone", nil, "", 0); err != nil {
		return nil, err
	}
	for _, selection := range r.selections {
		if selection.Name == name {
			return selection.Value(), nil
		}
	}
	return nil, nil
}

// resolver carries the mutable state of one Resolve call.
type resolver struct {
	project       *Loaded
	req           Request
	layers        []valueLayer
	selections    []Selection
	contributions []Contribution
	skipped       []Skipped
	override      Pipeline
	// secrets collects every password value resolved so far so the display
	// copies of the override can mask them.
	secrets   []string
	lookupEnv func(string) (string, bool)
}

// valueLayer is one source of option values.
type valueLayer struct {
	source ValueSource
	values map[string]any
}

// layerOptions lists the option names contributed by one PI layer.
type layerOptions struct {
	layer Layer
	names []string
}

func (r *resolver) resolve() error {
	r.layers = r.valueLayers()

	// Protocol merge order: global_option → resource.option → controller.option
	// → task.option.
	var layers []layerOptions
	layers = append(layers, layerOptions{LayerGlobal, r.project.GlobalOption})
	if resource := r.selectedResource(); resource != nil {
		layers = append(layers, layerOptions{LayerResource, resource.Option})
	}
	if controller := r.selectedController(); controller != nil {
		layers = append(layers, layerOptions{LayerController, controller.Option})
	}
	if r.req.Task != nil {
		layers = append(layers, layerOptions{LayerTask, r.req.Task.Option})
	}
	for _, layer := range layers {
		for _, name := range layer.names {
			if err := r.resolveOption(name, layer.layer, nil, "", 0); err != nil {
				return err
			}
		}
	}

	if r.req.Task != nil && r.req.Task.Override != nil {
		fragment, err := toPipeline(r.req.Task.Override, "task.pipeline_override")
		if err != nil {
			return err
		}
		r.addContribution("task.pipeline_override", SourceDefault, fragment)
	}
	if len(r.req.ExtraOverride) > 0 {
		r.addContribution("override", SourceCLI, r.req.ExtraOverride)
	}
	return nil
}

// valueLayers builds the option value sources from lowest to highest priority.
func (r *resolver) valueLayers() []valueLayer {
	layers := []valueLayer{}
	add := func(source ValueSource, values map[string]any) {
		if len(values) > 0 {
			layers = append(layers, valueLayer{source: source, values: values})
		}
	}
	add(SourceConfig, r.req.ConfigGlobal)
	add(SourceConfig, r.req.ConfigTask)
	add(SourcePreset, r.req.PresetOptions)
	add(SourceOptionFile, r.req.OptionFile)
	add(SourceCLI, r.req.CLI)
	return layers
}

// selectedController returns the PI controller the request refers to.
func (r *resolver) selectedController() *Controller {
	if r.req.ControllerName == "" {
		return nil
	}
	for i := range r.project.Controller {
		if r.project.Controller[i].Name == r.req.ControllerName {
			return &r.project.Controller[i]
		}
	}
	return nil
}

// selectedResource returns the PI resource the request refers to.
func (r *resolver) selectedResource() *Resource {
	if r.req.ResourceName == "" {
		return nil
	}
	for i := range r.project.Resource {
		if r.project.Resource[i].Name == r.req.ResourceName {
			return &r.project.Resource[i]
		}
	}
	return nil
}

// resolveOption evaluates one option and, when selected, its nested options.
func (r *resolver) resolveOption(name string, layer Layer, parent *optionPath, parentName string, depth int) error {
	option, ok := r.project.Option.Get(name)
	if !ok {
		return fmt.Errorf("%s references option %q, which is not defined in `option`", layer, name)
	}
	if !OptionActive(option, r.req.ControllerName, r.req.ResourceName) {
		r.skipped = append(r.skipped, Skipped{Name: name, Layer: layer, Reason: applicabilityReason(option, r.req.ControllerName, r.req.ResourceName)})
		return nil
	}
	if parent != nil && parent.has(name) {
		return fmt.Errorf("cyclic option reference: %s", parent.chain(name))
	}
	next := &optionPath{parent: parent, name: name}

	value, source, provided := r.valueFor(name)
	selection := Selection{Name: name, Layer: layer, Source: source, Parent: parentName, Depth: depth}
	if !provided {
		selection.Source = SourceDefault
	}
	label := fmt.Sprintf("%s %s", layer, name)

	switch option.Kind() {
	case OptionTypeSelect, OptionTypeSwitch:
		caseName, err := r.pickCase(option, value, provided)
		if err != nil {
			return err
		}
		selection.Cases = []string{caseName}
		r.selections = append(r.selections, selection)
		chosen, _ := option.FindCase(caseName)
		return r.applyCase(option, chosen, layer, next, depth, label+"="+caseName)

	case OptionTypeCheckbox:
		cases, err := optionCases(option, value, provided)
		if err != nil {
			return err
		}
		if err := validateCount(option, cases); err != nil {
			return err
		}
		selection.Cases = cases
		r.selections = append(r.selections, selection)
		for _, caseName := range cases {
			chosen, _ := option.FindCase(caseName)
			if err := r.applyCase(option, chosen, layer, next, depth, label+"="+caseName); err != nil {
				return err
			}
		}
		return nil

	case OptionTypeInput:
		placeholders, values, secrets, err := r.inputValues(option, value, provided, source)
		if err != nil {
			return err
		}
		selection.Inputs = values
		selection.Secrets = secrets
		r.selections = append(r.selections, selection)
		return r.applyTemplate(option, placeholders, label)

	case OptionTypeHotkey:
		values, err := r.hotkeyValues(option, value, provided)
		if err != nil {
			return err
		}
		selection.Inputs = values
		r.selections = append(r.selections, selection)
		return r.applyHotkey(option, values, label)

	default:
		return fmt.Errorf("option %q has unsupported type %q", name, option.Type)
	}
}

// optionPath tracks the active option chain to detect cyclic references.
type optionPath struct {
	parent *optionPath
	name   string
}

func (p *optionPath) has(name string) bool {
	for current := p; current != nil; current = current.parent {
		if current.name == name {
			return true
		}
	}
	return false
}

func (p *optionPath) chain(name string) string {
	names := []string{name}
	for current := p; current != nil; current = current.parent {
		names = append(names, current.name)
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return Join(names)
}

// applyCase merges one selected case's override and then its nested options.
func (r *resolver) applyCase(option *Option, chosen *OptionCase, layer Layer, path *optionPath, depth int, label string) error {
	if chosen == nil {
		return nil
	}
	if chosen.Override != nil {
		fragment, err := toPipeline(chosen.Override, fmt.Sprintf("option %q case %q pipeline_override", option.Name, chosen.Name))
		if err != nil {
			return err
		}
		r.addContribution(label, r.sourceFor(option.Name), fragment)
	}
	for _, child := range chosen.Option {
		if err := r.resolveOption(child, layer, path, option.Name, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// applyTemplate substitutes an input option's template with its typed values.
func (r *resolver) applyTemplate(option *Option, values map[string]placeholder, label string) error {
	if option.Override == nil {
		return nil
	}
	fragment, err := toPipeline(option.Override, fmt.Sprintf("option %q pipeline_override", option.Name))
	if err != nil {
		return err
	}
	substituted, err := substituteTemplates(fragment, values)
	if err != nil {
		return err
	}
	result, ok := substituted.(map[string]any)
	if !ok {
		return fmt.Errorf("option %q pipeline_override: expected an object", option.Name)
	}
	r.addContribution(label, r.sourceFor(option.Name), result)
	return nil
}

// applyHotkey substitutes a hotkey option's template with virtual key codes.
func (r *resolver) applyHotkey(option *Option, values map[string]string, label string) error {
	if option.Override == nil {
		return nil
	}
	placeholders := map[string]placeholder{}
	for _, field := range option.Hotkeys {
		raw, ok := values[field.Name]
		if !ok {
			continue
		}
		codes, text, err := r.hotkeyCodes(raw)
		if err != nil {
			return fmt.Errorf("option %q field %q: %w", option.Name, field.Name, err)
		}
		placeholders[field.Name] = placeholder{text: text, typed: codes.Primary}
		placeholders[field.Name+".primary"] = placeholder{text: text, typed: codes.Primary}
		for i, code := range codes.Modifiers {
			name := fmt.Sprintf("%s.modifier%d", field.Name, i+1)
			placeholders[name] = placeholder{text: text, typed: code}
		}
	}
	fragment, err := toPipeline(option.Override, fmt.Sprintf("option %q pipeline_override", option.Name))
	if err != nil {
		return err
	}
	substituted, err := substituteTemplates(fragment, placeholders)
	if err != nil {
		return err
	}
	result, ok := substituted.(map[string]any)
	if !ok {
		return fmt.Errorf("option %q pipeline_override: expected an object", option.Name)
	}
	r.addContribution(label, r.sourceFor(option.Name), result)
	return nil
}

// hotkeyCodes maps a hotkey string for the selected controller and returns the
// display text used for non-whole-string placeholders.
func (r *resolver) hotkeyCodes(value string) (KeyCodes, string, error) {
	parsed, err := ParseHotkey(value)
	if err != nil {
		return KeyCodes{}, "", err
	}
	controllerType := ""
	if controller := r.selectedController(); controller != nil {
		controllerType = controller.Type
	}
	if controllerType == "" {
		return KeyCodes{}, "", fmt.Errorf("cannot map hotkey %q without a selected controller", value)
	}
	codes, err := parsed.KeyCodes(controllerType)
	if err != nil {
		return KeyCodes{}, "", err
	}
	return codes, parsed.String(), nil
}

// pickCase resolves a select/switch value against the option's cases.
func (r *resolver) pickCase(option *Option, value any, provided bool) (string, error) {
	name := ""
	if provided {
		text, err := coerceString(value)
		if err != nil {
			return "", fmt.Errorf("option %q: %w", option.Name, err)
		}
		name = text
	} else {
		name = option.DefaultCase.String()
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("option %q has no default_case; provide --option %s=<case>; cases: %s", option.Name, option.Name, Join(option.CaseNames()))
	}
	if _, ok := option.FindCase(name); ok {
		return name, nil
	}
	if option.Kind() == OptionTypeSwitch {
		if wanted, ok := normalizeSwitch(name); ok {
			for _, candidate := range option.Cases {
				if got, ok := normalizeSwitch(candidate.Name); ok && got == wanted {
					return candidate.Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("option %q: %q is not a case; cases: %s", option.Name, name, Join(option.CaseNames()))
}

// optionCases resolves a checkbox value into case names in definition order.
func optionCases(option *Option, value any, provided bool) ([]string, error) {
	var requested []string
	if provided {
		list, err := coerceStringSlice(value)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", option.Name, err)
		}
		requested = list
	} else {
		requested = option.DefaultCase.Slice()
	}
	selected := map[string]bool{}
	for _, name := range requested {
		if selected[name] {
			continue
		}
		if _, ok := option.FindCase(name); !ok {
			return nil, fmt.Errorf("option %q: %q is not a case; cases: %s", option.Name, name, Join(option.CaseNames()))
		}
		selected[name] = true
	}
	// Protocol: selected cases merge in `cases` declaration order, never in the
	// order the caller listed them.
	ordered := make([]string, 0, len(selected))
	for _, candidate := range option.Cases {
		if selected[candidate.Name] {
			ordered = append(ordered, candidate.Name)
		}
	}
	return ordered, nil
}

// validateCount enforces checkbox min_count / max_count.
func validateCount(option *Option, selected []string) error {
	if min := option.MinCount; min != nil && len(selected) < *min {
		return fmt.Errorf("option %q requires at least %d selected case(s), got %d", option.Name, *min, len(selected))
	}
	if max := option.MaxCount; max != nil && len(selected) > *max {
		return fmt.Errorf("option %q allows at most %d selected case(s), got %d", option.Name, *max, len(selected))
	}
	return nil
}

// inputValues resolves an input option's fields. It returns the typed
// placeholder values, the plain field values, and the set of password fields.
func (r *resolver) inputValues(option *Option, value any, provided bool, source ValueSource) (map[string]placeholder, map[string]string, map[string]bool, error) {
	raw := map[string]any{}
	if provided {
		object, err := coerceStringMap(value)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("option %q: %w", option.Name, err)
		}
		raw = object
	}
	placeholders := map[string]placeholder{}
	values := map[string]string{}
	secrets := map[string]bool{}
	for _, field := range option.Inputs {
		var fieldValue string
		if rawValue, ok := raw[field.Name]; ok {
			resolved, err := r.resolveFieldValue(option, field, rawValue, source)
			if err != nil {
				return nil, nil, nil, err
			}
			fieldValue = resolved
		} else {
			switch {
			case field.Password:
				return nil, nil, nil, fmt.Errorf("option %q: password field %q must be provided via --option-file, the client config, or an {\"env\":\"NAME\"} reference", option.Name, field.Name)
			case field.Default != "":
				fieldValue = field.Default
			default:
				return nil, nil, nil, fmt.Errorf("option %q: input field %q has no default; provide --option %s.%s=<value>", option.Name, field.Name, option.Name, field.Name)
			}
		}
		if err := verifyField(option, field, fieldValue); err != nil {
			return nil, nil, nil, err
		}
		typed, err := convertInput(fieldValue, field.PipelineType)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("option %q field %q: %w", option.Name, field.Name, err)
		}
		placeholders[field.Name] = placeholder{text: fieldValue, typed: typed}
		values[field.Name] = fieldValue
		if field.Password {
			secrets[field.Name] = true
			r.addSecret(fieldValue)
		}
	}
	return placeholders, values, secrets, nil
}

// addSecret records one password literal so MaskedOverride can hide it. Empty
// values are ignored: they would make every string match.
func (r *resolver) addSecret(value string) {
	if value == "" {
		return
	}
	for _, existing := range r.secrets {
		if existing == value {
			return
		}
	}
	r.secrets = append(r.secrets, value)
}

// resolveFieldValue turns a raw field value into the string used in the
// Pipeline, rejecting plaintext passwords supplied on the command line.
func (r *resolver) resolveFieldValue(option *Option, field OptionInput, raw any, source ValueSource) (string, error) {
	if reference, ok := envReference(raw); ok {
		value, found := r.lookupEnv(reference)
		if !found {
			return "", fmt.Errorf("option %q field %q: environment variable %s is not set", option.Name, field.Name, reference)
		}
		return value, nil
	}
	text, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("option %q field %q: %w", option.Name, field.Name, err)
	}
	if field.Password && source == SourceCLI {
		return "", fmt.Errorf("option %q field %q: passwords cannot be passed with --option; use --option-file, the client config, or an {\"env\":\"NAME\"} reference", option.Name, field.Name)
	}
	return text, nil
}

// hotkeyValues resolves a hotkey option's fields and validates them against the
// selected controller.
func (r *resolver) hotkeyValues(option *Option, value any, provided bool) (map[string]string, error) {
	raw := map[string]any{}
	if provided {
		object, err := coerceStringMap(value)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", option.Name, err)
		}
		raw = object
	}
	values := map[string]string{}
	for _, field := range option.Hotkeys {
		if text, ok := raw[field.Name]; ok {
			coerced, err := coerceString(text)
			if err != nil {
				return nil, fmt.Errorf("option %q field %q: %w", option.Name, field.Name, err)
			}
			values[field.Name] = coerced
			continue
		}
		if field.Default != "" {
			values[field.Name] = field.Default
			continue
		}
		return nil, fmt.Errorf("option %q: hotkey field %q has no default; provide --option %s.%s=<hotkey>", option.Name, field.Name, option.Name, field.Name)
	}
	for _, field := range option.Hotkeys {
		if _, _, err := r.hotkeyCodes(values[field.Name]); err != nil {
			return nil, fmt.Errorf("option %q field %q: %w", option.Name, field.Name, err)
		}
	}
	return values, nil
}

// valueFor returns the value for name from the highest-priority layer that has
// it, plus where it came from.
func (r *resolver) valueFor(name string) (any, ValueSource, bool) {
	for i := len(r.layers) - 1; i >= 0; i-- {
		if value, ok := r.layers[i].values[name]; ok {
			return value, r.layers[i].source, true
		}
	}
	return nil, SourceDefault, false
}

// sourceFor reports the value source currently used for an option.
func (r *resolver) sourceFor(name string) ValueSource {
	if _, source, ok := r.valueFor(name); ok {
		return source
	}
	return SourceDefault
}

// addContribution merges one Pipeline fragment and records it for --explain.
func (r *resolver) addContribution(label string, source ValueSource, fragment Pipeline) {
	if len(fragment) == 0 {
		return
	}
	cloned, _ := CloneJSON(fragment).(map[string]any)
	r.contributions = append(r.contributions, Contribution{Label: label, Source: source, Override: cloned, secrets: r.secrets})
	MergePipeline(r.override, fragment)
}

// applicabilityReason explains why an option was filtered out.
func applicabilityReason(option *Option, controllerName, resourceName string) string {
	var parts []string
	if !Compatible(option.Controller, controllerName) {
		parts = append(parts, fmt.Sprintf("controller must be one of %s", Join(option.Controller)))
	}
	if !Compatible(option.Resource, resourceName) {
		parts = append(parts, fmt.Sprintf("resource must be one of %s", Join(option.Resource)))
	}
	return Join(parts)
}

// normalizeSwitch maps the protocol's Yes/No spellings onto "yes"/"no".
func normalizeSwitch(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "yes", "y":
		return "yes", true
	case "no", "n":
		return "no", true
	default:
		return "", false
	}
}

// toPipeline narrows an override fragment to an object.
func toPipeline(value any, what string) (Pipeline, error) {
	if value == nil {
		return Pipeline{}, nil
	}
	object, ok := asObject(value)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", what)
	}
	return object, nil
}

// coerceString accepts the string forms the config file, preset, option file,
// and command line may produce.
func coerceString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case bool:
		if v {
			return "Yes", nil
		}
		return "No", nil
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%v", v), ".0"), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	default:
		return "", fmt.Errorf("expected a string, got %T", value)
	}
}

// coerceStringSlice accepts a []string, the []any that JSON decoding produces,
// or one comma-separated string.
func coerceStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil
	case string:
		var out []string
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text, err := coerceString(item)
			if err != nil {
				return nil, err
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected a string list, got %T", value)
	}
}

// coerceStringMap accepts a map[string]string or a decoded JSON object.
func coerceStringMap(value any) (map[string]any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out, nil
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected an object of field values, got %T", value)
	}
}

// envReference detects the {"env": "NAME"} password reference.
func envReference(value any) (string, bool) {
	object, ok := asObject(value)
	if !ok {
		return "", false
	}
	name, ok := object["env"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "", false
	}
	return name, true
}

// verifyField applies the input field's `verify` pattern.
func verifyField(option *Option, field OptionInput, value string) error {
	if field.Verify == "" {
		return nil
	}
	pattern, err := regexp.Compile(field.Verify)
	if err != nil {
		return fmt.Errorf("option %q field %q: invalid verify pattern %q: %w", option.Name, field.Name, field.Verify, err)
	}
	if !pattern.MatchString(value) {
		if field.PatternMsg != "" {
			return fmt.Errorf("option %q field %q: %s", option.Name, field.Name, field.PatternMsg)
		}
		return fmt.Errorf("option %q field %q: %q does not match %s", option.Name, field.Name, value, field.Verify)
	}
	return nil
}

// convertInput applies `pipeline_type` to an input field value.
func convertInput(value, pipelineType string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(pipelineType)) {
	case "", "string":
		return value, nil
	case "int":
		return jsonNumber(value), nil
	case "bool":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "yes", "y", "1":
			return true, nil
		case "false", "no", "n", "0":
			return false, nil
		default:
			return nil, fmt.Errorf("expected a boolean, got %q", value)
		}
	default:
		return nil, fmt.Errorf("unsupported pipeline_type %q", pipelineType)
	}
}
