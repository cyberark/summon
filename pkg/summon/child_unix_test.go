//go:build !windows && !solaris

package summon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pidGone reports whether pid no longer runs. A zombie counts as gone: when
// PID 1 does not reap orphans (containers), a killed descendant stays one.
func pidGone(pid int) bool {
	if syscall.Kill(pid, 0) == syscall.ESRCH {
		return true
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat") // Linux only
	if err != nil {
		return false
	}
	i := strings.LastIndexByte(string(b), ')')
	return i >= 0 && i+2 < len(b) && b[i+2] == 'Z'
}

func TestSupervisedChildStop(t *testing.T) {
	t.Run("cooperative child stops gracefully without force", func(t *testing.T) {
		child, err := startChild([]string{"bash", "-c", `trap 'exit 0' TERM; sleep 60 & wait`}, os.Environ(), 5*time.Second, &treeGuard{})
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond) // let the trap install

		start := time.Now()
		forced := child.stop()

		assert.False(t, forced)
		assert.Less(t, time.Since(start), 3*time.Second)
		assert.False(t, child.tree.alive())
	})

	t.Run("child ignoring SIGTERM is force-killed after the grace period", func(t *testing.T) {
		child, err := startChild([]string{"bash", "-c", `trap '' TERM; sleep 60`}, os.Environ(), 300*time.Millisecond, &treeGuard{})
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		forced := child.stop()

		assert.True(t, forced)
		assert.False(t, child.tree.alive())
	})

	t.Run("descendants do not survive a stop", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
		script := `(trap '' TERM; sleep 60) & echo $! > ` + pidFile + `; trap '' TERM; wait`
		child, err := startChild([]string{"bash", "-c", script}, os.Environ(), 300*time.Millisecond, &treeGuard{})
		require.NoError(t, err)
		waitFor(t, "grandchild pid file", 3*time.Second, func() bool {
			b, err := os.ReadFile(pidFile)
			return err == nil && strings.TrimSpace(string(b)) != ""
		})
		b, _ := os.ReadFile(pidFile)
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		require.NoError(t, err)

		child.stop()

		waitFor(t, "grandchild to be gone", 3*time.Second, func() bool { return pidGone(pid) })
	})

	t.Run("exited reports the child's own exit", func(t *testing.T) {
		child, err := startChild([]string{"bash", "-c", "exit 3"}, os.Environ(), time.Second, &treeGuard{})
		require.NoError(t, err)

		select {
		case <-child.exited:
		case <-time.After(3 * time.Second):
			t.Fatal("child did not exit")
		}

		code, err := returnStatusOfError(child.err)
		require.NoError(t, err)
		assert.Equal(t, 3, code)
	})
}

func TestIgnoredSignal(t *testing.T) {
	assert.True(t, ignoredSignal(syscall.SIGURG))
	assert.True(t, ignoredSignal(syscall.SIGCHLD))
	assert.True(t, ignoredSignal(syscall.SIGPIPE))
	assert.False(t, ignoredSignal(syscall.SIGTERM))
	assert.False(t, ignoredSignal(syscall.SIGUSR1))
}

func TestStartChildGrantsTerminalForeground(t *testing.T) {
	var got []int
	orig := grantForeground
	grantForeground = func(pgid int) { got = append(got, pgid) }
	t.Cleanup(func() { grantForeground = orig })

	child, err := startChild([]string{"bash", "-c", "exit 0"}, os.Environ(), time.Second, &treeGuard{})
	require.NoError(t, err)
	<-child.exited
	child.stop()

	require.Len(t, got, 1)
	assert.Equal(t, child.cmd.Process.Pid, got[0], "child leads its own process group")
}
