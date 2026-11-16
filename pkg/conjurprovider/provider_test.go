package conjurprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

		cfg, err := loadConjurConfig(0)

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

				cfg, err := loadConjurConfig(0)

				require.NoError(t, err)
				assert.Equal(t, tc.want, cfg.CredentialStorageMode)
			})
		}
	})
}

// ---- validateNetrcPath ----

func TestValidateNetrcPath(t *testing.T) {
	t.Run("no-op when CONJUR_NETRC_PATH is unset", func(t *testing.T) {
		t.Setenv("CONJUR_NETRC_PATH", "")
		assert.NoError(t, validateNetrcPath())
	})

	t.Run("no error when path points to a regular file", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "netrc-*")
		require.NoError(t, err)
		f.Close()
		t.Setenv("CONJUR_NETRC_PATH", f.Name())
		assert.NoError(t, validateNetrcPath())
	})

	t.Run("configuration error when path is a directory", func(t *testing.T) {
		t.Setenv("CONJUR_NETRC_PATH", t.TempDir())
		err := validateNetrcPath()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error")
		assert.Contains(t, err.Error(), "not a regular file")
	})

	t.Run("configuration error when path does not exist", func(t *testing.T) {
		t.Setenv("CONJUR_NETRC_PATH", "/nonexistent/path/to/netrc")
		err := validateNetrcPath()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error")
		assert.Contains(t, err.Error(), "not accessible")
	})
}

// ---- New ----

func TestNew(t *testing.T) {
	t.Run("creates provider with explicit environment config", func(t *testing.T) {
		// Isolate from developer machine config files so this test is deterministic.
		// Also clear any SPIFFE/cert auth env vars that leak from the integration
		// test environment (e.g. CONJUR_AUTHN_CERT_SERVICE_ID exported by
		// setup-spiffe.sh) to prevent conjur-api-go from triggering cert auth and
		// failing the HTTPS URL validation on the intentionally-invalid test URL.
		isolateHome(t)
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")
		t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		t.Setenv("CONJUR_APPLIANCE_URL", "://not-a-url")
		t.Setenv("CONJUR_ACCOUNT", "test")
		t.Setenv("CONJUR_AUTHN_LOGIN", "host/test")
		t.Setenv("CONJUR_AUTHN_API_KEY", "dummy")

		provider, err := New(0)

		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("returns configuration error when SPIFFE socket set without service ID", func(t *testing.T) {
		isolateHome(t)
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/spiffe.sock")
		t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "")
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")
		t.Setenv("CONJUR_APPLIANCE_URL", "https://conjur.example")
		t.Setenv("CONJUR_ACCOUNT", "conjur")

		_, err := New(0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "SPIFFE_ENDPOINT_SOCKET is set but no authenticator service ID is configured")
	})

	t.Run("returns configuration error for missing authn-cert files", func(t *testing.T) {
		// Detailed cert/key validation cases (missing key file, permission denied,
		// readable file pair, inline-PEM path skip, conflicting inline+file paths)
		// live in conjur-api-go TestConfig_Validate (merged in PR #155, v0.15.3).
		// Summon keeps one contract test: New(0) must surface Validate() failures as
		// configuration errors, not authentication failures.
		isolateHome(t)
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", "/missing/cert.pem")
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "/missing/key.pem")
		t.Setenv("CONJUR_APPLIANCE_URL", "https://conjur.example")
		t.Setenv("CONJUR_ACCOUNT", "conjur")

		_, err := New(0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		// The error message varies between conjur-api-go editions; assert that the
		// cert file path is mentioned rather than hard-coding a specific phrasing.
		assert.Contains(t, err.Error(), "/missing/cert.pem")
		assert.NotContains(t, err.Error(), "authentication failed")
	})
}

// ---- validateSpiffeConfig ----

func TestValidateSpiffeConfig_EmptySocketNamesVariable(t *testing.T) {
	t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
	t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "swa-jwt")
	t.Setenv("CONJUR_AUTHN_JWT_TOKEN", "")
	t.Setenv("JWT_TOKEN_PATH", "")
	err := validateSpiffeConfig()
	assert.ErrorContains(t, err, "SPIFFE_ENDPOINT_SOCKET is set but empty")

	t.Setenv("CONJUR_AUTHN_JWT_TOKEN", "static")
	assert.NoError(t, validateSpiffeConfig())
}

func TestNormalizeSpiffeEndpointEnv(t *testing.T) {
	t.Run("bare absolute path becomes a unix URL", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "/run/spire/agent.sock")
		assert.NoError(t, normalizeSpiffeEndpointEnv())
		assert.Equal(t, "unix:///run/spire/agent.sock", os.Getenv("SPIFFE_ENDPOINT_SOCKET"))
	})

	t.Run("value with a scheme is unchanged", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///run/spire/agent.sock")
		assert.NoError(t, normalizeSpiffeEndpointEnv())
		assert.Equal(t, "unix:///run/spire/agent.sock", os.Getenv("SPIFFE_ENDPOINT_SOCKET"))
	})

	t.Run("empty value is left alone", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		assert.NoError(t, normalizeSpiffeEndpointEnv())
		assert.Equal(t, "", os.Getenv("SPIFFE_ENDPOINT_SOCKET"))
	})

	t.Run("relative path is a configuration error", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "relative/agent.sock")
		assert.ErrorContains(t, normalizeSpiffeEndpointEnv(), "not an absolute path")
	})
}

