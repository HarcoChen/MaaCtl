package cli

import (
	"strings"
	"testing"

	"maactl/internal/pi"

	"github.com/MaaXYZ/maa-framework-go/v4/controller/win32"
)

func testWindows() []win32Window {
	return []win32Window{
		{handle: 0x100, className: "UnityWndClass", windowName: "原神"},
		{handle: 0x200, className: "Notepad", windowName: "notes.txt - Notepad"},
		{handle: 0x300, className: "Chrome_WidgetWin_1", windowName: "Notepad clone"},
	}
}

func TestMatchWin32Windows(t *testing.T) {
	cases := []struct {
		name     string
		selector win32Selector
		want     []uintptr
	}{
		{name: "empty keeps all", selector: win32Selector{}, want: []uintptr{0x100, 0x200, 0x300}},
		{name: "class regex", selector: win32Selector{class: "^Notepad$"}, want: []uintptr{0x200}},
		{name: "window regex", selector: win32Selector{window: "Notepad"}, want: []uintptr{0x200, 0x300}},
		{name: "class and window are ANDed", selector: win32Selector{class: "Chrome", window: "Notepad"}, want: []uintptr{0x300}},
		{name: "decimal handle", selector: win32Selector{handle: "256"}, want: []uintptr{0x100}},
		{name: "hex handle", selector: win32Selector{handle: "0x200"}, want: []uintptr{0x200}},
		{name: "handle and class", selector: win32Selector{handle: "0x300", class: "Notepad"}, want: nil},
		{name: "no match", selector: win32Selector{window: "missing"}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := matchWin32Windows(testWindows(), tc.selector)
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

func TestMatchWin32WindowsRejectsBadInput(t *testing.T) {
	for name, selector := range map[string]win32Selector{
		"bad handle":  {handle: "nope"},
		"bad class":   {class: "("},
		"bad title":   {window: "["},
		"bad integer": {handle: "0xzz"},
	} {
		if _, err := matchWin32Windows(testWindows(), selector); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseWin32Handle(t *testing.T) {
	handle, ok, err := parseWin32Handle("")
	if ok || err != nil || handle != 0 {
		t.Errorf("empty handle = %#x, %v, %v", handle, ok, err)
	}
	for input, want := range map[string]uintptr{"0x2a": 0x2a, "42": 42, " 0XFF ": 0xff} {
		handle, ok, err := parseWin32Handle(input)
		if err != nil || !ok || handle != want {
			t.Errorf("parse %q = %#x, %v, %v; want %#x", input, handle, ok, err, want)
		}
	}
	if _, _, err := parseWin32Handle("0x"); err == nil {
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
		{Name: "mac", Type: "MacOS"},
		{Name: "play", Type: "PlayCover"},
		{Name: "empty"},
	} {
		_, err := createController(spec, runOptions{})
		if err == nil {
			t.Errorf("controller %q: expected an error", spec.Type)
			continue
		}
		if !strings.Contains(err.Error(), "Adb") || !strings.Contains(err.Error(), "Win32") {
			t.Errorf("controller %q error should list supported types: %v", spec.Type, err)
		}
	}
}
