//go:build !windows && !solaris

package summon

import (
	"os"
	"os/exec"
	"syscall"
)

// unixTree is a process group led by the supervised child.
type unixTree struct {
	pgid int
}

// prepareChild puts the child in its own process group so signals reach the
// child and every descendant.
func prepareChild(cmd *exec.Cmd) {
	setProcessGroup(cmd)
}

func attachTree(cmd *exec.Cmd) (processTree, error) {
	return &unixTree{pgid: getProcessGroupID(cmd.Process.Pid)}, nil
}

func (t *unixTree) gracefulStop() error { return syscall.Kill(-t.pgid, syscall.SIGTERM) }

func (t *unixTree) forceKill() error { return syscall.Kill(-t.pgid, syscall.SIGKILL) }

// alive reports whether the group still has a running member (see
// groupAlive for how zombies are treated).
func (t *unixTree) alive() bool { return groupAlive(t.pgid) }

// signalProbe probes the group with signal 0: it succeeds while any member
// exists, zombies included.
func signalProbe(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil || err == syscall.EPERM
}

func (t *unixTree) signal(sig os.Signal) {
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(-t.pgid, s)
	}
}

func (t *unixTree) close() {}

// warnIfGracefulStopUnavailable is a no-op on Unix: process groups always
// support a graceful stop.
func warnIfGracefulStopUnavailable() {}

// ignoredSignal reports signals Summon receives for its own sake (runtime
// preemption, child-exit notification, a write of Summon's own to a closed
// pipe) that must not be relayed to the child: SIGPIPE would otherwise kill a
// healthy child.
func ignoredSignal(sig os.Signal) bool {
	return sig == syscall.SIGURG || sig == syscall.SIGCHLD || sig == syscall.SIGPIPE
}

// stopExitCode maps the exit code of a child that Summon stopped. On Unix a
// stop shows up as death by signal, not an exit code, so codes pass through.
func stopExitCode(code int) int { return code }
