//go:build windows

package summon

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// The tests re-execute the test binary as the supervised process tree:
// parent -> grandchild, both reporting their PIDs to files.

func TestWindowsHelperProcess(t *testing.T) {
	role := os.Getenv("SUMMON_WIN_HELPER")
	if role == "" {
		return // a normal test run
	}
	dir := os.Getenv("SUMMON_WIN_DIR")
	stubborn := os.Getenv("SUMMON_WIN_STUBBORN") == "1"

	writePid := func(name string) {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(strconv.Itoa(os.Getpid())), 0o600)
	}

	interrupts := make(chan os.Signal, 1)
	if stubborn {
		swallowCtrlEvents()
	} else if role == "parent" {
		signal.Notify(interrupts, os.Interrupt)
	}

	switch role {
	case "parent":
		writePid("parent.pid")
		grandchild := exec.Command(os.Args[0], "-test.run=TestWindowsHelperProcess")
		grandchild.Env = append(os.Environ(), "SUMMON_WIN_HELPER=grandchild")
		if err := grandchild.Start(); err != nil {
			os.Exit(1)
		}
		select {
		case <-interrupts:
			_ = os.WriteFile(filepath.Join(dir, "graceful.marker"), nil, 0o600)
			os.Exit(0)
		case <-time.After(time.Minute):
		}
	case "grandchild":
		writePid("grandchild.pid")
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

// swallowCtrlEvents makes the process survive CTRL_BREAK. signal.Ignore does not
// (the Go runtime still exits on CTRL_BREAK), so register a console control
// handler that reports every event as handled; it runs before the runtime's.
func swallowCtrlEvents() {
	handler := syscall.NewCallback(func(uint32) uintptr { return 1 })
	setHandler := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	_, _, _ = setHandler.Call(handler, 1)
}

func startHelperTree(t *testing.T, stubborn bool, grace time.Duration) (*supervisedChild, string) {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(), "SUMMON_WIN_HELPER=parent", "SUMMON_WIN_DIR="+dir)
	if stubborn {
		env = append(env, "SUMMON_WIN_STUBBORN=1")
	}
	child, err := startChild([]string{os.Args[0], "-test.run=TestWindowsHelperProcess"}, env, grace, &treeGuard{})
	require.NoError(t, err)
	t.Cleanup(func() { child.stop() })
	for _, name := range []string{"parent.pid", "grandchild.pid"} {
		waitFor(t, name, 10*time.Second, func() bool {
			b, err := os.ReadFile(filepath.Join(dir, name))
			return err == nil && strings.TrimSpace(string(b)) != ""
		})
	}
	return child, dir
}

func readPid(t *testing.T, dir, name string) uint32 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)
	return uint32(pid)
}

func processGone(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code != 259 // STILL_ACTIVE
}

func TestWindowsStopTree(t *testing.T) {
	t.Run("CTRL_BREAK stops the parent and grandchild without force-kill", func(t *testing.T) {
		if !consoleAttached() {
			t.Skip("needs a console to deliver CTRL_BREAK")
		}
		child, dir := startHelperTree(t, false, 10*time.Second)
		parent, grandchild := readPid(t, dir, "parent.pid"), readPid(t, dir, "grandchild.pid")

		forced := child.stop()

		assert.False(t, forced)
		assert.FileExists(t, filepath.Join(dir, "graceful.marker"))
		waitFor(t, "parent gone", 5*time.Second, func() bool { return processGone(parent) })
		waitFor(t, "grandchild gone", 5*time.Second, func() bool { return processGone(grandchild) })
	})

	t.Run("processes ignoring CTRL_BREAK are force-killed with their descendants", func(t *testing.T) {
		child, dir := startHelperTree(t, true, 500*time.Millisecond)
		parent, grandchild := readPid(t, dir, "parent.pid"), readPid(t, dir, "grandchild.pid")

		forced := child.stop()

		assert.True(t, forced)
		waitFor(t, "parent gone", 5*time.Second, func() bool { return processGone(parent) })
		waitFor(t, "grandchild gone", 5*time.Second, func() bool { return processGone(grandchild) })
	})
}
