//go:build integration

package conjurprovider

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIKeyAuth verifies API key auth end-to-end using the built-in Conjur
// provider with no --provider flag (default routing path).
func TestAPIKeyAuth(t *testing.T) {
	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

	// Use admin credentials for policy provisioning; test host for summon invocations
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via API key auth with no --provider flag", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()

		varPath := fmt.Sprintf("ci/test/%s/secret", generateRandomString(8))
		cleanup, err := setupVariablePolicy(conjur, varPath)
		require.NoError(t, err)
		defer cleanup()

		secretValue, err := addSecretWithRandomValue(conjur, varPath)
		require.NoError(t, err)

		retrieved, stderr, err := runSummonWithVar(varPath)

		require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
		assert.Equal(t, secretValue, retrieved)
	})

	t.Run("returns non-zero exit on invalid API key", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, "invalid-api-key")
		defer snap.RestoreEnv()

		_, stderr, err := runSummonWithVar("ci/test/irrelevant-var")

		assert.Error(t, err, "expected non-zero exit on auth failure")
		assert.NotContains(t, stderr.String(), "invalid-api-key",
			"raw credential must not appear in stderr")
	})

	t.Run("returns non-zero exit when variable does not exist", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()

		_, _, err := runSummonWithVar("ci/test/non-existent-variable-that-does-not-exist")

		assert.Error(t, err, "expected non-zero exit when variable is missing")
	})
}

// TestAuthnTokenEnvVar verifies CONJUR_AUTHN_TOKEN env var auth.
func TestAuthnTokenEnvVar(t *testing.T) {
	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

	// Use admin credentials for policy provisioning; obtain token via test host credentials
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via CONJUR_AUTHN_TOKEN with no API key set", func(t *testing.T) {
		// Obtain a raw Conjur token via the authn endpoint without shell interpolation
		authnURL := fmt.Sprintf("%s/authn/%s/%s/authenticate", applianceURL, account, url.QueryEscape(login))
		tokenStdout, _, err := runProgramCommand("curl", "-sk", "--data", apiKey, authnURL)
		require.NoError(t, err)
		token := tokenStdout.String()
		require.Contains(t, token, "signature", "expected a valid Conjur token in response")

		varPath := fmt.Sprintf("ci/test/%s/secret", generateRandomString(8))
		cleanup, err := setupVariablePolicy(conjur, varPath)
		require.NoError(t, err)
		defer cleanup()

		secretValue, err := addSecretWithRandomValue(conjur, varPath)
		require.NoError(t, err)

		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv("CONJUR_AUTHN_TOKEN", token)
		// Zero-auth proof (AC#2): conjur-api-go uses CONJUR_AUTHN_TOKEN at priority 2,
		// before API key (priority 6). An invalid API key here ensures that any fallback
		// to API key auth would fail; success proves no authn calls were made.
		os.Setenv("CONJUR_AUTHN_API_KEY", "invalid-api-key-zero-auth-sentinel")

		retrieved, stderr, err := runSummonWithVar(varPath)

		require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
		assert.Equal(t, secretValue, retrieved)
	})

	t.Run("returns non-zero on syntactically invalid CONJUR_AUTHN_TOKEN", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		invalidToken := "not-a-valid-base64-token"
		os.Setenv("CONJUR_AUTHN_TOKEN", invalidToken)

		_, stderr, err := runSummonWithVar("ci/test/any-var")

		assert.Error(t, err)
		assert.NotContains(t, stderr.String(), invalidToken,
			"raw token must not appear in stderr")
	})
}

// TestAuthnTokenFile verifies CONJUR_AUTHN_TOKEN_FILE auth.
func TestAuthnTokenFile(t *testing.T) {
	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

	// Use admin credentials for policy provisioning; obtain token via test host credentials
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via CONJUR_AUTHN_TOKEN_FILE", func(t *testing.T) {
		// Obtain a raw token via the authn endpoint without shell interpolation
		authnURL := fmt.Sprintf("%s/authn/%s/%s/authenticate", applianceURL, account, url.QueryEscape(login))
		tokenStdout, _, err := runProgramCommand("curl", "-sk", "--data", apiKey, authnURL)
		require.NoError(t, err)
		token := tokenStdout.String()
		require.Contains(t, token, "signature", "expected a valid Conjur token")

		// Write token to a temp file; clean up in t.Cleanup
		tokenFile, err := os.CreateTemp("", "conjur-token-*")
		require.NoError(t, err)
		tokenFileName := tokenFile.Name()
		_, err = tokenFile.WriteString(token)
		require.NoError(t, err)
		require.NoError(t, tokenFile.Close())
		t.Cleanup(func() { os.Remove(tokenFileName) })

		varPath := fmt.Sprintf("ci/test/%s/secret", generateRandomString(8))
		cleanup, err := setupVariablePolicy(conjur, varPath)
		require.NoError(t, err)
		defer cleanup()

		secretValue, err := addSecretWithRandomValue(conjur, varPath)
		require.NoError(t, err)

		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv("CONJUR_AUTHN_TOKEN_FILE", tokenFileName)
		// Zero-auth proof (AC#2): CONJUR_AUTHN_TOKEN_FILE is priority 1 in conjur-api-go;
		// an invalid API key here ensures any fallback to API key auth would fail.
		os.Setenv("CONJUR_AUTHN_API_KEY", "invalid-api-key-zero-auth-sentinel")

		retrieved, stderr, err := runSummonWithVar(varPath)

		require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
		assert.Equal(t, secretValue, retrieved)
	})

	t.Run("returns non-zero on invalid token in CONJUR_AUTHN_TOKEN_FILE", func(t *testing.T) {
		invalidToken := "not-a-valid-base64-token"
		tokenFile, err := os.CreateTemp("", "conjur-token-invalid-*")
		require.NoError(t, err)
		tokenFileName := tokenFile.Name()
		_, err = tokenFile.WriteString(invalidToken)
		require.NoError(t, err)
		require.NoError(t, tokenFile.Close())
		t.Cleanup(func() { os.Remove(tokenFileName) })

		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv("CONJUR_AUTHN_TOKEN_FILE", tokenFileName)

		_, stderr, err := runSummonWithVar("ci/test/any-var")

		assert.Error(t, err)
		assert.NotContains(t, stderr.String(), invalidToken,
			"raw token must not appear in stderr")
	})
}
