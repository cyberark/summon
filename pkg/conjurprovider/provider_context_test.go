package conjurprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	conjurapi "github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenFor returns a syntactically valid, fresh Conjur access token.
func tokenFor(t *testing.T) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"sub": "host/test", "iat": time.Now().Unix()})
	require.NoError(t, err)
	tok, err := json.Marshal(map[string]string{
		"protected": "e30", "payload": base64.RawURLEncoding.EncodeToString(payload), "signature": "c2ln",
	})
	require.NoError(t, err)
	return string(tok)
}

func realClient(t *testing.T, url string, httpTimeout int) *conjurapi.Client {
	t.Helper()
	c, err := conjurapi.NewClientFromToken(conjurapi.Config{
		ApplianceURL: url, Account: "acct", HTTPTimeout: httpTimeout,
	}, tokenFor(t))
	require.NoError(t, err)
	return c
}

func TestFetchSecret_AbortsHungRequest(t *testing.T) {
	aborted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			close(aborted)
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()
	p := NewWithClient(realClient(t, srv.URL, 0))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := p.FetchSecret(ctx, "app/a")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
	select {
	case <-aborted:
	case <-time.After(2 * time.Second):
		t.Fatal("the HTTP request was not aborted")
	}
}

func TestFetchSecret_ReturnsSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "s3cret")
	}))
	defer srv.Close()
	p := NewWithClient(realClient(t, srv.URL, 0))

	v, err := p.FetchSecret(context.Background(), "app/a")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", string(v))
}

func TestFetchSecret_NotFoundMapsLikeLegacyWording(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	p := NewWithClient(realClient(t, srv.URL, 0))

	_, err := p.FetchSecret(context.Background(), "app/a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable 'app/a' not found or inaccessible")
}

func TestFetchSecret_FallsBackForClientsWithoutRequestAPI(t *testing.T) {
	calls := 0
	p := NewWithClient(&mockConjurClient{retrieveSecret: func(string) ([]byte, error) {
		calls++
		return []byte("v"), nil
	}})

	v, err := p.FetchSecret(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, "v", string(v))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.FetchSecret(ctx, "x")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "a done context must not start another request")
}

func TestCapHTTPTimeout(t *testing.T) {
	cases := []struct {
		name    string
		current int
		limit   time.Duration
		want    int
	}{
		{"zero limit keeps default", 0, 0, 0},
		{"rounds up to seconds", 0, 1500 * time.Millisecond, 2},
		{"never raises an operator value", 10, 30 * time.Second, 10},
		{"lowers a longer operator value", 120, 30 * time.Second, 30},
		{"lowers the 60s default", 0, 5 * time.Second, 5},
		{"keeps default when the limit is longer", 0, 90 * time.Second, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := conjurapi.Config{HTTPTimeout: c.current}
			capHTTPTimeout(&cfg, c.limit)
			assert.Equal(t, c.want, cfg.HTTPTimeout)
		})
	}
}
