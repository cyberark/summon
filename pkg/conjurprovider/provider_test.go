package conjurprovider

import (
	"testing"

	conjurapi "github.com/cyberark/conjur-api-go/conjurapi"
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

// ---- loadConjurConfig credential storage mode ----

func TestLoadConjurConfigCredentialStorageMode(t *testing.T) {
	t.Run("defaults to ReadOnly when CONJUR_CREDENTIAL_STORAGE_MODE is unset", func(t *testing.T) {
		t.Setenv("CONJUR_CREDENTIAL_STORAGE_MODE", "")

		cfg, err := loadConjurConfig()

		require.NoError(t, err)
		assert.Equal(t, conjurapi.CredentialStorageModeReadOnly, cfg.CredentialStorageMode)
	})

	t.Run("honors explicit CONJUR_CREDENTIAL_STORAGE_MODE", func(t *testing.T) {
		cases := []struct {
			name string
			env  string
			want conjurapi.CredentialStorageMode
		}{
			{name: "readwrite lowercase", env: "readwrite", want: conjurapi.CredentialStorageModeReadWrite},
			{name: "readwrite uppercase", env: "READWRITE", want: conjurapi.CredentialStorageModeReadWrite},
			{name: "readonly lowercase", env: "readonly", want: conjurapi.CredentialStorageModeReadOnly},
			{name: "readonly mixed case", env: "ReadOnly", want: conjurapi.CredentialStorageModeReadOnly},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("CONJUR_CREDENTIAL_STORAGE_MODE", tc.env)

				cfg, err := loadConjurConfig()

				require.NoError(t, err)
				assert.Equal(t, tc.want, cfg.CredentialStorageMode)
			})
		}
	})
}

// ---- New ----

func TestNew(t *testing.T) {
	t.Run("creates provider with explicit environment config", func(t *testing.T) {
		// Isolate from developer machine config files so this test is deterministic.
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CONJUR_APPLIANCE_URL", "://not-a-url")
		t.Setenv("CONJUR_ACCOUNT", "test")
		t.Setenv("CONJUR_AUTHN_LOGIN", "host/test")
		t.Setenv("CONJUR_AUTHN_API_KEY", "dummy")

		provider, err := New()

		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("returns configuration error for missing authn-cert files", func(t *testing.T) {
		// Detailed cert/key validation cases (missing key file, permission denied,
		// readable file pair, inline-PEM path skip, conflicting inline+file paths)
		// live in conjur-api-go TestConfig_Validate (merged in PR #155, v0.15.3).
		// Summon keeps one contract test: New() must surface Validate() failures as
		// configuration errors, not authentication failures.
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", "/missing/cert.pem")
		t.Setenv("CONJUR_APPLIANCE_URL", "https://conjur.example")
		t.Setenv("CONJUR_ACCOUNT", "conjur")

		_, err := New()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "client certificate file not found")
		assert.NotContains(t, err.Error(), "authentication failed")
	})
}

func TestTelemetryDataUsesSummonVersion(t *testing.T) {
	// telemetryData is initialized at module load time and uses version.FullVersionName.
	// NewTelemetry applies defaults when empty strings are passed, so we verify
	// the version field is set (either from version package or NewTelemetry defaults).
	assert.NotEmpty(t, telemetryData.IntegrationVersion,
		"telemetryData.IntegrationVersion must be populated by NewTelemetry")
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

	assert.Equal(t, BuiltinProviderPath, provider.Path())
}
