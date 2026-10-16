//go:build !windows && !solaris

package summon

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestartLogsChildThatExitedBeforeStop(t *testing.T) {
	logs := captureLogs(t)
	root := NewTempFactory(t.TempDir())
	r := &refresher{rc: &RefreshConfig{TermGracePeriod: 2 * time.Second}, root: &root}
	child, err := startChild([]string{"sh", "-c", "exit 7"}, os.Environ(), 2*time.Second, &r.tree)
	require.NoError(t, err)
	r.child = child
	r.cycleTF = newCycleTempFactory(&root)
	<-child.exited

	out := r.restart(&fetchedCycle{args: []string{"sleep", "60"}, env: nil}, newCycleTempFactory(&root), true)
	require.False(t, out.done)
	defer r.child.stop()
	assert.Contains(t, logs.String(), "Subprocess exited before restart")
	assert.Contains(t, logs.String(), "exit_status=7")
}

func TestRestartDoesNotWarnForChildWeStop(t *testing.T) {
	logs := captureLogs(t)
	root := NewTempFactory(t.TempDir())
	r := &refresher{rc: &RefreshConfig{TermGracePeriod: 2 * time.Second}, root: &root}
	child, err := startChild([]string{"sleep", "60"}, os.Environ(), 2*time.Second, &r.tree)
	require.NoError(t, err)
	r.child = child
	r.cycleTF = newCycleTempFactory(&root)

	out := r.restart(&fetchedCycle{args: []string{"sleep", "60"}}, newCycleTempFactory(&root), true)
	require.False(t, out.done)
	defer r.child.stop()
	assert.NotContains(t, logs.String(), "exited before restart")
}

func TestStoppedChildStatus(t *testing.T) {
	run := func(script string) error { return exec.Command("sh", "-c", script).Run() }
	code, err := stoppedChildStatus(run("kill -TERM $$"))
	assert.NoError(t, err)
	assert.Equal(t, 0, code, "death by our stop signal is a clean shutdown")
	code, err = stoppedChildStatus(run("exit 3"))
	assert.NoError(t, err)
	assert.Equal(t, 3, code)
	code, err = stoppedChildStatus(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, code)
}
