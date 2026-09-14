package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"maactl/internal/output"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/adb"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/win32"
)

// Controller types this build can create. MaaFramework also defines MacOS,
// PlayCover, and Linux controllers, but they only run on their own platforms and
// maactl ships for Windows, so those fail fast with an explicit message instead
// of an opaque native error.
const (
	controllerTypeAdb     = "Adb"
	controllerTypeWin32   = "Win32"
	controllerTypeGamepad = "Gamepad"
)

// win32ScreencapAll enables every Win32 screencap method. MaaFramework tests
// all provided methods and uses the fastest available one, so enabling them all
// is the most forgiving default when the PI controller names none. Input
// methods, in contrast, cannot be combined: one has to be picked.
const win32ScreencapAll = win32.ScreencapAll

// win32DefaultInput is the mouse and keyboard method used when neither the PI
// controller nor the command line selects one. Seize is the most compatible
// Win32 method and needs no administrator rights.
const win32DefaultInput = win32.InputSeize

// createController builds the MaaFramework controller described by a PI
// controller entry, using the run options for anything the PI leaves open.
func createController(spec *pi.Controller, opt runOptions) (*maa.Controller, error) {
	switch {
	case strings.EqualFold(spec.Type, controllerTypeAdb):
		return createAdbController(spec, opt)
	case strings.EqualFold(spec.Type, controllerTypeWin32):
		return createWin32Controller(spec, opt)
	case strings.EqualFold(spec.Type, controllerTypeGamepad):
		return createGamepadController(spec, opt)
	default:
		return nil, fmt.Errorf("controller %q has type %q; this build supports %s, %s and %s", spec.Name, spec.Type, controllerTypeAdb, controllerTypeWin32, controllerTypeGamepad)
	}
}

func createAdbController(spec *pi.Controller, opt runOptions) (*maa.Controller, error) {
	// Every ADB connection detail comes from MaaToolkit, so the controller is
	// built from information MaaFramework already discovered and validated. An
	// empty ADB path in particular makes MaaAdbControllerCreate fail, which is
	// why the address alone is never enough.
	device, err := resolveAdbDevice(opt.adbAddress, opt.adbName)
	if err != nil {
		return nil, err
	}
	// MaaToolkit recommends per-device screencap and input methods; the PI
	// controller overrides them when it declares its own.
	sc := device.ScreencapMethod
	if sc == adb.ScreencapNone {
		sc = adb.ScreencapDefault
	}
	if spec.Adb.Screencap != "" {
		sc, err = adb.ParseScreencapMethod(spec.Adb.Screencap)
		if err != nil {
			return nil, err
		}
	}
	in := device.InputMethod
	if in == adb.InputNone {
		in = adb.InputDefault
	}
	if spec.Adb.Input != "" {
		in, err = adb.ParseInputMethod(spec.Adb.Input)
		if err != nil {
			return nil, err
		}
	}
	ctrl, err := maa.NewAdbController(device.AdbPath, device.Address, sc, in, device.Config, "")
	if err != nil {
		return nil, fmt.Errorf("create ADB controller for %s (adb %s): %w", device.Address, device.AdbPath, err)
	}
	return ctrl, nil
}

func createWin32Controller(spec *pi.Controller, opt runOptions) (*maa.Controller, error) {
	window, err := resolveWin32Window(spec, opt)
	if err != nil {
		return nil, err
	}
	screencap, err := win32ScreencapMethod(opt.win32Screencap, spec.Win32.Screencap)
	if err != nil {
		return nil, err
	}
	mouse, err := win32InputMethod(opt.win32Mouse, spec.Win32.Mouse, "mouse")
	if err != nil {
		return nil, err
	}
	keyboard, err := win32InputMethod(opt.win32Keyboard, spec.Win32.Keyboard, "keyboard")
	if err != nil {
		return nil, err
	}
	ctrl, err := maa.NewWin32Controller(window.Handle, screencap, mouse, keyboard)
	if err != nil {
		return nil, fmt.Errorf("create Win32 controller for %s: %w", describeWin32Window(window), err)
	}
	return ctrl, nil
}

// resolveWin32Window picks the desktop window a Win32 controller drives. The
// command line always wins; when it names no window the PI controller regexes
// apply, and with neither a single detected window is used automatically.
func resolveWin32Window(spec *pi.Controller, opt runOptions) (*maa.DesktopWindow, error) {
	return resolveWindow(windowSelector(opt, spec.Win32.ClassRegex, spec.Win32.WindowRegex))
}

// windowSelector merges the command-line window selection with a PI controller's
// window regexes. Any command-line selector wins as a group, so a stale PI regex
// never narrows an explicit choice.
func windowSelector(opt runOptions, classRegex, windowRegex string) win32Selector {
	selector := win32Selector{handle: opt.win32Handle, class: opt.win32Class, window: opt.win32Window}
	if selector.empty() {
		selector.class = classRegex
		selector.window = windowRegex
	}
	return selector
}

