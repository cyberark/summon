package conjurprovider

import (
	"testing"

	"github.com/cyberark/conjur-api-go/conjurapi/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConjurClient is a test double for ConjurClient.
type mockConjurClient struct {
	retrieveSecret func(variableID string) ([]byte, error)
}

func (m *mockConjurClient) RetrieveSecret(variableID string) ([]byte, error) {
	return m.retrieveSecret(variableID)
}

// ---- New ----

func TestNew(t *testing.T) {
	t.Run("returns error when credentials are invalid", func(t *testing.T) {
		// New() loads config from env and config files, so we can't control
		// exact validation messages in a mixed environment. Verify it returns
		// a non-nil error.
		t.Setenv("CONJUR_APPLIANCE_URL", "")
		t.Setenv("CONJUR_ACCOUNT", "")
		t.Setenv("CONJUR_AUTHN_LOGIN", "")
		t.Setenv("CONJUR_AUTHN_API_KEY", "")
		t.Setenv("CONJUR_AUTHN_TOKEN_FILE", "")

		_, err := New()

		require.Error(t, err)
	})
}

// ---- BuiltinProviderName constant ----

func TestBuiltinProviderName(t *testing.T) {
	assert.Equal(t, "conjur", BuiltinProviderName,
		"BuiltinProviderName must equal 'conjur' — routing logic depends on this exact string")
}

// ---- FetchSecret ----

func TestFetchSecret(t *testing.T) {
	t.Run("returns secret bytes on success", func(t *testing.T) {
		expected := []byte("s3cret_v@lue")
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return expected, nil
			},
		})

		got, err := provider.FetchSecret("path/to/var")

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("missing variable (404) returns error with variable path but not raw API message", func(t *testing.T) {
		conjurErr := &response.ConjurError{Code: 404, Message: "Not Found"}
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return nil, conjurErr
			},
		})

		_, err := provider.FetchSecret("path/to/missing")

		require.Error(t, err)
		// Variable path is safe to include; raw conjurapi message is not.
		assert.Contains(t, err.Error(), "path/to/missing")
		assert.NotContains(t, err.Error(), "Not Found")
	})

	t.Run("empty value returns success", func(t *testing.T) {
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return []byte{}, nil
			},
		})

		got, err := provider.FetchSecret("path/to/empty")

		require.NoError(t, err)
		assert.Equal(t, []byte{}, got)
	})

	t.Run("auth failure (401) returns error", func(t *testing.T) {
		conjurErr := &response.ConjurError{Code: 401, Message: "Unauthorized"}
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return nil, conjurErr
			},
		})

		_, err := provider.FetchSecret("path/to/var")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "secret retrieval failed")
	})

	t.Run("auth failure (403) returns error", func(t *testing.T) {
		conjurErr := &response.ConjurError{Code: 403, Message: "Forbidden"}
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return nil, conjurErr
			},
		})

		_, err := provider.FetchSecret("path/to/var")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "secret retrieval failed")
	})
}

// ---- Path ----

func TestConjurProviderPath(t *testing.T) {
	provider := NewWithClient(&mockConjurClient{
		retrieveSecret: func(_ string) ([]byte, error) { return nil, nil },
	})

	assert.Equal(t, "conjur (built-in)", provider.Path())
}
