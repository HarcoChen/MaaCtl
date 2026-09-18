package cli

import (
	"fmt"
	"os"
	"strings"

	"maactl/internal/clientconfig"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/adb"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/macos"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/win32"
)

// Controller types this build can create. Which of them can actually be created
// depends on the platform: pi.RunnableControllerTypes lists the ones that make
// sense here, and createController refuses the others before touching
// MaaFramework.
const (
	controllerTypeAdb       = "Adb"
	controllerTypeWin32     = "Win32"
	controllerTypeGamepad   = "Gamepad"
	controllerTypeMacOS     = "MacOS"
	controllerTypePlayCover = "PlayCover"
	controllerTypeLinux     = "Linux"
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

// macosDefaultScreencap and macosDefaultInput mirror MaaPiCli: ScreenCaptureKit
// is the only screencap method, and GlobalEvent the input method that works
// without knowing the target process.
const (
	macosDefaultScreencap = macos.ScreencapScreenCaptureKit
	macosDefaultInput     = macos.InputGlobalEvent
)

// playCoverDefaultUUID is the bundle identifier MaaFramework and the
// ProjectInterface protocol use when a controller names none.
const playCoverDefaultUUID = "maa.playcover"

// createController builds the MaaFramework controller described by a PI
// controller entry, using the run options and client configuration for anything
// the PI leaves open.
func createController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	if !pi.ControllerRunnable(spec) {
		if kind := strings.TrimSpace(spec.Type); kind == "" {
			return nil, fmt.Errorf("controller %q has no type; this build supports %s", spec.Name, pi.Join(pi.RunnableControllerTypes()))
		}
		return nil, fmt.Errorf("controller %q has type %q, which cannot be created on %s; this build supports %s",
			spec.Name, spec.Type, pi.PlatformName(), pi.Join(pi.RunnableControllerTypes()))
	}
	// Protocol: permission_required tells the client that this controller needs
	// elevated rights on the host. Checking the current privilege level would
	// need platform specific code, so the declaration is reported instead of
	// being ignored.
	if spec.PermissionRequired {
		fmt.Fprintf(os.Stderr, "warning: controller %q declares permission_required; run maactl with the privileges that controller needs\n", spec.Name)
	}
	switch {
	case strings.EqualFold(spec.Type, controllerTypeAdb):
		return createAdbController(spec, opt, config)
	case strings.EqualFold(spec.Type, controllerTypeWin32):
		return createWin32Controller(spec, opt, config)
	case strings.EqualFold(spec.Type, controllerTypeGamepad):
		return createGamepadController(spec, opt, config)
	case strings.EqualFold(spec.Type, controllerTypeMacOS):
		return createMacOSController(spec, opt, config)
	case strings.EqualFold(spec.Type, controllerTypePlayCover):
		return createPlayCoverController(spec, opt, config)
	case strings.EqualFold(spec.Type, controllerTypeLinux):
		return createLinuxController(spec, opt, config)
	default:
		return nil, fmt.Errorf("controller %q has unsupported type %q; this build supports %s",
			spec.Name, spec.Type, pi.Join(pi.RunnableControllerTypes()))
	}
}

// createAdbController builds an ADB controller.
//
// Connection details come from MaaToolkit whenever possible: it discovers the
// ADB path and the per-device screencap/input methods. The client configuration
// and command line then override the address, the ADB path, and the methods.
func createAdbController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	address := firstNonEmpty(opt.adbAddress, config.Adb.Address)
	name := opt.adbName
	adbPath := firstNonEmpty(opt.adbPath, config.Adb.AdbPath)

	device, err := resolveAdbDevice(address, name)
	if err != nil {
		// MaaToolkit did not find a matching device. An explicit adb path plus
		// address is still enough to build a controller, which is what users
		// with an unusual setup (or an emulator MaaToolkit cannot enumerate)
		// need.
		if adbPath != "" && address != "" {
			return createAdbControllerDirect(spec, config, adbPath, address)
		}
		return nil, err
	}

	if adbPath == "" {
		adbPath = device.AdbPath
	}
	// MaaToolkit recommends per-device screencap and input methods; the PI
	// controller, the client config, and the command line override them.
	sc := device.ScreencapMethod
	if sc == adb.ScreencapNone {
		sc = adb.ScreencapDefault
	}
	for _, value := range []string{spec.Adb.Screencap, config.Adb.Screencap} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := adb.ParseScreencapMethod(value)
		if err != nil {
			return nil, err
		}
		sc = parsed
	}
	input := device.InputMethod
	if input == adb.InputNone {
		input = adb.InputDefault
	}
	for _, value := range []string{spec.Adb.Input, config.Adb.Input} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := adb.ParseInputMethod(value)
		if err != nil {
			return nil, err
		}
		input = parsed
	}
	ctrl, err := maa.NewAdbController(adbPath, device.Address, sc, input, device.Config, "")
	if err != nil {
		return nil, fmt.Errorf("create ADB controller for %s (adb %s): %w", device.Address, adbPath, err)
	}
	return ctrl, nil
}

