//go:build solaris

// Package summon provides the core logic for fetching secrets and executing
// subcommands with those secrets injected into the environment.
// This file contains Solaris stubs for process group signal delivery.
// syscall.Getpgid is not available on Solaris; signals are delivered to the
// direct child only.
package summon

import (
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on Solaris; process group management requires
// syscall.Getpgid which is not available in Go's Solaris syscall package.
func setProcessGroup(_ *exec.Cmd) {}

// forwardSignalToGroup falls back to signalling the direct child process on
// Solaris, where syscall.Getpgid is unavailable.
func forwardSignalToGroup(proc *os.Process, _ int, sig os.Signal) {
	_ = proc.Signal(sig)
}

// getProcessGroupID returns pid on Solaris; process groups are not used.
func getProcessGroupID(pid int) int { return pid }
