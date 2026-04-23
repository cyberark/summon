//go:build integration

package conjurprovider

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalProviderOverride verifies that when --provider is set to an external
// binary, summon routes to that binary and never invokes the built-in Conjur path.
// Because the built-in path requires Conjur config, a passing test proves it was not reached.
func TestExternalProviderOverride(t *testing.T) {
	t.Run("routes to external provider and returns expected value", func(t *testing.T) {
		// ClearEnv removes all CONJUR_* vars so the built-in path cannot succeed
		savedPath := os.Getenv("PATH")
		snap := ClearEnv()
		defer snap.RestoreEnv()
		// Restore PATH so summon can exec printenv
		os.Setenv("PATH", savedPath)

		stdout, stderr, err := RunCommand(
			"--provider", "./testdata/fake-provider",
			"--yaml", "X: !var any/path",
			"--", "printenv", "X",
		)

		require.NoError(t, err, "summon should exit 0 with fake provider; stderr: %s", stderr.String())
		assert.Equal(t, "FAKE_PROVIDER\n", stdout.String())
	})

	t.Run("uses external provider even when CONJUR_* vars are set", func(t *testing.T) {
		applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
		account := os.Getenv("CONJUR_ACCOUNT")
		login := os.Getenv("CONJUR_AUTHN_LOGIN")
		apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()

		stdout, stderr, err := RunCommand(
			"--provider", "./testdata/fake-provider",
			"--yaml", "X: !var any/path",
			"--", "printenv", "X",
		)

		require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
		assert.Equal(t, "FAKE_PROVIDER\n", stdout.String(),
			"external provider value, not a Conjur value, must be returned")
	})

	t.Run("routes to external provider via SUMMON_PROVIDER env var", func(t *testing.T) {
		savedPath := os.Getenv("PATH")
		snap := ClearEnv()
		defer snap.RestoreEnv()
		os.Setenv("PATH", savedPath)
		os.Setenv("SUMMON_PROVIDER", "./testdata/fake-provider")

		stdout, stderr, err := RunCommand(
			"--yaml", "X: !var any/path",
			"--", "printenv", "X",
		)

		require.NoError(t, err, "summon should exit 0 with SUMMON_PROVIDER set; stderr: %s", stderr.String())
		assert.Equal(t, "FAKE_PROVIDER\n", stdout.String())
	})
}

// TestDefaultProviderRouting verifies that when neither --provider nor SUMMON_PROVIDER
// is set, summon selects the built-in Conjur path and does not attempt external binary
// dispatch. This is distinct from TestAPIKeyAuth: TestAPIKeyAuth validates that the
// API key auth outcome succeeds; TestDefaultProviderRouting validates the routing
// decision itself — that no external binary is invoked.
func TestDefaultProviderRouting(t *testing.T) {
	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("selects built-in Conjur provider when no --provider or SUMMON_PROVIDER is set", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()

		// Explicitly unset both provider env vars to prove default routing
		os.Unsetenv("SUMMON_PROVIDER")

		varPath := fmt.Sprintf("ci/test/%s/secret", generateRandomString(8))
		cleanup, err := setupVariablePolicy(conjur, varPath)
		require.NoError(t, err)
		defer cleanup()

		secretValue, err := addSecretWithRandomValue(conjur, varPath)
		require.NoError(t, err)

		// Invoke summon with no --provider flag: must use built-in path
		retrieved, stderr, err := runSummonWithVar(varPath)

		require.NoError(t, err, "summon should use built-in provider and exit 0; stderr: %s", stderr.String())
		assert.Equal(t, secretValue, retrieved,
			"built-in Conjur provider must retrieve the secret value")
	})
}