// resolveWindow returns the single desktop window accepted by selector.
func resolveWindow(selector win32Selector) (*maa.DesktopWindow, error) {
	windows, err := desktopWindows()
	if err != nil {
		return nil, fmt.Errorf("list desktop windows: %w", err)
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("no desktop windows found; run \"maactl win32 devices\" to list them")
	}
	if selector.empty() {
		if len(windows) > 1 {
			return nil, fmt.Errorf("%d desktop windows found; specify --win32-handle/--win32-class/--win32-window or PI win32.class_regex/win32.window_regex", len(windows))
		}
		return windows[0].window, nil
	}
	matched, err := matchWin32Windows(windows, selector)
	if err != nil {
		return nil, err
	}
	switch len(matched) {
	case 0:
		return nil, fmt.Errorf("no desktop window matches %s; detected: %s", selector.describe(), describeWin32Windows(windows))
	case 1:
		return matched[0].window, nil
	default:
		return nil, fmt.Errorf("%d desktop windows match %s: %s", len(matched), selector.describe(), describeWin32Windows(matched))
	}
}

// createGamepadController creates a virtual gamepad. A window is optional: with
// one the controller also captures it, without one it only drives the gamepad.
func createGamepadController(spec *pi.Controller, opt runOptions) (*maa.Controller, error) {
	gamepadType, err := parseGamepadType(firstNonEmpty(opt.gamepadType, spec.Gamepad.GamepadType))
	if err != nil {
		return nil, err
	}
	selector := windowSelector(opt, spec.Gamepad.ClassRegex, spec.Gamepad.WindowRegex)
	if selector.empty() {
		ctrl, err := maa.NewGamepadController(nil, gamepadType, win32.ScreencapNone)
		if err != nil {
			return nil, fmt.Errorf("create %s Gamepad controller: %w", gamepadTypeName(gamepadType), err)
		}
		return ctrl, nil
	}
	window, err := resolveWindow(selector)
	if err != nil {
		return nil, err
	}
	screencap, err := win32ScreencapMethod(opt.win32Screencap, spec.Gamepad.Screencap)
	if err != nil {
		return nil, err
	}
	ctrl, err := maa.NewGamepadController(window.Handle, gamepadType, screencap)
	if err != nil {
		return nil, fmt.Errorf("create %s Gamepad controller for %s: %w", gamepadTypeName(gamepadType), describeWin32Window(window), err)
	}
	return ctrl, nil
}

// parseGamepadType accepts the PI gamepad_type names; an empty value defaults to
// Xbox360, matching the ProjectInterface protocol.
func parseGamepadType(value string) (maa.GamepadType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "xbox360":
		return maa.GamepadTypeXbox360, nil
	case "dualshock4", "ds4":
		return maa.GamepadTypeDualShock4, nil
	default:
		return maa.GamepadTypeXbox360, fmt.Errorf("invalid gamepad type %q; use Xbox360 or DualShock4", value)
	}
}

func gamepadTypeName(gamepadType maa.GamepadType) string {
	if gamepadType == maa.GamepadTypeDualShock4 {
		return "DualShock4"
	}
	return "Xbox360"
}

// win32Window is a desktop window in a form that is easy to filter and test
// without touching the native handle. window keeps the original so the matched
// entry can be passed straight to MaaFramework.
type win32Window struct {
	handle     uintptr
	className  string
	windowName string
	window     *maa.DesktopWindow
}

// win32Selector holds the filters used to pick one desktop window. Empty fields
// impose no restriction.
type win32Selector struct {
	handle string
	class  string
	window string
}

func (s win32Selector) empty() bool {
	return s.handle == "" && s.class == "" && s.window == ""
}

// describe labels a selector in error messages.
func (s win32Selector) describe() string {
	var parts []string
	if s.handle != "" {
		parts = append(parts, "--win32-handle "+s.handle)
	}
	if s.class != "" {
		parts = append(parts, "--win32-class "+s.class)
	}
	if s.window != "" {
		parts = append(parts, "--win32-window "+s.window)
	}
	return pi.Join(parts)
}

// desktopWindows lists the MaaToolkit desktop windows in the filter-friendly
// form used by resolveWin32Window.
func desktopWindows() ([]win32Window, error) {
	found, err := maa.FindDesktopWindows()
	if err != nil {
		return nil, err
	}
	windows := make([]win32Window, 0, len(found))
	for _, window := range found {
		windows = append(windows, win32Window{
			handle:     uintptr(window.Handle),
			className:  window.ClassName,
			windowName: window.WindowName,
			window:     window,
		})
	}
	return windows, nil
}

// matchWin32Windows keeps the windows accepted by every configured filter.
func matchWin32Windows(windows []win32Window, selector win32Selector) ([]win32Window, error) {
	handle, hasHandle, err := parseWin32Handle(selector.handle)
	if err != nil {
		return nil, err
	}
	class, err := compileWin32Regex(selector.class, "--win32-class")
	if err != nil {
		return nil, err
	}
	title, err := compileWin32Regex(selector.window, "--win32-window")
	if err != nil {
		return nil, err
	}
	var matched []win32Window
	for _, window := range windows {
		if hasHandle && window.handle != handle {
			continue
		}
		if class != nil && !class.MatchString(window.className) {
			continue
		}
		if title != nil && !title.MatchString(window.windowName) {
			continue
		}
		matched = append(matched, window)
	}
	return matched, nil
}

