//go:build !windows

package main

import "os"

// systemLanguage reads the POSIX locale environment.
func systemLanguage() language {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang, ok := parseLanguage(os.Getenv(name)); ok {
			return lang
		}
	}
	return langEN
}
