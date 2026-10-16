//go:build !windows && !solaris

package summon

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/conjurprovider"
	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customPanic struct{ code int }

func captureLogsAt(t *testing.T, level slog.Level) *lockedBuffer {
	t.Helper()
	logs := &lockedBuffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return logs
}

// panickySecrets serves values and panics with val once armed (after the
// startup fetch, which must succeed).
type panickySecrets struct {
	*fakeSecrets
	val atomic.Pointer[any]
}

func newPanickySecrets(values map[string]string) *panickySecrets {
	return &panickySecrets{fakeSecrets: newFakeSecrets(values)}
}

func (p *panickySecrets) arm(v any) { p.val.Store(&v) }
func (p *panickySecrets) disarm()   { p.val.Store(nil) }

func (p *panickySecrets) fetch(ctx context.Context, id string) ([]byte, error) {
	if v := p.val.Load(); v != nil {
		p.calls.Add(1)
		panic(*v)
	}
	return p.fakeSecrets.fetch(ctx, id)
}

func (p *panickySecrets) loop(t *testing.T, rc *RefreshConfig) *loopHarness {
	return startLoopWith(t, newFakeSecrets(nil), envYAML, recordScript, rc, func(sc *SubprocessConfig) {
		sc.FetchSecret = p.fetch
	})
}

func TestRefreshPanickingFetchIsAFailedPoll(t *testing.T) {
	logs := captureLogsAt(t, slog.LevelInfo)
	secrets := newPanickySecrets(map[string]string{"app/a": "v1"})
	h := secrets.loop(t, fastRefresh(true))
	h.waitStarts(1)

	calls := secrets.calls.Load()
	secrets.arm("kaboom")
	waitFor(t, "panicking polls", 5*time.Second, func() bool { return secrets.calls.Load() >= calls+3 })

	select {
	case r := <-h.result:
		t.Fatalf("loop ended on a panicking poll: %+v", r)
	default:
	}
	assert.Len(t, h.starts(), 1, "child must keep running with its last-known-good secrets")

	// Recovery: the next healthy poll is applied.
	secrets.disarm()
	secrets.set("app/a", "v2")
	h.waitStarts(2)
	assert.Equal(t, []string{"v1", "v2"}, h.starts()[:2])
	require.NoError(t, h.finish().err)

	out := logs.String()
	assert.Contains(t, out, "Secrets refresh failed")
	assert.Contains(t, out, "kaboom")
	assert.NotContains(t, out, "goroutine ", "no stack in default logs")
}

func TestRefreshPanicStackOnlyAtDebug(t *testing.T) {
	logs := captureLogsAt(t, slog.LevelDebug)
	secrets := newPanickySecrets(map[string]string{"app/a": "v1"})
	h := secrets.loop(t, fastRefresh(true))
	h.waitStarts(1)
	secrets.arm("kaboom")
	waitFor(t, "stack at debug", 5*time.Second, func() bool { return strings.Contains(logs.String(), "goroutine ") })
	h.finish()
}

func TestRefreshPanickingFetchHonoursOutageLimit(t *testing.T) {
	secrets := newPanickySecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxOutage = 200 * time.Millisecond
	h := secrets.loop(t, rc)
	h.waitStarts(1)
	secrets.arm("kaboom")

	h.expectExit(exitCodeOutageLimit)
	assert.Len(t, h.starts(), 1)
}

func TestRefreshPanicIsNotAnAuthFailure(t *testing.T) {
	secrets := newPanickySecrets(map[string]string{"app/a": "v1"})
	rc := fastRefresh(true)
	rc.MaxAuthFailures = 2
	rc.MaxOutage = 400 * time.Millisecond
	h := secrets.loop(t, rc)
	h.waitStarts(1)
	secrets.arm("kaboom")

	// Many panicking polls pass the auth limit; only the outage limit ends it.
	h.expectExit(exitCodeOutageLimit)
}

func newPanicTestRefresher(t *testing.T, fetch secretFetcher) *refresher {
	t.Helper()
	cfg, err := secretsyml.ParseFromString("A: !var app/a\nB: !var app/b", "", nil)
	require.NoError(t, err)
	root := NewTempFactory(t.TempDir())
	rc := &RefreshConfig{Interval: time.Second, Timeout: time.Second, TermGracePeriod: time.Second}
	return &refresher{
		ctx: context.Background(), rc: rc, root: &root, config: cfg, pollConfig: cfg,
		sc: &SubprocessConfig{
			Args: []string{"true"}, Provider: conjurprovider.BuiltinProviderPath, FetchSecret: fetch, Refresh: rc,
		},
		fileSums: map[string]string{}, fileNonEmpty: map[string]bool{}, now: time.Now,
	}
}

