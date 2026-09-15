package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"maactl/internal/output"
	"maactl/internal/pi"
	"maactl/internal/platform"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
)

// desktopWindow is a window reported by MaaToolkit in a form that is easy to
// filter and test without touching the native handle. window keeps the original
// so the matched entry can be passed straight to MaaFramework.
//
// The handle means different things per platform: a Win32 HWND on Windows, a
// CGWindowID on macOS, and an X11 window id on Linux. It is always the value
// that platform's controller needs.
type desktopWindow struct {
	handle     uintptr
	className  string
	windowName string
	window     *maa.DesktopWindow
}

// windowSelector holds the filters used to pick one desktop window. Empty
// fields impose no restriction.
type windowSelector struct {
	handle string
	class  string
	window string
	// labels names the flags this selector was filled from, so errors point at
	// the flags the caller actually has. The zero value means Win32.
	labels windowLabels
}

// windowLabels names the window selection flags of one platform.
type windowLabels struct {
	handle string
	class  string
	title  string
}

var (
	win32Labels = windowLabels{handle: "--win32-handle", class: "--win32-class", title: "--win32-window"}
	macosLabels = windowLabels{handle: "--macos-window-id", class: "--macos-window", title: "--macos-window"}
)

func (s windowSelector) empty() bool {
	return s.handle == "" && s.class == "" && s.window == ""
}

// flags returns the labels used in error messages.
func (s windowSelector) flags() windowLabels {
	if s.labels == (windowLabels{}) {
		return win32Labels
	}
	return s.labels
}

// describe labels a selector in error messages.
func (s windowSelector) describe() string {
	labels := s.flags()
	var parts []string
	for _, filter := range []struct {
		value string
		flag  string
	}{
		{s.handle, labels.handle},
		{s.class, labels.class},
		{s.window, labels.title},
	} {
		if filter.value != "" {
			parts = append(parts, filter.flag+" "+filter.value)
		}
	}
	return pi.Join(parts)
}

// desktopWindows lists the MaaToolkit desktop windows in the filter-friendly
// form used by resolveWindow.
func desktopWindows() ([]desktopWindow, error) {
	found, err := maa.FindDesktopWindows()
	if err != nil {
		return nil, err
	}
	windows := make([]desktopWindow, 0, len(found))
	for _, window := range found {
		windows = append(windows, desktopWindow{
			handle:     uintptr(window.Handle),
			className:  window.ClassName,
			windowName: window.WindowName,
			window:     window,
		})
	}
	return windows, nil
}

// resolveWindow returns the single desktop window accepted by selector. hint
// names the flags and PI fields a user can pick a window with, so an ambiguous
// desktop reports something actionable.
func resolveWindow(selector windowSelector, hint string) (*desktopWindow, error) {
	windows, err := desktopWindows()
	if err != nil {
		return nil, fmt.Errorf("list desktop windows: %w", err)
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("no desktop windows found; run \"maactl device window\" to list them")
	}
	if selector.empty() {
		if len(windows) > 1 {
			return nil, fmt.Errorf("%d desktop windows found; specify %s", len(windows), hint)
		}
		return &windows[0], nil
	}
	matched, err := matchWindows(windows, selector)
	if err != nil {
		return nil, err
	}
	switch len(matched) {
	case 0:
		return nil, fmt.Errorf("no desktop window matches %s; detected: %s", selector.describe(), describeWindows(windows))
	case 1:
		return &matched[0], nil
	default:
		return nil, fmt.Errorf("%d desktop windows match %s: %s", len(matched), selector.describe(), describeWindows(matched))
	}
}

// matchWindows keeps the windows accepted by every configured filter. The
// class filter matches the window class on Windows and the bundle identifier on
// macOS.
func matchWindows(windows []desktopWindow, selector windowSelector) ([]desktopWindow, error) {
	labels := selector.flags()
	handle, hasHandle, err := parseWindowHandle(selector.handle)
	if err != nil {
		return nil, err
	}
	class, err := compileWindowRegex(selector.class, labels.class)
	if err != nil {
		return nil, err
	}
	title, err := compileWindowRegex(selector.window, labels.title)
	if err != nil {
		return nil, err
	}
	var matched []desktopWindow
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

// parseWindowHandle accepts a window handle as decimal or 0x-prefixed hex. The
// boolean reports whether a handle was supplied at all.
func parseWindowHandle(value string) (uintptr, bool, error) {
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
		return 0, false, fmt.Errorf("invalid window handle %q: expected a decimal or 0x-prefixed hexadecimal window handle", value)
	}
	return uintptr(parsed), true, nil
}

// compileWindowRegex compiles an optional window filter regex.
func compileWindowRegex(pattern, flag string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", flag, pattern, err)
	}
	return re, nil
}

// describeWindows renders windows for error messages, capped so a filter that
// matches the whole desktop does not flood the terminal.
func describeWindows(windows []desktopWindow) string {
	const limit = 8
	descriptions := make([]string, 0, limit+1)
	for i, window := range windows {
		if i == limit {
			descriptions = append(descriptions, fmt.Sprintf("... (+%d more)", len(windows)-limit))
			break
		}
		descriptions = append(descriptions, describeWindow(window))
	}
	return pi.Join(descriptions)
}

// describeWindow renders one window the way "maactl device window" does.
func describeWindow(window desktopWindow) string {
	return fmt.Sprintf("%s (class %s, handle %s)", output.Value(window.windowName), output.Value(window.className), windowHandleString(window.handle))
}

// windowHandleString renders a window handle the way the selection flags accept
// it, so the value shown by "maactl device window" can be passed back verbatim.
// Windows and Linux report the native id in hexadecimal; macOS reports the
// decimal CGWindowID MaaFramework expects.
func windowHandleString(handle uintptr) string {
	if platform.Host().OS == platform.MacOS {
		return strconv.FormatUint(uint64(handle), 10)
	}
	return "0x" + strconv.FormatUint(uint64(handle), 16)
}
