package conjurprovider

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAuthnTokenFile(t *testing.T) {
	t.Run("no-op when CONJUR_AUTHN_TOKEN_FILE is unset", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", "")
		assert.NoError(t, validateAuthnTokenFile(context.Background()))
	})

	t.Run("no error for a readable regular file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
		assert.NoError(t, validateAuthnTokenFile(context.Background()))
	})

	t.Run("path with redundant segments is normalised before access (Snyk CWE-22)", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "token")
		require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
		// Supply a path with a redundant ./subdir/../ that resolves to p after Clean.
		dirty := dir + "/subdir/../token"
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", dirty)
		assert.NoError(t, validateAuthnTokenFile(context.Background()))
	})

	t.Run("configuration error when file does not exist", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "missing")
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "0")
		err := validateAuthnTokenFile(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error")
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), p)
	})

	t.Run("waits for a file that appears later", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "late")
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "1500ms")
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = os.WriteFile(p, []byte("{}"), 0o600)
		}()
		start := time.Now()
		require.NoError(t, validateAuthnTokenFile(context.Background()))
		assert.GreaterOrEqual(t, time.Since(start), 250*time.Millisecond)
	})

	t.Run("fails after the configured wait when file never appears", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "never")
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "400ms")
		start := time.Now()
		err := validateAuthnTokenFile(context.Background())
		el := time.Since(start)
		require.Error(t, err)
		assert.GreaterOrEqual(t, el, 380*time.Millisecond)
		assert.Less(t, el, 1500*time.Millisecond)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), p)
		assert.Contains(t, err.Error(), "400ms")
	})

	t.Run("wait 0 fails immediately", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", filepath.Join(t.TempDir(), "x"))
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "0")
		start := time.Now()
		require.Error(t, validateAuthnTokenFile(context.Background()))
		assert.Less(t, time.Since(start), 90*time.Millisecond)
	})

	t.Run("context cancellation stops the wait", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", filepath.Join(t.TempDir(), "x"))
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "30s")
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		require.Error(t, validateAuthnTokenFile(ctx))
		assert.Less(t, time.Since(start), 1500*time.Millisecond)
	})

	t.Run("malformed or negative wait is a configuration error", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", filepath.Join(t.TempDir(), "x"))
		for _, v := range []string{"abc", "-5s", "10"} {
			t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", v)
			err := validateAuthnTokenFile(context.Background())
			require.Error(t, err, v)
			assert.Contains(t, err.Error(), "SUMMON_AUTHN_TOKEN_FILE_WAIT")
			assert.Contains(t, err.Error(), "configuration error")
		}
	})

	t.Run("default wait is 30s", func(t *testing.T) {
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "")
		d, err := authnTokenFileWait()
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, d)
	})

	t.Run("directory fails immediately despite a long wait", func(t *testing.T) {
		t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "30s")
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", t.TempDir())
		start := time.Now()
		require.Error(t, validateAuthnTokenFile(context.Background()))
		assert.Less(t, time.Since(start), time.Second)
	})

	t.Run("configuration error when path is a directory", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", t.TempDir())
		err := validateAuthnTokenFile(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), "not a regular file")
	})

	t.Run("configuration error when file is unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("permission bits not enforced")
		}
		p := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
		require.NoError(t, os.Chmod(p, 0))
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
		err := validateAuthnTokenFile(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), "not readable")
	})
}

// A missing token file used to make the library poll forever (MaxWaitTime -1)
// on the first retrieval; New must now fail after the configured wait.
func TestNewFailsAfterWaitOnMissingTokenFile(t *testing.T) {
	t.Setenv("CONJUR_APPLIANCE_URL", "http://127.0.0.1:1")
	t.Setenv("CONJUR_ACCOUNT", "acct")
	p := filepath.Join(t.TempDir(), "missing")
	t.Setenv("CONJUR_AUTHN_TOKEN_FILE", p)
	t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "300ms")

	type result struct {
		p   *ConjurProvider
		err error
	}
	done := make(chan result, 1)
	go func() {
		pr, err := New(0)
		if err == nil {
			// Reproduces the old hang: retrieval polls for the file forever.
			_, err = pr.FetchSecret(context.Background(), "var/x")
		}
		done <- result{pr, err}
	}()
	select {
	case r := <-done:
		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
	case <-time.After(2 * time.Second):
		t.Fatal("hung past the configured wait on a nonexistent CONJUR_AUTHN_TOKEN_FILE")
	}
}
