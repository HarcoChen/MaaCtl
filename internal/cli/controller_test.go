package cli

import (
	"strings"
	"testing"

	"maactl/internal/clientconfig"
	"maactl/internal/pi"

	"github.com/MaaXYZ/maa-framework-go/v4/controller/macos"
	"github.com/MaaXYZ/maa-framework-go/v4/controller/win32"
)

func testWindows() []desktopWindow {
	return []desktopWindow{
		{handle: 0x100, className: "UnityWndClass", windowName: "原神"},
		{handle: 0x200, className: "Notepad", windowName: "notes.txt - Notepad"},
		{handle: 0x300, className: "Chrome_WidgetWin_1", windowName: "Notepad clone"},
	}
}

func TestMatchWindows(t *testing.T) {
	cases := []struct {
		name     string
		selector windowSelector
		want     []uintptr
	}{
		{name: "empty keeps all", selector: windowSelector{}, want: []uintptr{0x100, 0x200, 0x300}},
		{name: "class regex", selector: windowSelector{class: "^Notepad$"}, want: []uintptr{0x200}},
		{name: "window regex", selector: windowSelector{window: "Notepad"}, want: []uintptr{0x200, 0x300}},
		{name: "class and window are ANDed", selector: windowSelector{class: "Chrome", window: "Notepad"}, want: []uintptr{0x300}},
		{name: "decimal handle", selector: windowSelector{handle: "256"}, want: []uintptr{0x100}},
		{name: "hex handle", selector: windowSelector{handle: "0x200"}, want: []uintptr{0x200}},
		{name: "handle and class", selector: windowSelector{handle: "0x300", class: "Notepad"}, want: nil},
		{name: "no match", selector: windowSelector{window: "missing"}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := matchWindows(testWindows(), tc.selector)
			if err != nil {
				t.Fatalf("match: %v", err)
			}
			if len(matched) != len(tc.want) {
				t.Fatalf("got %d matches, want %d: %+v", len(matched), len(tc.want), matched)
			}
			for i, want := range tc.want {
				if matched[i].handle != want {
					t.Errorf("match %d handle = %#x, want %#x", i, matched[i].handle, want)
				}
			}
		})
	}
}