// parseWin32Handle accepts a window handle as decimal or 0x-prefixed hex. The
// boolean reports whether a handle was supplied at all.
func parseWin32Handle(value string) (uintptr, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}
	base, digits := 10, value
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		base, digits = 16, value[2:]
	}
	parsed, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid --win32-handle %q: expected a decimal or 0x-prefixed hexadecimal window handle", value)
	}
	return uintptr(parsed), true, nil
}

func compileWin32Regex(pattern, flag string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", flag, pattern, err)
	}
	return re, nil
}

// win32ScreencapMethod resolves the screencap method, preferring the command
// line over PI and falling back to every method.
func win32ScreencapMethod(cliValue, piValue string) (win32.ScreencapMethod, error) {
	value := firstNonEmpty(cliValue, piValue)
	if value == "" {
		return win32ScreencapAll, nil
	}
	method, err := win32.ParseScreencapMethod(value)
	if err != nil {
		return 0, fmt.Errorf("win32 screencap: %w", err)
	}
	return method, nil
}

// win32InputMethod resolves one mouse or keyboard method, preferring the command
// line over PI and falling back to Seize. Win32 input methods cannot be combined.
func win32InputMethod(cliValue, piValue, kind string) (win32.InputMethod, error) {
	value := firstNonEmpty(cliValue, piValue)
	if value == "" {
		return win32DefaultInput, nil
	}
	method, err := win32.ParseInputMethod(value)
	if err != nil {
		return 0, fmt.Errorf("win32 %s: %w", kind, err)
	}
	return method, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// describeWin32Windows renders windows for error messages, capped so a filter
// that matches the whole desktop does not flood the terminal.
func describeWin32Windows(windows []win32Window) string {
	const limit = 8
	descriptions := make([]string, 0, limit+1)
	for i, window := range windows {
		if i == limit {
			descriptions = append(descriptions, fmt.Sprintf("... (+%d more)", len(windows)-limit))
			break
		}
		descriptions = append(descriptions, describeWin32Window(window.window))
	}
	return pi.Join(descriptions)
}

// describeWin32Window renders one window the way "maactl win32 devices" does.
func describeWin32Window(window *maa.DesktopWindow) string {
	return fmt.Sprintf("%s (class %s, handle %s)", output.Value(window.WindowName), output.Value(window.ClassName), win32HandleString(window.Handle))
}

// resolveAdbDevice picks the ADB device to connect to from the devices
// MaaToolkit discovered. --adb-address matches the device address and --name
// matches the device name; with neither, the only detected device is used.
// The returned device carries the ADB path and config needed to create a
// controller, so callers must pass those to MaaFramework instead of rebuilding
// them from the address.
func resolveAdbDevice(address, name string) (*maa.AdbDevice, error) {
	devices, err := maa.FindAdbDevices()
	if err != nil {
		return nil, fmt.Errorf("find ADB devices: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no ADB devices found; connect a device and check \"maactl adb devices\"")
	}
	if address == "" && name == "" {
		if len(devices) > 1 {
			return nil, fmt.Errorf("%d ADB devices found; specify --adb-address/-a or --name: %s", len(devices), describeAdbDevices(devices))
		}
		return devices[0], nil
	}
	var matched []*maa.AdbDevice
	for _, device := range devices {
		if address != "" && !strings.EqualFold(device.Address, address) {
			continue
		}
		if name != "" && !strings.EqualFold(device.Name, name) {
			continue
		}
		matched = append(matched, device)
	}
	switch len(matched) {
	case 0:
		return nil, fmt.Errorf("no ADB device matches %s; detected: %s", adbSelector(address, name), describeAdbDevices(devices))
	case 1:
		return matched[0], nil
	default:
		return nil, fmt.Errorf("%d ADB devices match %s: %s", len(matched), adbSelector(address, name), describeAdbDevices(matched))
	}
}

// describeAdbDevices renders discovered devices for error messages, including
// both the address and the name needed for --adb-address/--name.
func describeAdbDevices(devices []*maa.AdbDevice) string {
	descriptions := make([]string, len(devices))
	for i, device := range devices {
		descriptions[i] = fmt.Sprintf("%s (%s)", device.Address, device.Name)
	}
	return pi.Join(descriptions)
}

// adbSelector labels the --adb-address/--name filter in error messages.
func adbSelector(address, name string) string {
	var parts []string
	if address != "" {
		parts = append(parts, fmt.Sprintf("--adb-address %s", address))
	}
	if name != "" {
		parts = append(parts, fmt.Sprintf("--name %s", name))
	}
	return pi.Join(parts)
}