func TestPollPanicIsOrdinaryFailedPoll(t *testing.T) {
	logs := captureLogsAt(t, slog.LevelInfo)
	fetch := func(_ context.Context, id string) ([]byte, error) {
		if id == "app/b" {
			panic("boom")
		}
		return []byte("s3cr3t-value"), nil
	}
	r := newPanicTestRefresher(t, fetch)
	r.authFailures = 1

	out := r.poll()

	assert.True(t, out.failed)
	assert.False(t, out.done)
	assert.Equal(t, uint(1), r.failures, "counts for the retry backoff")
	assert.Equal(t, 0, r.authFailures, "not an authentication failure")
	assert.False(t, r.outageStart.IsZero(), "starts the outage clock")
	assert.Contains(t, logs.String(), "Secrets refresh failed")
	assert.NotContains(t, logs.String(), "s3cr3t-value")
	entries, err := os.ReadDir(r.root.path)
	require.NoError(t, err)
	assert.Empty(t, entries, "temp files of the failed poll are removed")
}

func TestPollPanicValueTypes(t *testing.T) {
	var nilMap map[string]int
	idx := 5
	cases := []struct {
		name string
		val  func() any
		want []string
	}{
		{"string", func() any { return "plain text" }, []string{"string", "plain text"}},
		{"error", func() any { return errors.New("an error value") }, []string{"*errors.errorString", "an error value"}},
		{"runtime error", func() any { nilMap["x"] = 1; return nil }, []string{"runtime.plainError", "assignment to entry in nil map"}},
		{"index out of range", func() any { return []int{1}[idx] }, []string{"runtime.boundsError", "index out of range"}},
		{"custom type", func() any { return customPanic{code: 7} }, []string{"summon.customPanic", "{7}"}},
		{"pointer to custom type", func() any { return &customPanic{code: 8} }, []string{"*summon.customPanic"}},
		{"nil", func() any { return nil }, []string{"runtime.PanicNilError"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogsAt(t, slog.LevelInfo)
			r := newPanicTestRefresher(t, func(context.Context, string) ([]byte, error) { panic(tc.val()) })

			out := r.poll()

			assert.True(t, out.failed)
			assert.False(t, out.done)
			for _, w := range tc.want {
				assert.Contains(t, logs.String(), w)
			}
		})
	}
}

func TestPanicErrorDoesNotWrapPanicValue(t *testing.T) {
	// A panic carrying an HTTP 401 must not be classified as an auth failure.
	r := newPanicTestRefresher(t, func(context.Context, string) ([]byte, error) { panic(unauthorized()) })
	r.rc.MaxAuthFailures = 1
	out := r.poll()
	assert.True(t, out.failed)
	assert.False(t, out.done)
	assert.Equal(t, 0, r.authFailures)
}

func TestFallbackFetchPanicInGoroutineBecomesResultError(t *testing.T) {
	// The per-secret fallback runs FetchSecret in goroutines of its own; a
	// panic there would crash the process whatever the poll recovers.
	r := newPanicTestRefresher(t, func(context.Context, string) ([]byte, error) { panic("boom") })
	tf := newCycleTempFactory(r.root)
	results := nonInteractiveProviderFallback(context.Background(), r.pollConfig.EnvSecrets, r.cycleConfig(false), tf)
	require.Len(t, results, 2)
	for _, res := range results {
		require.Error(t, res.Error)
		assert.Contains(t, res.Error.Error(), "boom")
	}
}

func TestRefreshLoopPanicStopsChild(t *testing.T) {
	logs := captureLogsAt(t, slog.LevelInfo)
	tmp := t.TempDir()
	pidFile := tmp + "-pid"
	t.Cleanup(func() { os.Remove(pidFile) })

	old := refreshClock
	var armed atomic.Bool
	refreshClock = func() time.Time {
		if armed.Load() {
			panic("clock exploded")
		}
		return time.Now()
	}
	t.Cleanup(func() { refreshClock = old })

	secrets := newFakeSecrets(map[string]string{"app/a": "v1"})
	h := startLoop(t, secrets, envYAML, `echo $$ > `+pidFile+`; echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60`, fastRefresh(true))
	h.waitStarts(1)
	b, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)

	armed.Store(true) // the next poll panics outside the fetch step

	r := h.awaitResult(5 * time.Second)
	require.Error(t, r.err)
	assert.Contains(t, r.err.Error(), "clock exploded")
	waitFor(t, "child gone", 3*time.Second, func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH })
	assert.NotContains(t, logs.String(), "goroutine ")
}
