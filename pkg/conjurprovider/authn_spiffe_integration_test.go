//go:build integration

package conjurprovider

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthnSpiffeJWTIntegration verifies JWT-SVID authentication via the
// SPIFFE Workload API end-to-end against OSS Conjur.
//
// Prerequisites (set by bin/setup-spiffe.sh):
//   - SPIFFE_ENDPOINT_SOCKET: address of the SPIRE agent socket
//   - CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID: authn-jwt service ID
//   - CONJUR_AUTHN_JWT_HOST_ID: Conjur host identity for URL-based authn-jwt
//   - CONJUR_SPIFFE_TEST_SECRET_ID: variable path to retrieve
//   - CONJUR_SPIFFE_TEST_SECRET_VALUE: expected secret value
func TestAuthnSpiffeJWTIntegration(t *testing.T) {
	if os.Getenv("TEST_SPIFFE") != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv("SPIFFE_ENDPOINT_SOCKET")
	serviceID := os.Getenv("CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID")
	hostID := os.Getenv("CONJUR_AUTHN_JWT_HOST_ID")
	secretID := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_ID")
	expectedValue := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_VALUE")

	if socketPath == "" || serviceID == "" || hostID == "" || secretID == "" {
		t.Skip("Skipping SPIFFE JWT test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID, CONJUR_AUTHN_JWT_HOST_ID, or CONJUR_SPIFFE_TEST_SECRET_ID not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	require.NotEmpty(t, applianceURL, "CONJUR_APPLIANCE_URL must be set")
	require.NotEmpty(t, account, "CONJUR_ACCOUNT must be set")

	t.Run("retrieves secret via SPIFFE JWT-SVID", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", serviceID)
		os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", hostID)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE JWT-SVID auth; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})

	t.Run("SPIFFE_ENDPOINT_SOCKET is not filtered from child env", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", serviceID)
		os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", hostID)

		yamlSpec := fmt.Sprintf("SECRET: !var %s", secretID)
		// printenv exits 1 when the variable is absent; exit 0 proves it was passed.
		_, stderr, err := RunCommand(
			"--yaml", yamlSpec, "--",
			"printenv", "SPIFFE_ENDPOINT_SOCKET",
		)
		require.NoError(t, err, "SPIFFE_ENDPOINT_SOCKET should reach child process; stderr: %s", stderr.String())
	})

	t.Run("missing JWT service ID returns configuration error", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		// No CONJUR_AUTHN_JWT_SERVICE_ID.

		_, stderr, err := runSummonWithVar(secretID)
		assert.Error(t, err)
		assert.NotContains(t, stderr.String(), expectedValue,
			"error output must not contain the secret value")
	})
}

// TestAuthnSpiffeJWTOIDCIntegration verifies JWT-SVID authentication via the
// SPIFFE Workload API using an authn-jwt authenticator configured with a
// jwks-uri (dynamic key fetch from the SPIRE OIDC Discovery Provider) rather
// than a static public-keys blob. This exercises the alternate key-source path
// that customers use when the SPIRE OIDC provider plugin is enabled.
//
// Prerequisites (set by bin/setup-spiffe.sh):
//   - SPIFFE_ENDPOINT_SOCKET: address of the SPIRE agent socket
//   - CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID: authn-jwt service ID using jwks-uri
//   - CONJUR_AUTHN_JWT_HOST_ID: Conjur host identity for URL-based authn-jwt
//   - CONJUR_SPIFFE_TEST_SECRET_ID: variable path to retrieve
//   - CONJUR_SPIFFE_TEST_SECRET_VALUE: expected secret value
func TestAuthnSpiffeJWTOIDCIntegration(t *testing.T) {
	if os.Getenv("TEST_SPIFFE") != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv("SPIFFE_ENDPOINT_SOCKET")
	serviceID := os.Getenv("CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID")
	hostID := os.Getenv("CONJUR_AUTHN_JWT_HOST_ID")
	secretID := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_ID")
	expectedValue := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_VALUE")

	if socketPath == "" || serviceID == "" || hostID == "" || secretID == "" {
		t.Skip("Skipping SPIFFE JWT OIDC test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID, CONJUR_AUTHN_JWT_HOST_ID, or CONJUR_SPIFFE_TEST_SECRET_ID not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	require.NotEmpty(t, applianceURL, "CONJUR_APPLIANCE_URL must be set")
	require.NotEmpty(t, account, "CONJUR_ACCOUNT must be set")

	t.Run("retrieves secret via SPIFFE JWT-SVID with OIDC jwks-uri key source", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", serviceID)
		os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", hostID)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE JWT-SVID auth via jwks-uri; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})
}
// SPIFFE Workload API end-to-end against OSS Conjur.
//
// Prerequisites (set by bin/setup-spiffe.sh):
//   - SPIFFE_ENDPOINT_SOCKET: address of the SPIRE agent socket
//   - CONJUR_AUTHN_CERT_SERVICE_ID: authn-cert service ID configured with the SPIRE CA
//   - CONJUR_SPIFFE_TEST_SECRET_ID: variable path to retrieve
//   - CONJUR_SPIFFE_TEST_SECRET_VALUE: expected secret value
func TestAuthnSpiffeCertIntegration(t *testing.T) {
	if os.Getenv("TEST_SPIFFE") != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv("SPIFFE_ENDPOINT_SOCKET")
	serviceID := os.Getenv("CONJUR_AUTHN_CERT_SERVICE_ID")
	secretID := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_ID")
	expectedValue := os.Getenv("CONJUR_SPIFFE_TEST_SECRET_VALUE")

	if socketPath == "" || serviceID == "" || secretID == "" {
		t.Skip("Skipping SPIFFE cert test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_CERT_SERVICE_ID, or CONJUR_SPIFFE_TEST_SECRET_ID not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	require.NotEmpty(t, applianceURL, "CONJUR_APPLIANCE_URL must be set")
	require.NotEmpty(t, account, "CONJUR_ACCOUNT must be set")

	t.Run("retrieves secret via SPIFFE X.509-SVID", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", serviceID)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE X.509-SVID auth; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})

	t.Run("invalid cert service ID returns authentication error without leaking secret", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", "nonexistent-service")

		_, stderr, err := runSummonWithVar(secretID)
		assert.Error(t, err)
		assert.NotContains(t, stderr.String(), expectedValue,
			"error output must not contain the secret value")
	})

	t.Run("SPIFFE_ENDPOINT_SOCKET is not filtered from child env", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv("SPIFFE_ENDPOINT_SOCKET", socketPath)
		os.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", serviceID)

		yamlSpec := fmt.Sprintf("SECRET: !var %s", secretID)
		_, stderr, err := RunCommand(
			"--yaml", yamlSpec, "--",
			"printenv", "SPIFFE_ENDPOINT_SOCKET",
		)
		require.NoError(t, err, "SPIFFE_ENDPOINT_SOCKET should reach child process; stderr: %s", stderr.String())
	})
}
