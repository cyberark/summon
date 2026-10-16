//go:build !windows && !solaris

package summon

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/conjurprovider"
	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSecrets is a controllable secrets backend for FetchSecret.
type fakeSecrets struct {
	mu      sync.Mutex
	values  map[string]string
	fail    atomic.Bool
	failN   atomic.Int32 // number of upcoming calls that fail
	failID  map[string]bool
	failErr error // returned instead of the generic failure when set
	block   atomic.Bool
	calls   atomic.Int32
}

func newFakeSecrets(values map[string]string) *fakeSecrets {
	return &fakeSecrets{values: values}
}

func (f *fakeSecrets) set(id, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[id] = value
}

// failFor makes fetches of one secret id fail (or succeed again).
func (f *fakeSecrets) failFor(id string, fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failID == nil {
		f.failID = map[string]bool{}
	}
	f.failID[id] = fail
}

// fetch blocks (when f.block is set) until ctx is done.
func (f *fakeSecrets) fetch(ctx context.Context, id string) ([]byte, error) {
	f.calls.Add(1)
	f.mu.Lock()
	idFails := f.failID[id]
	f.mu.Unlock()
	if idFails {
		return nil, errors.New("backend unavailable")
	}
	if f.block.Load() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	if f.fail.Load() || f.failN.Add(-1) >= 0 {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failErr != nil {
			return nil, f.failErr
		}
		return nil, errors.New("backend unavailable")
	}
	f.failN.Store(0)
	f.mu.Lock()
	defer f.mu.Unlock()
	return []byte(f.values[id]), nil
}

type loopResult struct {
	code int
	err  error
}

type loopHarness struct {
	t       *testing.T
	cancel  context.CancelFunc
	result  chan loopResult
	outFile string
}

// startLoop runs a refreshing subprocess whose script appends $A to outFile on
// every (re)start and then sleeps.
func startLoop(t *testing.T, secrets *fakeSecrets, yaml string, script string, rc *RefreshConfig) *loopHarness {
	t.Helper()
	return startLoopWith(t, secrets, yaml, script, rc, nil)
}

// startLoopWith is startLoop with a hook to adjust the SubprocessConfig.
func startLoopWith(t *testing.T, secrets *fakeSecrets, yaml string, script string, rc *RefreshConfig, adjust func(*SubprocessConfig)) *loopHarness {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	t.Setenv("SUMMON_TEST_OUT", out)
	oldRetry := retryInterval
	retryInterval = 20 * time.Millisecond
	t.Cleanup(func() { retryInterval = oldRetry })

	ctx, cancel := context.WithCancel(context.Background())
	h := &loopHarness{t: t, cancel: cancel, result: make(chan loopResult, 1), outFile: out}
	t.Cleanup(cancel)
	go func() {
		sc := &SubprocessConfig{
			Args:        []string{"bash", "-c", script},
			YamlInline:  strings.ReplaceAll(yaml, "{{dir}}", dir),
			Provider:    conjurprovider.BuiltinProviderPath,
			FetchSecret: secrets.fetch,
			Refresh:     rc,
		}
		if adjust != nil {
			adjust(sc)
		}
		code, err := runSubprocess(ctx, sc)
		h.result <- loopResult{code, err}
	}()
	return h
}

func (h *loopHarness) starts() []string {
	b, _ := os.ReadFile(h.outFile)
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func (h *loopHarness) waitStarts(n int) {
	h.t.Helper()
	waitFor(h.t, "subprocess starts", 10*time.Second, func() bool { return len(h.starts()) >= n })
}

func (h *loopHarness) finish() loopResult {
	h.t.Helper()
	h.cancel()
	select {
	case r := <-h.result:
		return r
	case <-time.After(10 * time.Second):
		h.t.Fatal("refresh loop did not return")
		return loopResult{}
	}
}

func fastRefresh(restart bool) *RefreshConfig {
	return &RefreshConfig{
		Interval:          40 * time.Millisecond,
		Timeout:           500 * time.Millisecond,
		TermGracePeriod:   3 * time.Second,
		RestartOnRotation: restart,
	}
}

const (
	envYAML       = "A: !var app/a"
	recordScript  = `echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60`
	fileOnlyYAML  = "summon.files:\n  - path: {{dir}}/secret.txt\n    format: template\n    template: '{{ secret \"F\" }}'\n    secrets:\n      F: !var app/f\n"
	envAndFileYML = envYAML + "\n" + fileOnlyYAML
)

func TestRefreshRestartsOnEnvChange(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, recordScript, fastRefresh(true))
	h.waitStarts(1)

	secrets.set("app/a", "v2")
	h.waitStarts(2)

	assert.Equal(t, []string{"v1", "v2"}, h.starts()[:2])
	r := h.finish()
	require.NoError(t, r.err)
}

