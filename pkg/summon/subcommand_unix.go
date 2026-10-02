//go:build !windows && !solaris

// Package summon provides the core logic for fetching secrets and executing
// subcommands with those secrets injected into the environment.
// This file contains Unix-specific helpers for process group signal delivery.
package summon

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"
)

// setProcessGroup places the command in its own process group so that
// signals can be delivered to the entire group (parent + all children).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// isStdinTerminal reports whether os.Stdin is a character device (terminal).
func isStdinTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// grantTerminalForeground gives the process group identified by pgid foreground
// access to the controlling terminal. It is a no-op when stdin is not a
// terminal. Ignores SIGTTOU (following the shell convention) so that summon
// can call tcsetpgrp even if it is currently in a background process group.
//
// This call fixes the SIGTTIN regression introduced in 0.13.0 by commit
// 5af7e13: setting Setpgid without granting the child foreground terminal
// access causes SIGTTIN when the child tries to read stdin (CNJR-15475).
func grantTerminalForeground(pgid int) {
	if !isStdinTerminal() {
		return
	}
	// Ignore SIGTTOU so the ioctl does not stop us if we are in a background
	// process group when this function runs — the standard shell pattern before
	// calling tcsetpgrp. We intentionally do not call signal.Reset afterward:
	// signal.Reset restores the default (stop) disposition and also removes any
	// prior signal.Notify registration for SIGTTOU from the forwarding channel,
	// which is strictly worse than leaving it ignored for the child's lifetime.
	// summon does not write to the terminal after this point, so ignoring SIGTTOU
	// is safe.
	signal.Ignore(syscall.SIGTTOU)

	// pid_t is int32 on all supported platforms (Linux, macOS, BSDs).
	// TIOCSPGRP takes a pid_t pointer. If ioctl fails (e.g. stdin is not the
	// controlling terminal for this session), we proceed silently — the child
	// may receive SIGTTIN and the user will observe it as a hang, but there is
	// no diagnostic we can emit without a logger at this call site.
	pgid32 := int32(pgid)
	//nolint:unsafeptr
	_, _, _ = syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdin.Fd(),
		uintptr(syscall.TIOCSPGRP),
		uintptr(unsafe.Pointer(&pgid32)),
	)
}

// restoreTerminalForeground reclaims the controlling terminal's foreground
// process group for summon after the interactive child exits. It is a no-op
// when stdin is not a terminal. Ignores SIGTTOU for the same reason as
// grantTerminalForeground.
func restoreTerminalForeground() {
	if !isStdinTerminal() {
		return
	}
	pgid, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		return
	}
	// At this point summon is in a background process group (the child held the
	// terminal foreground). SIGTTOU is required to be ignored before calling
	// ioctl(TIOCSPGRP) from a background group, otherwise the kernel stops us.
	// As in grantTerminalForeground, we do not call signal.Reset afterward.
	signal.Ignore(syscall.SIGTTOU)

	pgid32 := int32(pgid) // pid_t is int32 on all supported platforms
	//nolint:unsafeptr
	_, _, _ = syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdin.Fd(),
		uintptr(syscall.TIOCSPGRP),
		uintptr(unsafe.Pointer(&pgid32)),
	)
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
