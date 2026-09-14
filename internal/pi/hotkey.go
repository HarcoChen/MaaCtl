package pi

import (
	"fmt"
	"strconv"
	"strings"
)

// Hotkey is a human-readable key combination such as "Ctrl+Shift+A". The last
// segment is the primary key; the earlier segments are modifiers.
type Hotkey struct {
	Primary   string
	Modifiers []string
}

// String renders the hotkey in the canonical "Modifier+Modifier+Key" form.
func (h Hotkey) String() string {
	if len(h.Modifiers) == 0 {
		return h.Primary
	}
	return strings.Join(append(append([]string(nil), h.Modifiers...), h.Primary), "+")
}

// KeyCodes is a hotkey translated for one controller: the primary virtual key
// code plus the modifier codes in the order the user wrote them.
type KeyCodes struct {
	Primary   int
	Modifiers []int
}

// ParseHotkey parses a hotkey string. An empty string is rejected so callers do
// not silently produce a key code of 0.
func ParseHotkey(value string) (Hotkey, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return Hotkey{}, fmt.Errorf("empty hotkey")
	}
	parts := strings.Split(trimmed, "+")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return Hotkey{}, fmt.Errorf("invalid hotkey %q", value)
	}
	if len(cleaned) == 1 {
		return Hotkey{Primary: canonicalKeyName(cleaned[0])}, nil
	}
	hotkey := Hotkey{Primary: canonicalKeyName(cleaned[len(cleaned)-1])}
	for _, modifier := range cleaned[:len(cleaned)-1] {
		hotkey.Modifiers = append(hotkey.Modifiers, canonicalKeyName(modifier))
	}
	return hotkey, nil
}

// KeyCodes maps a hotkey onto a controller's virtual key codes. Only Adb and
// Win32 define key codes that MaaFramework's ClickKey/KeyDown accept; other
// controller types report an explicit error instead of guessing.
func (h Hotkey) KeyCodes(controllerType string) (KeyCodes, error) {
	table, err := hotkeyTable(controllerType)
	if err != nil {
		return KeyCodes{}, err
	}
	primary, ok := table[lookupKey(h.Primary)]
	if !ok {
		return KeyCodes{}, fmt.Errorf("unknown key %q for %s controller", h.Primary, controllerType)
	}
	codes := KeyCodes{Primary: primary}
	for _, modifier := range h.Modifiers {
		code, ok := table[lookupKey(modifier)]
		if !ok {
			return KeyCodes{}, fmt.Errorf("unknown modifier %q for %s controller", modifier, controllerType)
		}
		codes.Modifiers = append(codes.Modifiers, code)
	}
	return codes, nil
}

// modifierAliases maps the many spellings of modifier keys onto one canonical
// name so a hotkey parses the same way regardless of who wrote it.
var modifierAliases = map[string]string{
	"CTRL": "Ctrl", "CONTROL": "Ctrl",
	"SHIFT": "Shift",
	"ALT":   "Alt", "OPTION": "Alt",
	"WIN": "Win", "SUPER": "Win", "META": "Win", "CMD": "Win", "COMMAND": "Win",
}

// canonicalKeyName normalizes a key segment: modifiers get one spelling, single
// letters are uppercased, and everything else is trimmed.
func canonicalKeyName(name string) string {
	name = strings.TrimSpace(name)
	if canonical, ok := modifierAliases[strings.ToUpper(name)]; ok {
		return canonical
	}
	upper := strings.ToUpper(name)
	if len(upper) == 1 {
		return upper
	}
	// F1..F24 are conventionally written uppercase.
	if strings.HasPrefix(upper, "F") {
		if n, err := strconv.Atoi(upper[1:]); err == nil && n >= 1 && n <= 24 {
			return "F" + strconv.Itoa(n)
		}
	}
	return name
}

// lookupKey is the table lookup form of a canonical key name.
func lookupKey(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

// hotkeyTable returns the virtual key code table for a controller type.
func hotkeyTable(controllerType string) (map[string]int, error) {
	switch strings.ToLower(strings.TrimSpace(controllerType)) {
	case "win32":
		return win32KeyCodes, nil
	case "adb":
		return adbKeyCodes, nil
	default:
		return nil, fmt.Errorf("hotkeys are not supported for %q controllers", controllerType)
	}
}

// win32KeyCodes maps key names to Windows virtual-key codes.
var win32KeyCodes = func() map[string]int {
	table := map[string]int{
		"CTRL": 0x11, "SHIFT": 0x10, "ALT": 0x12, "WIN": 0x5B,
		"SPACE": 0x20, "ENTER": 0x0D, "RETURN": 0x0D, "TAB": 0x09,
		"ESC": 0x1B, "ESCAPE": 0x1B, "BACKSPACE": 0x08,
		"DELETE": 0x2E, "DEL": 0x2E, "INSERT": 0x2D,
		"HOME": 0x24, "END": 0x23, "PAGEUP": 0x21, "PAGEDOWN": 0x22,
		"UP": 0x26, "DOWN": 0x28, "LEFT": 0x25, "RIGHT": 0x27,
		"CAPSLOCK": 0x14, "NUMLOCK": 0x90, "PRINTSCREEN": 0x2C, "PAUSE": 0x13,
		"MINUS": 0xBD, "EQUAL": 0xBB, "COMMA": 0xBC, "PERIOD": 0xBE,
		"SLASH": 0xBF, "SEMICOLON": 0xBA, "QUOTE": 0xDE, "BACKQUOTE": 0xC0,
		"BRACKETLEFT": 0xDB, "BACKSLASH": 0xDC, "BRACKETRIGHT": 0xDD,
	}
	for c := 'A'; c <= 'Z'; c++ {
		table[string(c)] = 0x41 + int(c-'A')
	}
	for d := '0'; d <= '9'; d++ {
		table[string(d)] = 0x30 + int(d-'0')
	}
	for n := 1; n <= 24; n++ {
		table[fmt.Sprintf("F%d", n)] = 0x70 + n - 1
	}
	return table
}()

// adbKeyCodes maps key names to Android KeyEvent key codes.
var adbKeyCodes = func() map[string]int {
	table := map[string]int{
		"CTRL": 113, "SHIFT": 59, "ALT": 57, "WIN": 117,
		"SPACE": 62, "ENTER": 66, "RETURN": 66, "TAB": 61,
		"ESC": 111, "ESCAPE": 111, "BACKSPACE": 67,
		"DELETE": 67, "DEL": 67, "INSERT": 124,
		"HOME": 3, "END": 123, "PAGEUP": 92, "PAGEDOWN": 93,
		"UP": 19, "DOWN": 20, "LEFT": 21, "RIGHT": 22,
		"CAPSLOCK": 115, "MINUS": 69, "EQUAL": 70, "COMMA": 55, "PERIOD": 56,
		"SLASH": 76, "SEMICOLON": 74, "QUOTE": 75, "BACKQUOTE": 68,
		"BRACKETLEFT": 71, "BACKSLASH": 73, "BRACKETRIGHT": 72,
	}
	for c := 'A'; c <= 'Z'; c++ {
		table[string(c)] = 29 + int(c-'A')
	}
	for d := '0'; d <= '9'; d++ {
		table[string(d)] = 7 + int(d-'0')
	}
	for n := 1; n <= 12; n++ {
		table[fmt.Sprintf("F%d", n)] = 131 + n - 1
	}
	return table
}()
