//go:build !windows && !solaris

// Package summon provides the core logic for fetching secrets and executing
// subcommands with those secrets injected into the environment.
// This file contains Unix-specific helpers for process group signal delivery.
package summon

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup places the command in its own process group so that
// signals can be delivered to the entire group (parent + all children).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// forwardSignalToGroup delivers sig to the process group whose leader is proc.
// Using a negative pgid targets every process in the group, not just the
// direct child — this ensures grandchildren do not outlive the deadline.
func forwardSignalToGroup(proc *os.Process, pgid int, sig os.Signal) {
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(-pgid, s)
	}
}

// getProcessGroupID returns the process group ID for the given pid. If Getpgid
// fails (e.g. the process already exited), it falls back to pid itself so that
// signal delivery still targets at least the direct child.
func getProcessGroupID(pid int) int {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return pid
	}
	return pgid
}