func TestRefreshDoesNotRestartWithoutChange(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, recordScript, fastRefresh(true))
	h.waitStarts(1)

	polls := secrets.calls.Load()
	waitFor(t, "several polls", 5*time.Second, func() bool { return secrets.calls.Load() >= polls+3 })

	assert.Len(t, h.starts(), 1)
	h.finish()
}

func TestRefreshRewritesFilesWithoutRestartWhenDisabled(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/f": "f1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(false))
	h.waitStarts(1)
	secretPath := filepath.Join(filepath.Dir(h.outFile), "secret.txt")
	content, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	assert.Equal(t, "f1", string(content))

	secrets.set("app/f", "f2")
	secrets.set("app/a", "v2") // env change must not restart either
	waitFor(t, "file rewrite", 5*time.Second, func() bool {
		b, _ := os.ReadFile(secretPath)
		return string(b) == "f2"
	})

	assert.Len(t, h.starts(), 1)
	h.finish()
}

func TestRefreshRestartsOnFileChangeWhenEnabled(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/f": "f1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(true))
	h.waitStarts(1)

	secrets.set("app/f", "f2")
	h.waitStarts(2)

	h.finish()
}

func TestRefreshPropagatesChildExit(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, `sleep 0.3; exit 3`, fastRefresh(true))

	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 3, r.code)
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not return after child exit")
	}
}

func TestRefreshTerminatesAfterMaxOutage(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxOutage = 200 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	secrets.fail.Store(true)

	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 2, r.code)
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not terminate after max failures")
	}
}

func TestRefreshFetchTimeoutCountsAsFailure(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.Timeout = 100 * time.Millisecond
	rc.MaxOutage = 40 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	secrets.block.Store(true)

	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 2, r.code)
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not terminate after a timed-out fetch")
	}
}

func TestRefreshOutageShorterThanLimitDoesNotTerminate(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxOutage = 2 * time.Second
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	// Many failed polls and retries, well below the wall-clock limit.
	before := secrets.calls.Load()
	secrets.fail.Store(true)
	waitFor(t, "ten failed polls", 5*time.Second, func() bool { return secrets.calls.Load() >= before+10 })
	secrets.fail.Store(false)
	waitPolls(t, secrets, 3)

	select {
	case r := <-h.result:
		t.Fatalf("loop exited early with code %d, err %v", r.code, r.err)
	default:
	}
	h.finish()
}

func TestRefreshSingleFailedPollBeyondLimitTerminates(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.Timeout = 150 * time.Millisecond
	rc.MaxOutage = 100 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	before := secrets.calls.Load()
	secrets.block.Store(true) // the first failed poll itself outlasts the limit
	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 2, r.code)
		assert.Equal(t, int32(1), secrets.calls.Load()-before, "one poll sufficed")
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not terminate")
	}
}

func TestRefreshSuccessResetsOutageClock(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxOutage = 600 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	// Three outages of ~300ms each: 900ms in total, but never 600ms in a row.
	for i := 0; i < 3; i++ {
		secrets.fail.Store(true)
		time.Sleep(300 * time.Millisecond)
		secrets.fail.Store(false)
		waitPolls(t, secrets, 3)
	}

	select {
	case r := <-h.result:
		t.Fatalf("loop exited early with code %d, err %v", r.code, r.err)
	default:
	}
	h.finish()
}

