//go:build !windows

package cli

import "syscall"

// agentSysProcAttr returns no Windows-specific attributes; on other systems a
// silent agent is one that only writes to the pipes maactl gives it.
func agentSysProcAttr() *syscall.SysProcAttr {
	return nil
}
