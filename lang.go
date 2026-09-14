package main

import (
	"os"
	"strings"
	"sync"
)

// language identifies a supported help output language.
type language int

const (
	langEN language = iota
	langZH
)

// localized returns the English or Chinese text for the active help language.
func localized(en, zh string) string {
	if activeLanguage() == langZH {
		return zh
	}
	return en
}

// activeLanguage resolves the help language: MAACTL_LANG overrides the system
// UI language, and English is the fallback for unsupported languages.
func activeLanguage() language {
	if lang, ok := parseLanguage(os.Getenv("MAACTL_LANG")); ok {
		return lang
	}
	return systemLang()
}

// systemLang caches the system language; it never changes during a run.
var systemLang = sync.OnceValue(systemLanguage)

// parseLanguage maps locale strings such as "zh-CN" or "en_US.UTF-8" to a
// supported language.
func parseLanguage(value string) (language, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(value, "zh"):
		return langZH, true
	case strings.HasPrefix(value, "en"):
		return langEN, true
	default:
		return langEN, false
	}
}
