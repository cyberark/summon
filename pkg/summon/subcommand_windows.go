//go:build windows

// Package summon provides the core logic for fetching secrets and executing
// subcommands with those secrets injected into the environment.
// This file contains Windows-specific stubs for process group signal delivery.
package summon

import (
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on Windows; process groups are not used.
func setProcessGroup(_ *exec.Cmd) {}

// forwardSignalToGroup falls back to signalling the direct child process on
// Windows, where process groups and syscall.Kill(-pgid, sig) are unavailable.
func forwardSignalToGroup(proc *os.Process, _ int, sig os.Signal) {
	_ = proc.Signal(sig)
}

// getProcessGroupID returns pid on Windows; process groups are not used.
func getProcessGroupID(pid int) int { return pid }