// createAdbControllerDirect builds an ADB controller without MaaToolkit device
// information, using the configured ADB path and address directly.
func createAdbControllerDirect(spec *pi.Controller, config *clientconfig.Config, adbPath, address string) (*maa.Controller, error) {
	sc := adb.ScreencapDefault
	for _, value := range []string{spec.Adb.Screencap, config.Adb.Screencap} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := adb.ParseScreencapMethod(value)
		if err != nil {
			return nil, err
		}
		sc = parsed
	}
	input := adb.InputDefault
	for _, value := range []string{spec.Adb.Input, config.Adb.Input} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parsed, err := adb.ParseInputMethod(value)
		if err != nil {
			return nil, err
		}
		input = parsed
	}
	ctrl, err := maa.NewAdbController(adbPath, address, sc, input, config.Adb.ConfigJSON(), "")
	if err != nil {
		return nil, fmt.Errorf("create ADB controller for %s (adb %s): %w", address, adbPath, err)
	}
	return ctrl, nil
}

// createWin32Controller builds a controller driving a Windows desktop window.
func createWin32Controller(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	window, err := resolveWin32Window(spec, opt, config)
	if err != nil {
		return nil, err
	}
	screencap, err := win32ScreencapMethod(firstNonEmpty(opt.win32Screencap, config.Win32.Screencap), spec.Win32.Screencap)
	if err != nil {
		return nil, err
	}
	mouse, err := win32InputMethod(firstNonEmpty(opt.win32Mouse, config.Win32.Mouse), spec.Win32.Mouse, "mouse")
	if err != nil {
		return nil, err
	}
	keyboard, err := win32InputMethod(firstNonEmpty(opt.win32Keyboard, config.Win32.Keyboard), spec.Win32.Keyboard, "keyboard")
	if err != nil {
		return nil, err
	}
	ctrl, err := maa.NewWin32Controller(window.window.Handle, screencap, mouse, keyboard)
	if err != nil {
		return nil, fmt.Errorf("create Win32 controller for %s: %w", describeWindow(*window), err)
	}
	return ctrl, nil
}

// win32WindowHint names the flags and PI fields that select a Win32 window.
const win32WindowHint = "--win32-handle/--win32-class/--win32-window or the PI win32.class_regex/win32.window_regex"

// resolveWin32Window picks the desktop window a Win32 controller drives. The
// command line always wins; then the client configuration; then the PI
// controller regexes; and with none of them a single detected window is used.
func resolveWin32Window(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*desktopWindow, error) {
	selector := windowSelector{handle: opt.win32Handle, class: opt.win32Class, window: opt.win32Window, labels: win32Labels}
	if selector.empty() {
		selector = windowSelector{handle: config.Win32.Handle, class: config.Win32.ClassRegex, window: config.Win32.WindowRegex, labels: win32Labels}
	}
	if selector.empty() {
		selector = windowSelector{class: spec.Win32.ClassRegex, window: spec.Win32.WindowRegex, labels: win32Labels}
	}
	return resolveWindow(selector, win32WindowHint)
}