func TestRefreshHungFetchIsBoundedByWallClock(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.Timeout = 50 * time.Millisecond
	rc.MaxOutage = 600 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	start := time.Now()
	secrets.block.Store(true) // the fetch hangs until its timeout cancels it

	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 2, r.code)
		assert.GreaterOrEqual(t, time.Since(start), 600*time.Millisecond, "ended by the clock, not by tick count")
	case <-time.After(4 * time.Second):
		t.Fatal("hung fetch did not end the run")
	}
}

func TestRefreshRequiresNoChangeOnSeed(t *testing.T) {
	// The first poll diffs against the startup fetch, never against empty
	// state: an unchanged secret must not cause a restart.
	secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/f": "f1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(true))
	h.waitStarts(1)
	polls := secrets.calls.Load()
	waitFor(t, "first polls", 5*time.Second, func() bool { return secrets.calls.Load() >= polls+4 })

	assert.Len(t, h.starts(), 1)
	h.finish()
}

// lockedBuffer is a goroutine-safe log sink.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRefreshNeverLogsSecretValues(t *testing.T) {
	logs := &lockedBuffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(original)

	secrets := newFakeSecrets(map[string]string{"app/a": "SECRET-env-1", "app/f": "SECRET-file-1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(true))
	h.waitStarts(1)
	secrets.set("app/a", "SECRET-env-2")
	secrets.set("app/f", "SECRET-file-2")
	h.waitStarts(2)
	secrets.failN.Store(2) // exercise the failure paths too
	calls := secrets.calls.Load()
	waitFor(t, "failed polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+4 })
	h.finish()

	assert.NotEmpty(t, logs.String())
	assert.NotContains(t, logs.String(), "SECRET-")
}

func TestRefreshShutdownPropagatesChildExitCode(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML,
		`trap 'exit 7' TERM; echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60 & wait`, fastRefresh(true))
	h.waitStarts(1)
	time.Sleep(200 * time.Millisecond) // let the trap install

	r := h.finish()
	require.NoError(t, r.err)
	assert.Equal(t, 7, r.code)
}

func TestRefreshShutdownAfterSignalDeathIsClean(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, recordScript, fastRefresh(true))
	h.waitStarts(1)

	r := h.finish()
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
}

func TestRefreshBacksOffExponentiallyOnFailure(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.Interval = 10 * time.Second
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	// retryInterval is 20ms in tests: with doubling, 5 failures need >= ~620ms
	// (20+40+80+160+320 minus jitter), far more than a constant 20ms retry.
	secrets.fail.Store(true)
	before := secrets.calls.Load()
	waitFor(t, "first failed poll", 15*time.Second, func() bool { return secrets.calls.Load() > before })
	start := time.Now()
	waitFor(t, "five more failed polls", 15*time.Second, func() bool { return secrets.calls.Load() >= before+6 })
	assert.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond)
	h.finish()
}

