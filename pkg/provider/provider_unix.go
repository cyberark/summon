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
//  1. The child starts in its own process group (Setpgid = true), preventing
//     grandchildren from being orphaned when the context deadline fires.
//  2. WaitDelay is set to a generous fraction of the provider timeout, so
//     exec.CommandContext force-closes I/O pipes and unblocks cmd.Wait after
//     sending SIGKILL to the process group if the direct child does not exit.
//
// Without Setpgid, exec.CommandContext sends SIGKILL only to the direct child.
// Grandchildren keep the stdout pipe open, and cmd.Wait blocks indefinitely
// even though the context has already expired.
func setCallProcessGroup(cmd *exec.Cmd, timeout time.Duration) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// WaitDelay must be > 0 to take effect. Use half the provider timeout so
	// the total wall-clock budget stays within 1.5x the configured timeout.
	if timeout > 0 {
		cmd.WaitDelay = timeout / 2
	}
}
