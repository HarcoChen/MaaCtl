// Package i18n selects the help output language and provides localized text.
package i18n

import (
	"os"
	"strings"
	"sync"
)

// Language identifies a supported help output language.
type Language int

const (
	EN Language = iota
	ZH
)

// Text returns the English or Chinese text for the active help language.
func Text(en, zh string) string {
	if Active() == ZH {
		return zh
	}
	return en
}

// Active resolves the help language: MAACTL_LANG overrides the system UI
// language, and English is the fallback for unsupported languages.
func Active() Language {
	if lang, ok := Parse(os.Getenv("MAACTL_LANG")); ok {
		return lang
	}
	return systemLanguage()
}

// systemLanguage caches the system language; it never changes during a run.
var systemLanguage = sync.OnceValue(detectSystemLanguage)

// Parse maps locale strings such as "zh-CN" or "en_US.UTF-8" to a supported
// language.
func Parse(value string) (Language, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(value, "zh"):
		return ZH, true
	case strings.HasPrefix(value, "en"):
		return EN, true
	default:
		return EN, false
	}
}
