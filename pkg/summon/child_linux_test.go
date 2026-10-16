//go:build linux

package summon

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// procState returns the /proc state letter and process group of pid, or 0 if
// unreadable. Deliberately independent of the code under test.
func procState(pid int) (byte, int) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, 0
	}
	f := strings.Fields(string(b[i+1:])) // state ppid pgrp ...
	if len(f) < 3 {
		return 0, 0
	}
	pgrp, _ := strconv.Atoi(f[2])
	return f[0][0], pgrp
}

// zombiesInGroup counts unreaped zombies in process group pgid.
func zombiesInGroup(pgid int) int {
	n := 0
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil {
			if st, pg := procState(pid); st == 'Z' && pg == pgid {
				n++
			}
		}
	}
	return n
}

// startGroupMember starts argv in its own process group and returns the
// command; the caller owns Wait.
func startGroupMember(t *testing.T, argv ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	return cmd
}

// startZombie starts a process group whose only member exited and has not
// been reaped yet.
func startZombie(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := startGroupMember(t, "sh", "-c", "exit 0")
	waitFor(t, "member to become a zombie", 3*time.Second, func() bool {
		st, _ := procState(cmd.Process.Pid)
		return st == 'Z'
	})
	return cmd
}

func TestUnixTreeAliveIgnoresZombies(t *testing.T) {
	t.Run("group with only a zombie is not alive", func(t *testing.T) {
		z := startZombie(t)
		assert.False(t, (&unixTree{pgid: z.Process.Pid}).alive())
		_ = z.Wait()
	})

	t.Run("group with a live process is alive", func(t *testing.T) {
		p := startGroupMember(t, "sleep", "60")
		defer func() { _ = p.Process.Kill(); _ = p.Wait() }()
		assert.True(t, (&unixTree{pgid: p.Process.Pid}).alive())
	})

	t.Run("zombie plus live member is alive until the live one is gone", func(t *testing.T) {
		leader := startGroupMember(t, "sh", "-c", "sleep 60 & exit 0") // leader exits, sleep stays in the group
		pgid := leader.Process.Pid
		waitFor(t, "leader to become a zombie", 3*time.Second, func() bool { st, _ := procState(pgid); return st == 'Z' })
		tree := &unixTree{pgid: pgid}
		assert.True(t, tree.alive())

		require.NoError(t, syscall.Kill(-pgid, syscall.SIGKILL))
		_ = leader.Wait()
		// The orphaned sleep is reparented to init/subreaper; wait for it to
		// disappear or be reaped.
		waitFor(t, "group to drain", 5*time.Second, func() bool { return !tree.alive() })
	})

	t.Run("nonexistent group is not alive", func(t *testing.T) {
		assert.False(t, (&unixTree{pgid: 0x3ffffff0}).alive())
	})
}

func TestParseProcStat(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		state byte
		pgrp  int
		ok    bool
	}{
		{"plain", "123 (sleep) S 1 456 789 0 -1 4194304", 'S', 456, true},
		{"zombie", "123 (sh) Z 1 456 789 0 -1 4194304\n", 'Z', 456, true},
		{"comm with space", "123 (a b) R 10 20 30 0", 'R', 20, true},
		{"comm with parens", "123 (a (b) c) D 10 20 30 0", 'D', 20, true},
		{"comm mimicking fields", "123 (x) S 1 2 3) Z 9 8 7", 'Z', 8, true},
		{"comm trailing paren", "123 (x)) X 10 20 30", 'X', 20, true},
		{"no parens", "123 sleep S 1 2 3", 0, 0, false},
		{"empty", "", 0, 0, false},
		{"truncated after comm", "123 (sleep)", 0, 0, false},
		{"truncated after state", "123 (sleep) S", 0, 0, false},
		{"truncated after ppid", "123 (sleep) S 1", 0, 0, false},
		{"non-numeric pgrp", "123 (sleep) S 1 abc 3", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, pgrp, ok := parseProcStat([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.state, state)
				assert.Equal(t, tc.pgrp, pgrp)
			}
		})
	}
}

// PR_SET_CHILD_SUBREAPER: orphaned descendants are re-parented to this
// process, which never reaps them, like a container PID 1 that ignores
// orphans.
func becomeSubreaper(t *testing.T) {
	t.Helper()
	const prSetChildSubreaper = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		t.Skipf("cannot become a subreaper to create an unreaped zombie: %v", errno)
	}
}

func TestStopIgnoresUnreapedZombies(t *testing.T) {
	becomeSubreaper(t)

	// The short-lived sleep exits while its parent (the exec'd long sleep)
	// never reaps it; when TERM kills the parent the zombie is re-parented to
	// this test process, which does not reap it either.
	child, err := startChild([]string{"sh", "-c", `sleep 0.2 & exec sleep 60`}, os.Environ(), 5*time.Second, &treeGuard{})
	require.NoError(t, err)
	time.Sleep(600 * time.Millisecond) // the background sleep has exited

	pgid := child.tree.inner.(*unixTree).pgid // stop() closes the guard
	start := time.Now()
	child.stop()
	elapsed := time.Since(start)

	zombies := zombiesInGroup(pgid)
	if zombies == 0 {
		t.Skip("could not leave an unreaped zombie in the process group")
	}
	assert.Less(t, elapsed, time.Second, "stop must not wait for zombies")
}
