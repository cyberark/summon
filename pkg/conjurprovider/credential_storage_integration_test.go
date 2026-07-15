//go:build integration

package conjurprovider

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func netrcDigest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func setupFileCredentialStorage(t *testing.T) string {
	t.Helper()

	netrcPath := filepath.Join(t.TempDir(), ".netrc")
	require.NoError(t, os.WriteFile(netrcPath, nil, 0o600))

	// os.Setenv — callers pair this with ClearEnv/RestoreEnv; t.Setenv cleanups
	// would run after RestoreEnv and undo restored CONJUR_* values.
	os.Setenv("CONJUR_CREDENTIAL_STORAGE", "file")
	os.Setenv("CONJUR_NETRC_PATH", netrcPath)

	return netrcPath
}

// TestCredentialStorageModeIntegration smoke-tests built-in provider init with
// file credential storage and CONJUR_CREDENTIAL_STORAGE_MODE unset vs readwrite.
// ReadOnly write suppression is covered by unit tests; API-key-from-env auth does
// not write netrc in either mode, so these subtests do not assert cache mutation.
func TestCredentialStorageModeIntegration(t *testing.T) {
	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	apiKey := os.Getenv("CONJUR_AUTHN_API_KEY")

	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("default storage mode smoke test with file backend", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()
		setupFileCredentialStorage(t)
		t.Setenv("CONJUR_CREDENTIAL_STORAGE_MODE", "")

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

	t.Run("readwrite storage mode smoke test with file backend", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, login, apiKey)
		defer snap.RestoreEnv()
		setupFileCredentialStorage(t)
		t.Setenv("CONJUR_CREDENTIAL_STORAGE_MODE", "readwrite")

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
}
