//go:build !windows && !solaris

package summon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unauthorized() error {
	return fmt.Errorf("authentication failed: %w", &response.ConjurError{Code: 401, Message: "Unauthorized"})
}

// scriptedSecrets answers fetches of app/a from a per-call script: nil entries
// succeed with "v1". Once the script is exhausted the last entry repeats.
type scriptedSecrets struct {
	mu     sync.Mutex
	script []error
	byID   map[string]error // per-id errors, applied once the startup fetch is done
	calls  int
}

func (s *scriptedSecrets) fetch(_ context.Context, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.byID[id]; err != nil && s.calls > 1 {
		return nil, err
	}
	if id != "app/a" {
		return []byte("x"), nil
	}
	i := min(s.calls, len(s.script)-1)
	s.calls++
	if err := s.script[i]; err != nil {
		return nil, err
	}
	return []byte("v1"), nil
}

func (s *scriptedSecrets) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func authRefresh(max int) *RefreshConfig {
	rc := fastRefresh(true)
	rc.MaxAuthFailures = max
	return rc
}

// runScripted starts the loop with the script; the startup fetch is call 0 and
// must succeed.
func runScripted(t *testing.T, s *scriptedSecrets, yaml string, rc *RefreshConfig) *loopHarness {
	t.Helper()
	return startLoopWith(t, newFakeSecrets(nil), yaml, recordScript, rc, func(sc *SubprocessConfig) {
		sc.FetchSecret = s.fetch
	})
}

func (h *loopHarness) expectExit(code int) {
	h.t.Helper()
	r := h.awaitResult(5 * time.Second)
	require.NoError(h.t, r.err)
	assert.Equal(h.t, code, r.code)
}

func TestRefreshTerminatesAfterConsecutiveAuthFailures(t *testing.T) {
	logs := captureLogs(t)
	s := &scriptedSecrets{script: []error{nil, unauthorized()}}
	h := runScripted(t, s, envYAML, authRefresh(3))

	h.expectExit(exitCodeAuthFailed)
	assert.Equal(t, 1+3, s.callCount(), "startup + exactly 3 polls")
	out := logs.String()
	assert.Contains(t, out, "HTTP 401")
	assert.Contains(t, out, "rotated")
	assert.NotContains(t, out, "v1")
}

func TestRefreshNonAuthFailureResetsAuthCount(t *testing.T) {
	// startup, 401, 401, 404-ish, 401, 401, then 401 again => 3 only after the break
	other := errors.New("secret retrieval failed: variable 'app/a' not found or inaccessible")
	s := &scriptedSecrets{script: []error{nil, unauthorized(), unauthorized(), other, unauthorized(), unauthorized(), unauthorized()}}
	h := runScripted(t, s, envYAML, authRefresh(3))

	h.expectExit(exitCodeAuthFailed)
	assert.Equal(t, 7, s.callCount())
}

func TestRefreshSuccessResetsAuthCount(t *testing.T) {
	s := &scriptedSecrets{script: []error{nil, unauthorized(), unauthorized(), nil, unauthorized(), unauthorized(), unauthorized()}}
	h := runScripted(t, s, envYAML, authRefresh(3))

	h.expectExit(exitCodeAuthFailed)
	assert.Equal(t, 7, s.callCount())
}

func TestRefreshAuthLimitZeroDisablesCheck(t *testing.T) {
	s := &scriptedSecrets{script: []error{nil, unauthorized()}}
	h := runScripted(t, s, envYAML, authRefresh(0))

	waitFor(t, "many 401 polls", 5*time.Second, func() bool { return s.callCount() >= 10 })
	select {
	case r := <-h.result:
		t.Fatalf("loop ended with %+v", r)
	default:
	}
	h.finish()
}

func TestRefreshAuthFailureCombinedWithOtherErrorsInOnePoll(t *testing.T) {
	// Secrets are fetched in key order and the first error ends the poll, so
	// a 401 on the first secret counts even though the second would also fail.
	yaml := "A: !var app/a\nB: !var app/b"
	s := &scriptedSecrets{
		script: []error{nil, unauthorized()},
		byID:   map[string]error{"app/b": errors.New("backend unavailable")},
	}
	h := runScripted(t, s, yaml, authRefresh(2))

	h.expectExit(exitCodeAuthFailed)
}

func TestRefreshAuthFailuresStopChildAndLeaveNoLeaks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	pidFile := tmp + "-pid"
	t.Cleanup(func() { os.Remove(pidFile) })
	goroutines := runtime.NumGoroutine()

	s := &scriptedSecrets{script: []error{nil, unauthorized()}}
	yaml := "A: !var app/a\nF: !var:file app/f"
	h := startLoopWith(t, newFakeSecrets(nil), yaml, `echo $$ > `+pidFile+`; echo "$A" >> "$SUMMON_TEST_OUT"; sleep 60`, authRefresh(2),
		func(sc *SubprocessConfig) { sc.FetchSecret = s.fetch })
	h.waitStarts(1)

	h.expectExit(exitCodeAuthFailed)

	b, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)
	waitFor(t, "child gone", 3*time.Second, func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH })

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "temp files left behind")
	waitFor(t, "goroutines to wind down", 3*time.Second, func() bool { return runtime.NumGoroutine() <= goroutines })
}

func TestRefreshOutageLimitFirstWinsOverAuthLimit(t *testing.T) {
	s := &scriptedSecrets{script: []error{nil, unauthorized()}}
	rc := authRefresh(1000)
	rc.MaxOutage = 200 * time.Millisecond
	h := runScripted(t, s, envYAML, rc)

	h.expectExit(exitCodeOutageLimit)
}

func TestRefreshAuthLimitFirstWinsOverOutageLimit(t *testing.T) {
	s := &scriptedSecrets{script: []error{nil, unauthorized()}}
	rc := authRefresh(2)
	rc.MaxOutage = time.Hour
	h := runScripted(t, s, envYAML, rc)

	h.expectExit(exitCodeAuthFailed)
}
