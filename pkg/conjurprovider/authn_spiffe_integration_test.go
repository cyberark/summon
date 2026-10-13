//go:build integration

package conjurprovider

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Environment variable names used across the SPIFFE integration tests.
const (
	// envTestSPIFFE gates all SPIFFE integration tests.
	envTestSPIFFE = "TEST_SPIFFE"

	// envSPIFFESocket is the SPIFFE Workload API endpoint address.
	envSPIFFESocket = "SPIFFE_ENDPOINT_SOCKET"

	// Input vars read once at the top of each test function (set by setup-spiffe.sh).
	envAuthnJWTSPIFFEServiceID     = "CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID"
	envAuthnJWTSPIFFEOIDCServiceID = "CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID"
	envAuthnJWTHostID              = "CONJUR_AUTHN_JWT_HOST_ID"
	envAuthnCertServiceID          = "CONJUR_AUTHN_CERT_SERVICE_ID"
	envAuthnCertHostID             = "CONJUR_AUTHN_CERT_HOST_ID"
	envCertSSLCertificate          = "CONJUR_CERT_SSL_CERTIFICATE"
	envCertApplianceURL            = "CONJUR_CERT_APPLIANCE_URL"
	envSPIFFETestSecretID          = "CONJUR_SPIFFE_TEST_SECRET_ID"
	envSPIFFETestSecretValue       = "CONJUR_SPIFFE_TEST_SECRET_VALUE"
	envApplianceURL                = "CONJUR_APPLIANCE_URL"
	envAccount                     = "CONJUR_ACCOUNT"

	// Runtime vars set inside each subtest via os.Setenv.
	envAuthnJWTServiceID = "CONJUR_AUTHN_JWT_SERVICE_ID"
	envSSLCertificate    = "CONJUR_SSL_CERTIFICATE"
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
	if os.Getenv(envTestSPIFFE) != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv(envSPIFFESocket)
	serviceID := os.Getenv(envAuthnJWTSPIFFEServiceID)
	hostID := os.Getenv(envAuthnJWTHostID)
	secretID := os.Getenv(envSPIFFETestSecretID)
	expectedValue := os.Getenv(envSPIFFETestSecretValue)

	if socketPath == "" || serviceID == "" || hostID == "" || secretID == "" {
		t.Skip("Skipping SPIFFE JWT test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_JWT_SPIFFE_SERVICE_ID, CONJUR_AUTHN_JWT_HOST_ID, or CONJUR_SPIFFE_TEST_SECRET_ID not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	applianceURL := os.Getenv(envApplianceURL)
	account := os.Getenv(envAccount)
	require.NotEmpty(t, applianceURL, "CONJUR_APPLIANCE_URL must be set")
	require.NotEmpty(t, account, "CONJUR_ACCOUNT must be set")

	t.Run("retrieves secret via SPIFFE JWT-SVID", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnJWTServiceID, serviceID)
		os.Setenv(envAuthnJWTHostID, hostID)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE JWT-SVID auth; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})

	t.Run("SPIFFE_ENDPOINT_SOCKET is not filtered from child env", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnJWTServiceID, serviceID)
		os.Setenv(envAuthnJWTHostID, hostID)

		yamlSpec := fmt.Sprintf("SECRET: !var %s", secretID)
		// printenv exits 1 when the variable is absent; exit 0 proves it was passed.
		_, stderr, err := RunCommand(
			"--yaml", yamlSpec, "--",
			"printenv", envSPIFFESocket,
		)
		require.NoError(t, err, "SPIFFE_ENDPOINT_SOCKET should reach child process; stderr: %s", stderr.String())
	})

	t.Run("missing JWT service ID returns configuration error", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
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
	if os.Getenv(envTestSPIFFE) != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv(envSPIFFESocket)
	serviceID := os.Getenv(envAuthnJWTSPIFFEOIDCServiceID)
	hostID := os.Getenv(envAuthnJWTHostID)
	secretID := os.Getenv(envSPIFFETestSecretID)
	expectedValue := os.Getenv(envSPIFFETestSecretValue)

	if socketPath == "" || serviceID == "" || hostID == "" || secretID == "" {
		t.Skip("Skipping SPIFFE JWT OIDC test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_JWT_SPIFFE_OIDC_SERVICE_ID, CONJUR_AUTHN_JWT_HOST_ID, or CONJUR_SPIFFE_TEST_SECRET_ID not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	applianceURL := os.Getenv(envApplianceURL)
	account := os.Getenv(envAccount)
	require.NotEmpty(t, applianceURL, "CONJUR_APPLIANCE_URL must be set")
	require.NotEmpty(t, account, "CONJUR_ACCOUNT must be set")

	t.Run("retrieves secret via SPIFFE JWT-SVID with OIDC jwks-uri key source", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnJWTServiceID, serviceID)
		os.Setenv(envAuthnJWTHostID, hostID)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE JWT-SVID auth via jwks-uri; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})
}