func TestRefreshIgnoredAtStartupKeepsStartupState(t *testing.T) {
	const yaml = "A: !var app/a\nB: !var app/b"
	ignoreB := func(sc *SubprocessConfig) { sc.Ignores = []string{"B"} }
	const script = `echo "$A:$B" >> "$SUMMON_TEST_OUT"; sleep 60`

	t.Run("secret ignored at startup never fails polls nor blocks other changes", func(t *testing.T) {
		secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/b": "b1"})
		secrets.failFor("app/b", true)
		rc := fastRefresh(true)
		rc.MaxOutage = 100 * time.Millisecond
		h := startLoopWith(t, secrets, yaml, script, rc, ignoreB)
		h.waitStarts(1)

		calls := secrets.calls.Load()
		waitFor(t, "polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+4 })
		secrets.set("app/a", "v2")
		h.waitStarts(2)
		assert.Equal(t, []string{"v1:", "v2:"}, h.starts())

		// Recovery of the ignored secret is not picked up either.
		secrets.failFor("app/b", false)
		calls = secrets.calls.Load()
		waitFor(t, "polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+4 })
		assert.Len(t, h.starts(), 2)

		select {
		case r := <-h.result:
			t.Fatalf("loop ended unexpectedly: %+v", r)
		default:
		}
		h.finish()
	})

	t.Run("later failure of a startup-fetched secret is a failed poll", func(t *testing.T) {
		secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/b": "b1"})
		rc := fastRefresh(true)
		rc.MaxOutage = 100 * time.Millisecond
		h := startLoopWith(t, secrets, yaml, recordScript, rc, ignoreB)
		h.waitStarts(1)

		secrets.failFor("app/b", true)

		select {
		case r := <-h.result:
			require.NoError(t, r.err)
			assert.Equal(t, 2, r.code)
		case <-time.After(10 * time.Second):
			t.Fatal("failure of a startup-fetched secret was not counted as a failed poll")
		}
		assert.Len(t, h.starts(), 1, "no restart on a failed poll")
	})
}

func TestRefreshFailureLogsOmitProviderStderrUnlessDebug(t *testing.T) {
	providerErr := &prov.CallError{Err: errors.New("exit status 1"), Stderr: "leaked-stderr-token"}

	run := func(t *testing.T, level slog.Level) string {
		logs := &lockedBuffer{}
		original := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: level})))
		defer slog.SetDefault(original)

		secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
		h := startLoop(t, secrets, envYAML, recordScript, fastRefresh(true))
		h.waitStarts(1)
		secrets.mu.Lock()
		secrets.failErr = providerErr
		secrets.mu.Unlock()
		calls := secrets.calls.Load()
		secrets.failN.Store(2)
		waitFor(t, "failed polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+3 })
		h.finish()
		return logs.String()
	}

	t.Run("info", func(t *testing.T) {
		out := run(t, slog.LevelInfo)
		assert.Contains(t, out, "Secrets refresh failed")
		assert.Contains(t, out, "exit status 1")
		assert.NotContains(t, out, "leaked-stderr-token")
	})

	t.Run("debug keeps stderr", func(t *testing.T) {
		assert.Contains(t, run(t, slog.LevelDebug), "leaked-stderr-token")
	})
}

// signalSelf delivers sig to the test process, where forwardSignals handles it.
func signalSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	require.NoError(t, syscall.Kill(os.Getpid(), sig))
}

func (h *loopHarness) awaitResult(within time.Duration) loopResult {
	h.t.Helper()
	select {
	case r := <-h.result:
		return r
	case <-time.After(within):
		h.t.Fatal("refresh loop did not return in time")
		return loopResult{}
	}
}

func TestRefreshSIGHUPIsOnlyForwarded(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML,
		`trap 'echo hup >> "$SUMMON_TEST_OUT.hup"' HUP; echo "$A" >> "$SUMMON_TEST_OUT"; while :; do sleep 0.05; done`, fastRefresh(true))
	h.waitStarts(1)
	time.Sleep(200 * time.Millisecond) // let the trap install

	signalSelf(t, syscall.SIGHUP)
	waitFor(t, "child to get SIGHUP", 5*time.Second, func() bool {
		_, err := os.Stat(h.outFile + ".hup")
		return err == nil
	})

	// Summon must still restart the child on rotation.
	secrets.set("app/a", "v2")
	h.waitStarts(2)

	// And a later stop signal is a graceful stop, not an immediate force-kill.
	r := h.finish()
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
}

func TestRefreshStopSignalAppliesGracePeriod(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.TermGracePeriod = 500 * time.Millisecond
	h := startLoop(t, secrets, envYAML,
		`trap '' TERM; echo "$A" >> "$SUMMON_TEST_OUT"; while :; do sleep 0.05; done`, rc)
	h.waitStarts(1)
	time.Sleep(200 * time.Millisecond) // let the trap install

	start := time.Now()
	signalSelf(t, syscall.SIGTERM)
	r := h.awaitResult(5 * time.Second)
	assert.GreaterOrEqual(t, time.Since(start), 400*time.Millisecond, "child must get the grace period")
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
}

func TestRefreshStopSignalExitsWithChildCodeWithinGrace(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML,
		`trap 'exit 7' TERM; echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60 & wait`, fastRefresh(true))
	h.waitStarts(1)
	time.Sleep(200 * time.Millisecond)

	signalSelf(t, syscall.SIGTERM)
	r := h.awaitResult(5 * time.Second)
	require.NoError(t, r.err)
	assert.Equal(t, 7, r.code)
}