// createGamepadController creates a virtual gamepad. A window is optional: with
// one the controller also captures it, without one it only drives the gamepad.
func createGamepadController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	gamepadType, err := parseGamepadType(firstNonEmpty(opt.gamepadType, spec.Gamepad.GamepadType))
	if err != nil {
		return nil, err
	}
	selector := windowSelector{handle: opt.win32Handle, class: opt.win32Class, window: opt.win32Window, labels: win32Labels}
	if selector.empty() {
		selector = windowSelector{handle: config.Win32.Handle, class: config.Win32.ClassRegex, window: config.Win32.WindowRegex, labels: win32Labels}
	}
	if selector.empty() {
		selector = windowSelector{class: spec.Gamepad.ClassRegex, window: spec.Gamepad.WindowRegex, labels: win32Labels}
	}
	if selector.empty() {
		ctrl, err := maa.NewGamepadController(nil, gamepadType, win32.ScreencapNone)
		if err != nil {
			return nil, fmt.Errorf("create %s Gamepad controller: %w", gamepadTypeName(gamepadType), err)
		}
		return ctrl, nil
	}
	window, err := resolveWindow(selector, win32WindowHint)
	if err != nil {
		return nil, err
	}
	screencap, err := win32ScreencapMethod(firstNonEmpty(opt.win32Screencap, config.Win32.Screencap), spec.Gamepad.Screencap)
	if err != nil {
		return nil, err
	}
	ctrl, err := maa.NewGamepadController(window.window.Handle, gamepadType, screencap)
	if err != nil {
		return nil, fmt.Errorf("create %s Gamepad controller for %s: %w", gamepadTypeName(gamepadType), describeWindow(*window), err)
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

// createMacOSController builds a controller driving a native macOS window.
//
// MaaFramework identifies the target by CGWindowID, so the window is resolved
// from an explicit id or a title regex; with neither, the whole desktop (window
// id 0) is used, exactly like MaaPiCli.
func createMacOSController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	windowID, err := resolveMacOSWindowID(spec, opt, config)
	if err != nil {
		return nil, err
	}
	screencap, err := macOSScreencapMethod(firstNonEmpty(opt.macosScreencap, config.MacOS.Screencap), spec.MacOS.Screencap)
	if err != nil {
		return nil, err
	}
	input, err := macOSInputMethod(firstNonEmpty(opt.macosInput, config.MacOS.Input), spec.MacOS.Input)
	if err != nil {
		return nil, err
	}
	ctrl, err := maa.NewMacOSController(windowID, screencap, input)
	if err != nil {
		return nil, fmt.Errorf("create macOS controller for window %d: %w", windowID, err)
	}
	return ctrl, nil
}

// resolveMacOSWindowID picks the CGWindowID a macOS controller drives.
func resolveMacOSWindowID(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (uint32, error) {
	if raw := firstNonEmpty(opt.macosWindowID, formatWindowID(config.MacOS.WindowID)); raw != "" {
		id, _, err := parseWindowHandle(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid macOS window id %q: expected the window id printed by \"maactl device window\"", raw)
		}
		if id > 0xffffffff {
			return 0, fmt.Errorf("invalid macOS window id %q: macOS window ids are 32-bit", raw)
		}
		return uint32(id), nil
	}
	title := firstNonEmpty(opt.macosWindow, config.MacOS.TitleRegex, spec.MacOS.TitleRegex)
	if title == "" {
		return 0, nil
	}
	window, err := resolveWindow(windowSelector{window: title, labels: macosLabels}, "--macos-window/--macos-window-id or the PI macos.title_regex")
	if err != nil {
		return 0, err
	}
	return uint32(window.handle), nil
}

// formatWindowID renders a configured window id, or "" when it is unset (0 is
// the whole desktop, which is the same as having no id at all).
func formatWindowID(id uint64) string {
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("%d", id)
}

// macOSScreencapMethod resolves the macOS screencap method, preferring the
// command line over the PI and falling back to ScreenCaptureKit.
func macOSScreencapMethod(cliValue, piValue string) (macos.ScreencapMethod, error) {
	value := methodName(firstNonEmpty(cliValue, piValue))
	switch value {
	case "":
		return macosDefaultScreencap, nil
	case "screencapturekit":
		return macos.ScreencapScreenCaptureKit, nil
	default:
		return 0, fmt.Errorf("macos screencap: unknown method %q; use ScreenCaptureKit", firstNonEmpty(cliValue, piValue))
	}
}