// TestAuthnSpiffeCertIntegration verifies X.509-SVID authentication via the
// SPIFFE Workload API end-to-end against the Conjur Enterprise Appliance.
// authn-cert requires mTLS, which needs HTTPS — only the Enterprise Appliance
// (conjur-leader) serves HTTPS in the integration test stack.
//
// The test uses authn-cert URL-based mode: the host ID is embedded in the URL.
// The host carries an authn-cert/<service-id>/san-uri annotation that the
// Enterprise Appliance validates against the cert's URI SAN field.
//
// Prerequisites (set by bin/setup-spiffe.sh):
//   - SPIFFE_ENDPOINT_SOCKET: address of the SPIRE agent socket
//   - CONJUR_AUTHN_CERT_SERVICE_ID: authn-cert service ID configured with the SPIRE CA
//   - CONJUR_AUTHN_CERT_HOST_ID: Conjur host ID for URL-based authn-cert
//   - CONJUR_CERT_APPLIANCE_URL: URL of the Enterprise Appliance (https://...)
//   - CONJUR_SPIFFE_TEST_SECRET_ID: variable path to retrieve
//   - CONJUR_SPIFFE_TEST_SECRET_VALUE: expected secret value
func TestAuthnSpiffeCertIntegration(t *testing.T) {
	if os.Getenv(envTestSPIFFE) != "true" {
		t.Skip("Skipping SPIFFE integration tests (TEST_SPIFFE != true)")
	}

	socketPath := os.Getenv(envSPIFFESocket)
	serviceID := os.Getenv(envAuthnCertServiceID)
	certHostID := os.Getenv(envAuthnCertHostID)
	secretID := os.Getenv(envSPIFFETestSecretID)
	expectedValue := os.Getenv(envSPIFFETestSecretValue)
	// The Enterprise Appliance uses a self-signed cert. CONJUR_IGNORE_UNTRUSTED_SSL
	// is not honored by conjur-api-go for mTLS connections; the cert must be
	// provided explicitly via CONJUR_SSL_CERTIFICATE (config.ReadSSLCert).
	sslCert := os.Getenv(envCertSSLCertificate)

	if socketPath == "" || serviceID == "" || certHostID == "" || secretID == "" || sslCert == "" {
		t.Skip("Skipping SPIFFE cert test: SPIFFE_ENDPOINT_SOCKET, CONJUR_AUTHN_CERT_SERVICE_ID, CONJUR_AUTHN_CERT_HOST_ID, CONJUR_SPIFFE_TEST_SECRET_ID, or CONJUR_CERT_SSL_CERTIFICATE not set")
	}
	require.NotEmpty(t, expectedValue, "CONJUR_SPIFFE_TEST_SECRET_VALUE must be set when TEST_SPIFFE=true")

	// The cert test targets the Enterprise Appliance (HTTPS, mTLS).
	// CONJUR_CERT_APPLIANCE_URL is exported by start_conjur_leader_for_spiffe.
	// The Enterprise Appliance always uses account "conjur".
	applianceURL := os.Getenv(envCertApplianceURL)
	if applianceURL == "" {
		applianceURL = os.Getenv(envApplianceURL)
	}
	account := "conjur"
	require.NotEmpty(t, applianceURL, "CONJUR_CERT_APPLIANCE_URL must be set for SPIFFE cert tests")

	t.Run("retrieves secret via SPIFFE X.509-SVID", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnCertServiceID, serviceID)
		os.Setenv(envAuthnCertHostID, certHostID)
		os.Setenv(envSSLCertificate, sslCert)

		retrieved, stderr, err := runSummonWithVar(secretID)
		require.NoError(t, err, "summon should exit 0 with SPIFFE X.509-SVID auth; stderr: %s", stderr.String())
		assert.Equal(t, expectedValue, retrieved)
	})

	t.Run("invalid cert service ID returns authentication error without leaking secret", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnCertServiceID, "nonexistent-service")
		os.Setenv(envAuthnCertHostID, certHostID)
		os.Setenv(envSSLCertificate, sslCert)

		_, stderr, err := runSummonWithVar(secretID)
		assert.Error(t, err)
		assert.NotContains(t, stderr.String(), expectedValue,
			"error output must not contain the secret value")
	})

	t.Run("SPIFFE_ENDPOINT_SOCKET is not filtered from child env", func(t *testing.T) {
		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()

		os.Setenv(envSPIFFESocket, socketPath)
		os.Setenv(envAuthnCertServiceID, serviceID)
		os.Setenv(envAuthnCertHostID, certHostID)
		os.Setenv(envSSLCertificate, sslCert)

		yamlSpec := fmt.Sprintf("SECRET: !var %s", secretID)
		_, stderr, err := RunCommand(
			"--yaml", yamlSpec, "--",
			"printenv", envSPIFFESocket,
		)
		require.NoError(t, err, "SPIFFE_ENDPOINT_SOCKET should reach child process; stderr: %s", stderr.String())
	})
}