func TestRefreshSecondStopSignalForceKillsImmediately(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.TermGracePeriod = time.Minute
	h := startLoop(t, secrets, envYAML,
		`trap '' TERM; echo "$A" >> "$SUMMON_TEST_OUT"; while :; do sleep 0.05; done`, rc)
	h.waitStarts(1)
	time.Sleep(200 * time.Millisecond)

	signalSelf(t, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	signalSelf(t, syscall.SIGTERM)
	r := h.awaitResult(5 * time.Second)
	require.NoError(t, r.err)
	assert.Equal(t, 0, r.code)
}

// waitPolls waits for n further fetches (each poll fetches every secret once).
func waitPolls(t *testing.T, secrets *fakeSecrets, n int32) {
	t.Helper()
	calls := secrets.calls.Load()
	waitFor(t, "polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+n })
}

func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	logs := &lockedBuffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return logs
}

func TestRefreshEmptyEnvValueKeepsLastKnownGood(t *testing.T) {
	logs := captureLogs(t)
	secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/f": "f1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(true))
	h.waitStarts(1)
	secretPath := filepath.Join(filepath.Dir(h.outFile), "secret.txt")

	// Env secret goes empty while the file secret rotates: whole poll fails.
	secrets.set("app/a", "")
	secrets.set("app/f", "f2")
	waitPolls(t, secrets, 6)
	assert.Len(t, h.starts(), 1)
	content, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	assert.Equal(t, "f1", string(content))
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "empty value")
	assert.Contains(t, logs.String(), "A")

	// Recovery with a non-empty value applies everything.
	secrets.set("app/a", "v2")
	h.waitStarts(2)
	assert.Equal(t, "v2", h.starts()[1])
	content, err = os.ReadFile(secretPath)
	require.NoError(t, err)
	assert.Equal(t, "f2", string(content))
	h.finish()
}

func TestRefreshEmptyFileValueKeepsLastKnownGood(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1", "app/f": "f1"})
	h := startLoop(t, secrets, envAndFileYML, recordScript, fastRefresh(true))
	h.waitStarts(1)
	secretPath := filepath.Join(filepath.Dir(h.outFile), "secret.txt")

	secrets.set("app/f", "")
	secrets.set("app/a", "v2")
	waitPolls(t, secrets, 6)
	assert.Len(t, h.starts(), 1)
	content, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	assert.Equal(t, "f1", string(content))

	secrets.set("app/f", "f2")
	h.waitStarts(2)
	h.finish()
}

func TestRefreshEmptyValueCountsTowardOutage(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxOutage = 100 * time.Millisecond
	h := startLoop(t, secrets, envYAML, recordScript, rc)
	h.waitStarts(1)

	secrets.set("app/a", "")
	select {
	case r := <-h.result:
		require.NoError(t, r.err)
		assert.Equal(t, 2, r.code)
	case <-time.After(10 * time.Second):
		t.Fatal("empty values did not count as failed polls")
	}
}

func TestRefreshEmptyAtStartupStaysValid(t *testing.T) {
	secrets := newFakeSecrets(map[string]string{"app/a": "", "app/f": ""})
	rc := fastRefresh(true)
	rc.MaxOutage = 100 * time.Millisecond
	h := startLoop(t, secrets, envAndFileYML, `echo "start:$A" >> "$SUMMON_TEST_OUT"; sleep 60`, rc)
	h.waitStarts(1)

	// empty -> empty: no change, no failure.
	waitPolls(t, secrets, 8)
	select {
	case r := <-h.result:
		t.Fatalf("loop exited early with code %d, err %v", r.code, r.err)
	default:
	}
	assert.Len(t, h.starts(), 1)

	// empty -> non-empty is a normal change.
	secrets.set("app/a", "v1")
	h.waitStarts(2)
	// ... after which empty is a regression again.
	secrets.set("app/a", "")
	select {
	case r := <-h.result:
		assert.Equal(t, 2, r.code)
	case <-time.After(10 * time.Second):
		t.Fatal("empty after non-empty did not fail")
	}
}
