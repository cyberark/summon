package conjurprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTokenFileProvider builds a real provider that authenticates with a token
// file, against a server that answers every secret request with "s3cret".
func newTokenFileProvider(t *testing.T) (*ConjurProvider, string) {
	t.Helper()
	// Isolate HOME so no ~/.conjurrc is picked up, and clear cert-service-ID so
	// that an ambient CONJUR_AUTHN_CERT_SERVICE_ID (passed from the CI host into
	// the integration test container) does not switch the authn type to "cert"
	// and trigger conjur-api-go's HTTPS-required validation against the plain
	// HTTP test server.
	isolateHome(t)
	t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("s3cret"))
	}))
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte(tokenFor(t)), 0o600))
	t.Setenv("CONJUR_APPLIANCE_URL", srv.URL)
	t.Setenv("CONJUR_ACCOUNT", "acct")
	t.Setenv("CONJUR_AUTHN_TOKEN_FILE", path)
	t.Setenv("SUMMON_AUTHN_TOKEN_FILE_WAIT", "0")
	t.Setenv("SPIFFE_ENDPOINT_SOCKET", "") // prevent ambient CI value from tripping SPIFFE validation
	p, err := New(0)
	require.NoError(t, err)
	// The library caches the token after the first retrieval and from then on
	// only stats the file to detect rotation.
	_, err = p.FetchSecret(context.Background(), "app/a")
	require.NoError(t, err)
	return p, path
}

// fetchNoPanic fails the test instead of crashing when the fetch panics.
func fetchNoPanic(t *testing.T, p *ConjurProvider) (value []byte, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FetchSecret panicked: %v", r)
		}
	}()
	return p.FetchSecret(context.Background(), "app/a")
}

func TestFetchSecretTokenFile(t *testing.T) {
	t.Run("works while the file is present", func(t *testing.T) {
		p, _ := newTokenFileProvider(t)
		v, err := fetchNoPanic(t, p)
		require.NoError(t, err)
		assert.Equal(t, "s3cret", string(v))
	})

	t.Run("deleted after construction is an error, not a panic", func(t *testing.T) {
		p, path := newTokenFileProvider(t)
		require.NoError(t, os.Remove(path))
		v, err := fetchNoPanic(t, p)
		require.Error(t, err)
		assert.Nil(t, v)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), path)
		assert.NotContains(t, err.Error(), "s3cret")
	})

	t.Run("still reported after the process environment was cleared", func(t *testing.T) {
		p, path := newTokenFileProvider(t) // daemon mode unsets CONJUR_* after startup
		require.NoError(t, os.Unsetenv("CONJUR_AUTHN_TOKEN_FILE"))
		require.NoError(t, os.Remove(path))
		_, err := fetchNoPanic(t, p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("recovers once the file is recreated", func(t *testing.T) {
		p, path := newTokenFileProvider(t)
		require.NoError(t, os.Remove(path))
		_, err := fetchNoPanic(t, p)
		require.Error(t, err)

		require.NoError(t, os.WriteFile(path, []byte(tokenFor(t)), 0o600))
		v, err := fetchNoPanic(t, p)
		require.NoError(t, err)
		assert.Equal(t, "s3cret", string(v))
	})

	t.Run("replaced by a directory", func(t *testing.T) {
		p, path := newTokenFileProvider(t)
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Mkdir(path, 0o700))
		_, err := fetchNoPanic(t, p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), "not a regular file")
	})

	t.Run("unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("permission bits not enforced")
		}
		p, path := newTokenFileProvider(t)
		require.NoError(t, os.Chmod(path, 0))
		_, err := fetchNoPanic(t, p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
		assert.Contains(t, err.Error(), "not readable")
	})

	t.Run("replaced atomically keeps working", func(t *testing.T) {
		p, path := newTokenFileProvider(t)
		tmp := path + ".new"
		require.NoError(t, os.WriteFile(tmp, []byte(tokenFor(t)), 0o600))
		require.NoError(t, os.Rename(tmp, path))
		v, err := fetchNoPanic(t, p)
		require.NoError(t, err)
		assert.Equal(t, "s3cret", string(v))
	})
}

func TestFetchSecretWithoutTokenFileIsUnaffected(t *testing.T) {
	p := NewWithClient(realClient(t, "http://127.0.0.1:1", 0))
	_, err := p.FetchSecret(context.Background(), "app/a")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "CONJUR_AUTHN_TOKEN_FILE")
}
