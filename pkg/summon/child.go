//go:build !solaris

package summon

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// treePollInterval is how often stop checks whether the descendants of a
// stopped child are gone.
const treePollInterval = 50 * time.Millisecond

// killDrainTimeout bounds how long stop waits for force-killed processes to
// disappear.
const killDrainTimeout = 2 * time.Second

// processTree controls every process descended from a supervised child. The
// implementation is OS-specific: a process group on Unix, a console process
// group plus Job Object on Windows.
type processTree interface {
	// gracefulStop asks the whole tree to shut down (SIGTERM / CTRL_BREAK).
	gracefulStop() error
	// forceKill kills every process in the tree (SIGKILL / TerminateJobObject).
	forceKill() error
	// alive reports whether any process in the tree is still running.
	alive() bool
	// signal forwards a signal received by Summon to the tree.
	signal(sig os.Signal)
	// close releases OS resources held for the tree.
	close()
}

// treeGuard is the one place that owns and synchronizes the live process
// tree. It holds the current child's tree and turns every call into a no-op
// while no tree is attached or after close. The signal forwarder keeps
// calling it while the refresh loop closes the old tree and attaches the
// next one on restart; acting on a released OS handle (a Windows Job Object)
// would hit a closed or reused handle, so no OS call happens outside the lock
// or after close. The zero value is ready to use.
type treeGuard struct {
	mu    sync.Mutex
	inner processTree // nil: none attached, or closed
}

// attach makes t the guarded tree, replacing a previous (closed) one.
func (g *treeGuard) attach(t processTree) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inner = t
}

func (g *treeGuard) gracefulStop() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner == nil {
		return nil
	}
	return g.inner.gracefulStop()
}

func (g *treeGuard) forceKill() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner == nil {
		return nil
	}
	return g.inner.forceKill()
}

func (g *treeGuard) alive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inner != nil && g.inner.alive()
}

func (g *treeGuard) signal(sig os.Signal) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner != nil {
		g.inner.signal(sig)
	}
}

// close releases the attached tree; later calls are no-ops until the next
// attach.
func (g *treeGuard) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inner != nil {
		t := g.inner
		g.inner = nil
		t.close()
	}
}

// supervisedChild is one launch of the subprocess. Restarting the subprocess
// means stopping one supervisedChild and starting another.
type supervisedChild struct {
	cmd    *exec.Cmd
	tree   *treeGuard
	cancel context.CancelFunc
	grace  time.Duration

	// exited is closed once cmd.Wait has returned; err and exitedAt are valid
	// after that.
	exited   chan struct{}
	err      error
	exitedAt time.Time
}

// grantForeground is grantTerminalForeground, replaceable in tests.
var grantForeground = grantTerminalForeground

// startChild launches command in its own process tree and attaches that tree
// to guard. grace is the time stop allows for the tree to exit gracefully
// before force-killing it.
func startChild(command []string, env []string, grace time.Duration, guard *treeGuard) (*supervisedChild, error) {
	binary, err := exec.LookPath(command[0])
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	child := &supervisedChild{tree: guard, cancel: cancel, grace: grace, exited: make(chan struct{})}

	cmd := exec.CommandContext(ctx, binary, command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	// Cancelling the context asks the whole tree to stop; WaitDelay then makes
	// Go kill the direct child if it is still running after the grace period.
	// Descendants are handled by stop.
	cmd.Cancel = func() error { return child.tree.gracefulStop() }
	cmd.WaitDelay = grace
	prepareChild(cmd)
	child.cmd = cmd

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	tree, err := attachTree(cmd)
	if err != nil {
		// Without tree control a restart could leave orphans: refuse to run.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cancel()
		return nil, err
	}
	guard.attach(tree)
	// Like one-shot mode: with a tty stdin the child's process group needs the
	// foreground, or its first stdin read gets SIGTTIN (CNJR-15475). Also runs
	// on every restart; a no-op without a tty, and on Windows.
	grantForeground(getProcessGroupID(cmd.Process.Pid))

	go func() {
		child.err = cmd.Wait()
		child.exitedAt = time.Now()
		close(child.exited)
	}()
	return child, nil
}

// stop stops the whole tree: graceful stop, up to the grace period for the
// tree to drain, then force-kill of whatever is left. It reports whether the
// force-kill was needed.
func (c *supervisedChild) stop() (forced bool) {
	stopStart := time.Now()
	deadline := stopStart.Add(c.grace)
	c.cancel()
	<-c.exited
	defer c.tree.close()
	// Give the terminal back to Summon; the next child (if any) takes it again.
	defer restoreTerminalForeground()

	// WaitDelay killed the direct child if it outlived the grace period.
	forced = !c.exitedAt.Before(deadline)

	for c.tree.alive() && time.Now().Before(deadline) {
		time.Sleep(treePollInterval)
	}
	if c.tree.alive() {
		_ = c.tree.forceKill()
		forced = true
		// Killed processes linger briefly until the OS reaps them.
		for end := time.Now().Add(killDrainTimeout); c.tree.alive() && time.Now().Before(end); {
			time.Sleep(treePollInterval)
		}
	}
	return forced
}

// isStopSignal reports whether sig asks Summon itself to stop in daemon mode.
// Unlike isTermSignal (one-shot), SIGHUP is not one: it is only forwarded.
func isStopSignal(sig os.Signal) bool {
	return sig == syscall.SIGINT || sig == syscall.SIGTERM
}

// forwardSignals relays signals received by Summon to the guarded tree until
// done is closed. The first stop signal (SIGINT/SIGTERM) is not forwarded:
// it sets terminating and calls stop, which stops the tree gracefully within
// the grace period. A second one force-kills the tree so Summon can always
// exit. Other signals, SIGHUP included, are forwarded as-is.
func forwardSignals(tree *treeGuard, terminating *atomic.Bool, stop func(), done <-chan struct{}) {
	ch := make(chan os.Signal, 16)
	signal.Notify(ch)
	go func() {
		defer signal.Stop(ch)
		received := false
		for {
			select {
			case sig := <-ch:
				if ignoredSignal(sig) {
					continue
				}
				if isStopSignal(sig) {
					terminating.Store(true)
					if received {
						_ = tree.forceKill()
						return
					}
					received = true
					stop()
					continue
				}
				tree.signal(sig)
			case <-done:
				return
			}
		}
	}()
}
