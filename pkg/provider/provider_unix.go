//go:build !windows

// Package provider implements provider resolution and invocation for summon.
// This file contains Unix-specific helpers for process group management.
package provider

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// isProviderFile reports whether a directory entry qualifies as a provider
// binary on Unix: the file must have at least one execute bit set.
func isProviderFile(info os.FileInfo) bool {
	return info.Mode()&0111 != 0
}

// setCallProcessGroup configures cmd so that:
//  1. The child starts in its own process group (Setpgid = true) and, when the
//     context ends, cmd.Cancel sends SIGKILL to that whole group, so the
//     provider's own children (shell pipelines, helpers) die with it.
//  2. WaitDelay is set to a fraction of the provider timeout, so cmd.Wait
//     force-closes the I/O pipes and returns even if a process that left the
//     group (setsid) still holds them open.
//
// Without the group kill, exec.CommandContext signals only the direct child
// and its grandchildren would outlive the cancelled fetch.
func setCallProcessGroup(cmd *exec.Cmd, timeout time.Duration) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	// WaitDelay must be > 0 to take effect. The processes are already killed
	// when it starts counting, so keep it short (see maxWaitDelay).
	if timeout > 0 {
		cmd.WaitDelay = min(timeout/2, maxWaitDelay)
	}
}
