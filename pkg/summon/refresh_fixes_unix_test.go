//go:build !windows && !solaris

package summon

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/conjurprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestartWhileTerminatingMapsOurStopSignalToCleanExit(t *testing.T) {
	root := NewTempFactory(t.TempDir())
	r := &refresher{rc: &RefreshConfig{TermGracePeriod: 2 * time.Second}, root: &root}
	child, err := startChild([]string{"sleep", "60"}, os.Environ(), 2*time.Second, &r.tree)
	require.NoError(t, err)
	r.child = child
	r.cycleTF = newCycleTempFactory(&root)
	r.terminating.Store(true)

	out := r.restart(nil, newCycleTempFactory(&root), true)
	require.True(t, out.done)
	require.NoError(t, out.err)
	assert.Equal(t, 0, out.code)
}

func TestRefreshStopSignalBeforeChildStartLeavesNoChild(t *testing.T) {
	old := beforeStartChildHook
	beforeStartChildHook = func() {
		signalSelf(t, syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond) // let the handler run
	}
	defer func() { beforeStartChildHook = old }()

	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, `echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60`, fastRefresh(true))
	r := h.awaitResult(10 * time.Second)
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, h.starts(), "child must not be left running")
}

var _ = context.Background
var _ = conjurprovider.BuiltinProviderPath
