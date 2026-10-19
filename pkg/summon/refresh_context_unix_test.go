//go:build !windows && !solaris

package summon

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/conjurprovider"
	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hangScript is an external provider that hangs while $dir/hang exists
// (recording its pid in $dir/pids) and otherwise prints the contents of
// $dir/value. Without an argument (interactive mode) it exits silently, which
// makes Summon fall back to one call per secret.
func hangScript(dir string) string {
	return `#!/bin/sh
if [ -f "` + dir + `/hang" ]; then echo $$ >> "` + dir + `/pids"; exec sleep 60; fi
[ -z "$1" ] && exit 0
cat "` + dir + `/value"
`
}

func pidAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPids(dir string) []int {
	b, _ := os.ReadFile(filepath.Join(dir, "pids"))
	var pids []int
	for _, f := range strings.Fields(string(b)) {
		if p, err := strconv.Atoi(f); err == nil {
			pids = append(pids, p)
		}
	}
	return pids
}

// startExternalLoop runs the loop against a real external provider script.
func startExternalLoop(t *testing.T, dir string, rc *RefreshConfig) *loopHarness {
	t.Helper()
	require.NoError(t, os.Chmod(dir, 0o755))
	provPath := filepath.Join(dir, "prov")
	require.NoError(t, os.WriteFile(provPath, []byte(hangScript(dir)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "value"), []byte("v1"), 0o600))
	ep, err := prov.ResolveDaemon(provPath, 0)
	require.NoError(t, err)
	return startLoopWith(t, newFakeSecrets(nil), envYAML, recordScript, rc, func(sc *SubprocessConfig) {
		sc.Provider = ep.Path()
		sc.FetchSecret = ep.FetchSecret
	})
}

func TestRefreshHungExternalProviderIsKilledAndNextPollRunsFresh(t *testing.T) {
	dir := t.TempDir()
	rc := fastRefresh(true)
	rc.Timeout = 300 * time.Millisecond
	h := startExternalLoop(t, dir, rc)
	h.waitStarts(1)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "hang"), nil, 0o600))
	waitFor(t, "provider hung", 5*time.Second, func() bool { return len(readPids(dir)) > 0 })
	first := readPids(dir)[0]
	waitFor(t, "hung provider killed", 5*time.Second, func() bool { return !pidAlive(first) })

	// The next polls start a fresh fetch (a new provider process) rather than
	// failing with "still running"; once the provider recovers, its value is
	// applied.
	waitFor(t, "fresh fetch", 5*time.Second, func() bool { return len(readPids(dir)) > 1 })
	require.NoError(t, os.Remove(filepath.Join(dir, "hang")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "value"), []byte("v2"), 0o600))
	h.waitStarts(2)
	assert.Equal(t, "v2", h.starts()[1])
	for _, p := range readPids(dir) {
		waitFor(t, "every hung provider killed", 5*time.Second, func() bool { return !pidAlive(p) })
	}
	h.finish()
}

func TestRefreshTimeoutErrorAndSynchronousCleanup(t *testing.T) {
	root := NewTempFactory(t.TempDir())
	config, err := secretsyml.ParseFromString("A: !var:file app/a\nB: !var:file app/b", "", nil)
	require.NoError(t, err)
	ctxSeen := make(chan struct{})
	sc := &SubprocessConfig{
		Provider: conjurprovider.BuiltinProviderPath,
		FetchSecret: func(ctx context.Context, id string) ([]byte, error) {
			if id == "app/a" {
				return []byte("a"), nil // creates a temp file
			}
			close(ctxSeen)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	r := &refresher{
		ctx: context.Background(), sc: sc, config: config, pollConfig: config, root: &root,
		rc: &RefreshConfig{Timeout: 100 * time.Millisecond},
	}

	start := time.Now()
	fetched, tf, err := r.fetchWithTimeout()

	require.Error(t, err)
	assert.Nil(t, fetched)
	assert.Nil(t, tf)
	assert.EqualError(t, err, "fetch exceeded 100ms")
	assert.Less(t, time.Since(start), 2*time.Second)
	<-ctxSeen
	entries, _ := os.ReadDir(root.path)
	assert.Empty(t, entries, "temp files of the aborted fetch are removed before return")
}

func TestRefreshStopSignalAbortsHungFetchPromptly(t *testing.T) {
	t.Run("external provider", func(t *testing.T) {
		dir := t.TempDir()
		rc := fastRefresh(true)
		rc.Timeout = time.Minute
		rc.TermGracePeriod = 30 * time.Second
		h := startExternalLoop(t, dir, rc)
		h.waitStarts(1)
		time.Sleep(200 * time.Millisecond) // let the signal handlers install
		require.NoError(t, os.WriteFile(filepath.Join(dir, "hang"), nil, 0o600))
		waitFor(t, "provider hung", 5*time.Second, func() bool { return len(readPids(dir)) > 0 })
		pid := readPids(dir)[0]

		start := time.Now()
		signalSelf(t, syscall.SIGTERM)
		r := h.awaitResult(5 * time.Second)
		require.NoError(t, r.err)
		assert.Equal(t, 0, r.code)
		assert.Less(t, time.Since(start), 2*time.Second, "shutdown must not wait for the fetch")
		waitFor(t, "provider killed", 2*time.Second, func() bool { return !pidAlive(pid) })
	})

	t.Run("built-in provider", func(t *testing.T) {
		secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
		rc := fastRefresh(true)
		rc.Timeout = time.Minute
		rc.TermGracePeriod = 30 * time.Second
		h := startLoop(t, secrets, envYAML, recordScript, rc)
		h.waitStarts(1)
		time.Sleep(200 * time.Millisecond)
		secrets.block.Store(true)
		calls := secrets.calls.Load()
		waitFor(t, "fetch in flight", 5*time.Second, func() bool { return secrets.calls.Load() > calls })

		start := time.Now()
		signalSelf(t, syscall.SIGTERM)
		r := h.awaitResult(5 * time.Second)
		require.NoError(t, r.err)
		assert.Equal(t, 0, r.code)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}
