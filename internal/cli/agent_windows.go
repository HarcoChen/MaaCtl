//go:build windows

package cli

import "syscall"

// createNoWindow starts the agent child process without a console window.
const createNoWindow = 0x08000000

// agentSysProcAttr keeps an agent that is a console program (python.exe,
// node.exe, ...) from opening its own console window: the child runs silently in
// the background and writes to the pipes maactl gives it.
func agentSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