// macOSInputMethod resolves the macOS input method, preferring the command line
// over the PI and falling back to GlobalEvent.
func macOSInputMethod(cliValue, piValue string) (macos.InputMethod, error) {
	value := methodName(firstNonEmpty(cliValue, piValue))
	switch value {
	case "":
		return macosDefaultInput, nil
	case "globalevent":
		return macos.InputGlobalEvent, nil
	case "posttopid":
		return macos.InputPostToPid, nil
	default:
		return 0, fmt.Errorf("macos input: unknown method %q; use GlobalEvent or PostToPid", firstNonEmpty(cliValue, piValue))
	}
}

// methodName normalizes a method spelling so "ScreenCaptureKit",
// "screen_capture_kit", and "screencapturekit" are the same method.
func methodName(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "_", ""), " ", ""))
}

// createPlayCoverController builds a PlayCover controller, which drives an iOS
// application running under PlayCover through the PlayTools service.
func createPlayCoverController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	address := firstNonEmpty(opt.playcoverAddress, config.PlayCover.Address)
	uuid := firstNonEmpty(opt.playcoverUUID, config.PlayCover.UUID, spec.PlayCover.UUID, playCoverDefaultUUID)
	if address == "" {
		return nil, fmt.Errorf("PlayCover controller %q needs the PlayTools service address; pass --playcover-address or set playcover.address in the client config", spec.Name)
	}
	ctrl, err := maa.NewPlayCoverController(address, uuid)
	if err != nil {
		return nil, fmt.Errorf("create PlayCover controller for %s at %s: %w", uuid, address, err)
	}
	return ctrl, nil
}

// createLinuxController builds a Linux controller for a Wayland compositor.
//
// The Go binding exposes the wlroots controller, which is what drives a
// wlroots compositor: screencap through wlr-screencopy and input through
// virtual-keyboard/wlr-virtual-pointer. The PI `linux` fields that only the
// newer MaaLinuxControllerCreate accepts are rejected explicitly instead of
// being silently ignored.
func createLinuxController(spec *pi.Controller, opt runOptions, config *clientconfig.Config) (*maa.Controller, error) {
	if err := checkLinuxMethods(spec); err != nil {
		return nil, err
	}
	socket, err := linuxSocketPath(opt.linuxSocket, config.Linux.WlrSocketPath)
	if err != nil {
		return nil, err
	}
	useWin32VKCode := opt.linuxVK || spec.Linux.UseWin32VKCode
	ctrl, err := maa.NewWlRootsController(socket, useWin32VKCode)
	if err != nil {
		return nil, fmt.Errorf("create Linux controller on wayland socket %s: %w", socket, err)
	}
	return ctrl, nil
}

// checkLinuxMethods refuses the PI method overrides this build cannot honour.
func checkLinuxMethods(spec *pi.Controller) error {
	for _, field := range []struct {
		path  string
		value string
	}{
		{"linux.screencap", spec.Linux.Screencap},
		{"linux.input", spec.Linux.Input},
	} {
		switch methodName(field.value) {
		case "", "wlr":
		default:
			return fmt.Errorf("controller %q asks for %s %q, but this build only creates the wlroots controller; use Wlr or leave the field empty",
				spec.Name, field.path, field.value)
		}
	}
	return nil
}

// linuxSocketPath resolves the Wayland socket the wlroots controller connects
// to: the command line, then the client configuration (linux.wlr_socket_path,
// as the MaaPiCli ecosystem writes it), then $WAYLAND_DISPLAY, which every
// Wayland session sets.
func linuxSocketPath(flagValue, configValue string) (string, error) {
	if value := firstNonEmpty(flagValue, configValue); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("no Wayland socket to connect to; pass --linux-socket <path>, set linux.wlr_socket_path in the client config, or run inside a Wayland session (WAYLAND_DISPLAY)")
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
		return nil, fmt.Errorf("no ADB devices found; connect a device and check \"maactl device adb\"")
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
