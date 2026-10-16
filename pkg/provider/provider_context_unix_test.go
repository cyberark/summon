//go:build !windows

package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pidGone(pid int) bool {
	return syscall.Kill(pid, 0) == syscall.ESRCH
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	waitUntil(t, func() bool {
		b, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil
	})
	return pid
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestCall_CancelKillsProvider(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	prov := writeProvider(t, `echo $$ > "$1"; exec sleep 60`)
	// The spec path carries the pid file; the script writes its own pid there.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := Call(ctx, prov, pidFile); done <- err }()
	pid := readPID(t, pidFile)

	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Call did not return after cancel")
	}
	waitUntil(t, func() bool { return pidGone(pid) })
}

func TestCall_DeadlineReportsContextError(t *testing.T) {
	prov := writeProvider(t, `exec sleep 60`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Call(ctx, prov, "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestCall_KillsProcessGroupAndReturnsWithGrandchildHoldingPipe(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "gpid")
	// A background grandchild inherits stdout and outlives the script.
	prov := writeProvider(t, `sleep 60 & echo $! > "$1"; wait`)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := Call(ctx, prov, pidFile); done <- err }()
	gpid := readPID(t, pidFile)

	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Wait must return despite the grandchild holding the pipe")
	}
	waitUntil(t, func() bool { return pidGone(gpid) })
}

func TestCall_OwnTimeoutStillReportsProviderTimeout(t *testing.T) {
	t.Setenv(providerTimeoutEnvVar, "100ms")
	prov := writeProvider(t, `exec sleep 60`)
	_, err := Call(context.Background(), prov, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out after")
	assert.False(t, errors.Is(err, context.Canceled))
}

func TestExternalProviderFetchSecret_VerifiesBeforeExec(t *testing.T) {
	prov := writeProvider(t, `echo ok`)
	p, err := ResolveDaemon(prov, 0)
	require.NoError(t, err)

	v, err := p.FetchSecret(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, "ok", string(v))

	require.NoError(t, os.WriteFile(prov, []byte("#!/bin/sh\necho evil\n"), 0o755))
	_, err = p.FetchSecret(context.Background(), "x")
	require.Error(t, err, "replaced binary must be refused")
}

func TestCallInteractiveMode_CancelKillsProvider(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	prov := writeProvider(t, `echo $$ > `+pidFile+`; exec sleep 60`)
	ctx, cancel := context.WithCancel(context.Background())
	secrets := secretsyml.SecretsMap{"A": {Tags: []secretsyml.YamlTag{secretsyml.Var}, Path: "p"}}

	_, errorsCh, cleanup := CallInteractiveMode(ctx, prov, secrets)
	defer cleanup()
	pid := readPID(t, pidFile)

	cancel()
	select {
	case err := <-errorsCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("no error after cancel")
	}
	waitUntil(t, func() bool { return pidGone(pid) })
}
