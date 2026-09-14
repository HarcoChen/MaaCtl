//go:build windows

package main

import "syscall"

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// systemLanguage returns the primary language of the Windows UI language.
func systemLanguage() language {
	id, _, _ := procGetUserDefaultUILanguage.Call()
	const langChinese = 0x04 // LANG_CHINESE primary language id
	if uint16(id)&0x3ff == langChinese {
		return langZH
	}
	return langEN
}