func TestMatchWindowsRejectsBadInput(t *testing.T) {
	for name, selector := range map[string]windowSelector{
		"bad handle":  {handle: "nope"},
		"bad class":   {class: "("},
		"bad title":   {window: "["},
		"bad integer": {handle: "0xzz"},
	} {
		if _, err := matchWindows(testWindows(), selector); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseWindowHandle(t *testing.T) {
	handle, ok, err := parseWindowHandle("")
	if ok || err != nil || handle != 0 {
		t.Errorf("empty handle = %#x, %v, %v", handle, ok, err)
	}
	for input, want := range map[string]uintptr{"0x2a": 0x2a, "42": 42, " 0XFF ": 0xff} {
		handle, ok, err := parseWindowHandle(input)
		if err != nil || !ok || handle != want {
			t.Errorf("parse %q = %#x, %v, %v; want %#x", input, handle, ok, err, want)
		}
	}
	if _, _, err := parseWindowHandle("0x"); err == nil {
		t.Error("expected an empty hex handle to fail")
	}
}

func TestWin32ScreencapMethod(t *testing.T) {
	method, err := win32ScreencapMethod("", "")
	if err != nil || method != win32ScreencapAll {
		t.Errorf("default screencap = %v, %v; want all", method, err)
	}
	method, err = win32ScreencapMethod("", "FramePool")
	if err != nil || method != win32.ScreencapFramePool {
		t.Errorf("PI screencap = %v, %v", method, err)
	}
	method, err = win32ScreencapMethod("GDI", "FramePool")
	if err != nil || method != win32.ScreencapGDI {
		t.Errorf("CLI override = %v, %v; want GDI", method, err)
	}
	if _, err := win32ScreencapMethod("nope", ""); err == nil {
		t.Error("expected an invalid screencap method to fail")
	}
}

func TestWin32InputMethod(t *testing.T) {
	for _, kind := range []string{"mouse", "keyboard"} {
		method, err := win32InputMethod("", "", kind)
		if err != nil || method != win32DefaultInput {
			t.Errorf("%s default = %v, %v; want Seize", kind, method, err)
		}
	}
	method, err := win32InputMethod("", "SendMessage", "mouse")
	if err != nil || method != win32.InputSendMessage {
		t.Errorf("PI mouse = %v, %v", method, err)
	}
	method, err = win32InputMethod("PostMessage", "SendMessage", "mouse")
	if err != nil || method != win32.InputPostMessage {
		t.Errorf("CLI override = %v, %v; want PostMessage", method, err)
	}
	if _, err := win32InputMethod("", "nope", "keyboard"); err == nil {
		t.Error("expected an invalid input method to fail")
	}
}

func TestParseGamepadType(t *testing.T) {
	cases := []struct {
		value string
		want  uint64
	}{
		{"", 0},
		{"Xbox360", 0},
		{"xbox360", 0},
		{"DualShock4", 1},
		{"ds4", 1},
	}
	for _, tc := range cases {
		got, err := parseGamepadType(tc.value)
		if err != nil {
			t.Errorf("parseGamepadType(%q): %v", tc.value, err)
			continue
		}
		if uint64(got) != tc.want {
			t.Errorf("parseGamepadType(%q) = %d, want %d", tc.value, uint64(got), tc.want)
		}
	}
	if _, err := parseGamepadType("nope"); err == nil {
		t.Error("expected an invalid gamepad type to fail")
	}
}

func TestCreateControllerRejectsUnsupportedType(t *testing.T) {
	for _, spec := range []*pi.Controller{
		{Name: "empty"},
		{Name: "unknown", Type: "Nope"},
	} {
		_, err := createController(spec, runOptions{}, &clientconfig.Config{})
		if err == nil {
			t.Errorf("controller %q: expected an error", spec.Type)
			continue
		}
		for _, kind := range pi.RunnableControllerTypes() {
			if !strings.Contains(err.Error(), kind) {
				t.Errorf("controller %q error should list supported types: %v", spec.Type, err)
			}
		}
	}
	// A type that exists in the protocol but not on this platform is refused
	// before MaaFramework is touched, and the message names the platform.
	kind := unsupportedControllerType()
	if kind == "" {
		t.Skip("every controller type is runnable on this platform")
	}
	_, err := createController(&pi.Controller{Name: "other", Type: kind}, runOptions{}, &clientconfig.Config{})
	if err == nil {
		t.Fatalf("controller %q: expected an error on %s", kind, pi.PlatformName())
	}
	if !strings.Contains(err.Error(), pi.PlatformName()) || !strings.Contains(err.Error(), kind) {
		t.Errorf("error should name the platform and the type: %v", err)
	}
}

// unsupportedControllerType returns a ProjectInterface controller type this
// platform cannot create, or "" when there is none.
func unsupportedControllerType() string {
	for _, kind := range []string{"Win32", "Gamepad", "MacOS", "PlayCover", "Linux"} {
		if !pi.Compatible(pi.RunnableControllerTypes(), kind) {
			return kind
		}
	}
	return ""
}

func TestMacOSScreencapMethod(t *testing.T) {
	method, err := macOSScreencapMethod("", "")
	if err != nil || method != macosDefaultScreencap {
		t.Errorf("default screencap = %v, %v; want ScreenCaptureKit", method, err)
	}
	for _, spelling := range []string{"ScreenCaptureKit", "screen_capture_kit", "screencapturekit"} {
		method, err := macOSScreencapMethod("", spelling)
		if err != nil || method != macos.ScreencapScreenCaptureKit {
			t.Errorf("screencap %q = %v, %v", spelling, method, err)
		}
	}
	if _, err := macOSScreencapMethod("nope", ""); err == nil {
		t.Error("expected an invalid screencap method to fail")
	}
}

func TestMacOSInputMethod(t *testing.T) {
	method, err := macOSInputMethod("", "")
	if err != nil || method != macosDefaultInput {
		t.Errorf("default input = %v, %v; want GlobalEvent", method, err)
	}
	method, err = macOSInputMethod("", "PostToPid")
	if err != nil || method != macos.InputPostToPid {
		t.Errorf("PI input = %v, %v", method, err)
	}
	method, err = macOSInputMethod("GlobalEvent", "PostToPid")
	if err != nil || method != macos.InputGlobalEvent {
		t.Errorf("CLI override = %v, %v; want GlobalEvent", method, err)
	}
	if _, err := macOSInputMethod("nope", ""); err == nil {
		t.Error("expected an invalid input method to fail")
	}
}

func TestResolveMacOSWindowID(t *testing.T) {
	// No selector means the whole desktop, which is what MaaPiCli does.
	id, err := resolveMacOSWindowID(&pi.Controller{}, runOptions{}, &clientconfig.Config{})
	if err != nil || id != 0 {
		t.Errorf("no selector: id = %d, err = %v; want 0", id, err)
	}
	id, err = resolveMacOSWindowID(&pi.Controller{}, runOptions{macosWindowID: "0x2a"}, &clientconfig.Config{})
	if err != nil || id != 42 {
		t.Errorf("flag window id: id = %d, err = %v; want 42", id, err)
	}
	config := &clientconfig.Config{MacOS: clientconfig.MacOS{WindowID: 7}}
	if id, err = resolveMacOSWindowID(&pi.Controller{}, runOptions{}, config); err != nil || id != 7 {
		t.Errorf("config window id: id = %d, err = %v; want 7", id, err)
	}
	if id, err = resolveMacOSWindowID(&pi.Controller{}, runOptions{macosWindowID: "9"}, config); err != nil || id != 9 {
		t.Errorf("flag wins over config: id = %d, err = %v; want 9", id, err)
	}
	if _, err = resolveMacOSWindowID(&pi.Controller{}, runOptions{macosWindowID: "nope"}, &clientconfig.Config{}); err == nil {
		t.Error("expected an invalid window id to fail")
	}
}

func TestLinuxSocketPath(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")
	path, err := linuxSocketPath("", "")
	if err != nil || path != "wayland-1" {
		t.Errorf("WAYLAND_DISPLAY fallback = %q, %v", path, err)
	}
	if path, err = linuxSocketPath("/run/user/1000/wayland-0", "wayland-1"); err != nil || path != "/run/user/1000/wayland-0" {
		t.Errorf("flag = %q, %v", path, err)
	}
	if path, err = linuxSocketPath("", "wayland-9"); err != nil || path != "wayland-9" {
		t.Errorf("config = %q, %v", path, err)
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	if _, err := linuxSocketPath("", ""); err == nil {
		t.Error("expected a missing Wayland socket to fail")
	}
}

func TestCheckLinuxMethods(t *testing.T) {
	for name, config := range map[string]pi.LinuxConfig{
		"empty":      {},
		"wlr":        {Screencap: "Wlr", Input: "Wlr"},
		"mixed case": {Screencap: "wlr"},
	} {
		if err := checkLinuxMethods(&pi.Controller{Linux: config}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, config := range map[string]pi.LinuxConfig{
		"pipewire": {Screencap: "PipeWire"},
		"uinput":   {Input: "UInput"},
		"libei":    {Input: "Libei"},
	} {
		err := checkLinuxMethods(&pi.Controller{Name: "linux", Linux: config})
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if !strings.Contains(err.Error(), "wlroots") {
			t.Errorf("%s: error should explain what this build supports: %v", name, err)
		}
	}
}
