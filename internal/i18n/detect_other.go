//go:build !windows

package i18n

import "os"

// detectSystemLanguage reads the POSIX locale environment.
func detectSystemLanguage() Language {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang, ok := Parse(os.Getenv(name)); ok {
			return lang
		}
	}
	return EN
}