func TestValidateSpiffeConfig_DoesNotModifyEnvironment(t *testing.T) {
	t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "swa-jwt")
	t.Setenv("SPIFFE_ENDPOINT_SOCKET", "/run/spire/agent.sock")
	assert.NoError(t, validateSpiffeConfig())
	assert.Equal(t, "/run/spire/agent.sock", os.Getenv("SPIFFE_ENDPOINT_SOCKET"))
}

func TestValidateSpiffeConfig(t *testing.T) {
	t.Run("no socket — passes", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		assert.NoError(t, validateSpiffeConfig())
	})

	t.Run("socket with JWT service ID — passes", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/spiffe.sock")
		t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "swa-e2e-jwt")
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")
		assert.NoError(t, validateSpiffeConfig())
	})

	t.Run("socket with cert service ID — passes", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/spiffe.sock")
		t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "")
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "swa-e2e-cert")
		assert.NoError(t, validateSpiffeConfig())
	})

	t.Run("socket with no service ID — config error", func(t *testing.T) {
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/spiffe.sock")
		t.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", "")
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")

		err := validateSpiffeConfig()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "SPIFFE_ENDPOINT_SOCKET is set but no authenticator service ID is configured")
	})
}

// ---- validateCertFiles ----

func TestValidateCertFiles(t *testing.T) {
	t.Run("no cert service ID — passes", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "")
		assert.NoError(t, validateCertFiles())
	})

	t.Run("cert service ID with SPIFFE socket — skips file check", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "unix:///tmp/spiffe.sock")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", "")
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "")
		assert.NoError(t, validateCertFiles())
	})

	t.Run("cert file env var unset — config error", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", "")
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "")

		err := validateCertFiles()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_CERT_FILE is not configured")
	})

	t.Run("key file env var unset — config error", func(t *testing.T) {
		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0600))

		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", certFile)
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "")

		err := validateCertFiles()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "CONJUR_AUTHN_CERT_KEY_FILE is not configured")
	})

	t.Run("cert file missing — config error", func(t *testing.T) {
		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", "/nonexistent/cert.pem")
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "/nonexistent/key.pem")

		err := validateCertFiles()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "cannot read certificate file")
	})

	t.Run("key file missing — config error", func(t *testing.T) {
		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0600))

		t.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "acme-vm")
		t.Setenv("SPIFFE_ENDPOINT_SOCKET", "")
		t.Setenv("CONJUR_AUTHN_CERT_FILE", certFile)
		t.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", "/nonexistent/key.pem")

		err := validateCertFiles()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration error:")
		assert.Contains(t, err.Error(), "cannot read key file")
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

		got, err := provider.FetchSecret(context.Background(), "path/to/var")

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

		_, err := provider.FetchSecret(context.Background(), "path/to/missing")

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

		got, err := provider.FetchSecret(context.Background(), "path/to/empty")

		require.NoError(t, err)
		assert.Equal(t, []byte{}, got)
	})

	t.Run("authenticator not found (404 with Details) returns authentication-failed error", func(t *testing.T) {
		// A 404 from the auth endpoint (JWT webservice not configured) carries a
		// structured body with Details. Without the tightened guard, summon would
		// say "variable not found" which misleads the operator.
		conjurErr := &response.ConjurError{
			Code:    404,
			Message: "Not Found",
			Details: &response.ConjurErrorDetails{
				Code:    "CONJ00036E",
				Message: "Webservice conjur/acct:webservice:conjur/authn-jwt/myservice is not found",
			},
		}
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return nil, conjurErr
			},
		})

		_, err := provider.FetchSecret(context.Background(), "path/to/var")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication failed")
		assert.Contains(t, err.Error(), "authenticator not found")
		assert.NotContains(t, err.Error(), "variable")
	})

	t.Run("auth failure (401) returns error", func(t *testing.T) {
		conjurErr := &response.ConjurError{Code: 401, Message: "Unauthorized"}
		provider := NewWithClient(&mockConjurClient{
			retrieveSecret: func(_ string) ([]byte, error) {
				return nil, conjurErr
			},
		})

		_, err := provider.FetchSecret(context.Background(), "path/to/var")

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

		_, err := provider.FetchSecret(context.Background(), "path/to/var")

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

// isolateHome points the home directory at an empty temp dir so no user config
// (e.g. ~/.conjurrc) leaks into the test: HOME on Unix, USERPROFILE on Windows.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestIsUnauthorized(t *testing.T) {
	conjurErr := func(code int) error { return &response.ConjurError{Code: code} }
	joined := errors.Join(errors.New("other"), fmt.Errorf("authentication failed: %w", conjurErr(401)))

	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"bare 401":                   {conjurErr(401), true},
		"wrapped 401":                {fmt.Errorf("authentication failed: %w", conjurErr(401)), true},
		"401 through retrievalError": {retrievalError("v", conjurErr(401)), true},
		"401 in joined errors":       {joined, true},
		"403":                        {retrievalError("v", conjurErr(403)), false},
		"404":                        {retrievalError("v", conjurErr(404)), false},
		"500":                        {conjurErr(500), false},
		"plain error":                {errors.New("401 Unauthorized"), false},
		"context deadline":           {context.DeadlineExceeded, false},
		"nil":                        {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsUnauthorized(tc.err))
		})
	}
}
