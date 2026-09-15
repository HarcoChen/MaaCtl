package pi

import (
	"strings"
	"testing"
)

// TestParseHotkey covers the accepted spellings and the rejected inputs.
func TestParseHotkey(t *testing.T) {
	cases := []struct {
		input     string
		primary   string
		modifiers []string
	}{
		{"A", "A", nil},
		{"a", "A", nil},
		{"ctrl+a", "A", []string{"Ctrl"}},
		{"Ctrl+Shift+A", "A", []string{"Ctrl", "Shift"}},
		{"control+option+super+a", "A", []string{"Ctrl", "Alt", "Win"}},
		{"  Ctrl + A  ", "A", []string{"Ctrl"}},
		{"f1", "F1", nil},
		{"cmd+F24", "F24", []string{"Win"}},
		{"F13", "F13", nil},
	}
	for _, tc := range cases {
		got, err := ParseHotkey(tc.input)
		if err != nil {
			t.Errorf("ParseHotkey(%q): %v", tc.input, err)
			continue
		}
		if got.Primary != tc.primary {
			t.Errorf("ParseHotkey(%q).Primary = %q, want %q", tc.input, got.Primary, tc.primary)
		}
		if strings.Join(got.Modifiers, ",") != strings.Join(tc.modifiers, ",") {
			t.Errorf("ParseHotkey(%q).Modifiers = %v, want %v", tc.input, got.Modifiers, tc.modifiers)
		}
	}
	if _, err := ParseHotkey(""); err == nil {
		t.Error("an empty hotkey must be rejected")
	}
	if _, err := ParseHotkey("+"); err == nil {
		t.Error("a hotkey of only separators must be rejected")
	}
}

// TestHotkeyString checks the canonical rendering.
func TestHotkeyString(t *testing.T) {
	combined, err := ParseHotkey("ctrl+shift+a")
	if err != nil {
		t.Fatal(err)
	}
	if got := combined.String(); got != "Ctrl+Shift+A" {
		t.Errorf("String() = %q, want Ctrl+Shift+A", got)
	}
	single, err := ParseHotkey("a")
	if err != nil {
		t.Fatal(err)
	}
	if got := single.String(); got != "A" {
		t.Errorf("String() = %q, want A", got)
	}
}

// TestCanonicalKeyName covers the modifier aliases and the F1..F24 range.
func TestCanonicalKeyName(t *testing.T) {
	cases := map[string]string{
		"f1": "F1", "F24": "F24", "f12": "F12",
		"cmd": "Win", "option": "Alt", "CONTROL": "Ctrl", "meta": "Win",
		"a": "A", "PageUp": "PageUp",
	}
	for input, want := range cases {
		if got := canonicalKeyName(input); got != want {
			t.Errorf("canonicalKeyName(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestKeyCodeTables pins the win32 and adb tables, including the two Android
// codes that must not be swapped: BACKSPACE is KEYCODE_DEL (67) and DELETE is
// KEYCODE_FORWARD_DEL (112).
func TestKeyCodeTables(t *testing.T) {
	entries := []struct {
		key   string
		win32 int
		adb   int // 0 means the key is absent from the Android table
	}{
		{"DELETE", 0x2E, 112},
		{"DEL", 0x2E, 112},
		{"BACKSPACE", 0x08, 67},
		{"ENTER", 0x0D, 66},
		{"CTRL", 0x11, 113},
		{"SHIFT", 0x10, 59},
		{"A", 0x41, 29},
		{"Z", 0x5A, 54},
		{"0", 0x30, 7},
		{"9", 0x39, 16},
		{"F1", 0x70, 131},
		{"F12", 0x7B, 142},
		{"F13", 0x7C, 0},
		{"F24", 0x87, 0},
	}
	for _, e := range entries {
		if got := win32KeyCodes[e.key]; got != e.win32 {
			t.Errorf("win32 %s = %#x, want %#x", e.key, got, e.win32)
		}
		if e.adb != 0 {
			if got := adbKeyCodes[e.key]; got != e.adb {
				t.Errorf("adb %s = %d, want %d", e.key, got, e.adb)
			}
		} else if _, ok := adbKeyCodes[e.key]; ok {
			t.Errorf("adb should not define %s", e.key)
		}
	}
	if _, ok := win32KeyCodes["NONEXISTENT"]; ok {
		t.Error("win32 table should not define an unknown key")
	}
}

// TestKeyCodesModifierOrder checks the codes a combination maps to, in written
// order, and the lookup errors.
func TestKeyCodesModifierOrder(t *testing.T) {
	hotkey, err := ParseHotkey("Ctrl+Shift+A")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := hotkey.KeyCodes("Win32")
	if err != nil {
		t.Fatal(err)
	}
	if codes.Primary != 0x41 || len(codes.Modifiers) != 2 || codes.Modifiers[0] != 0x11 || codes.Modifiers[1] != 0x10 {
		t.Errorf("codes = %+v", codes)
	}
}

// TestKeyCodesErrors covers the unknown-key and unknown-controller messages.
func TestKeyCodesErrors(t *testing.T) {
	unknown, err := ParseHotkey("Nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unknown.KeyCodes("Win32"); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("unknown key err = %v, want unknown key", err)
	}
	ok, err := ParseHotkey("A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ok.KeyCodes("Nope"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unknown controller err = %v, want not supported", err)
	}
}

// TestHotkeyTableSelectsController checks the controller-type dispatch.
func TestHotkeyTableSelectsController(t *testing.T) {
	if _, err := hotkeyTable("adb"); err != nil {
		t.Errorf("adb: %v", err)
	}
	if _, err := hotkeyTable("WIN32"); err != nil {
		t.Errorf("case-insensitive win32: %v", err)
	}
	if _, err := hotkeyTable("nope"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("unknown controller err = %v", err)
	}
}
