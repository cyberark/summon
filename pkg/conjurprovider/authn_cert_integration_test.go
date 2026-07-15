//go:build integration

package conjurprovider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	conjurapi "github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/cyberark/conjur-api-go/conjurapi/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const certServiceID = "acme-vm"

var authnCertPolicy = `
- !policy
  id: acme-vm
  body:
  - !webservice
  - !group clients
  - !permit
    role: !group clients
    privilege: [ read, authenticate ]
    resource: !webservice
  - !variable ca-cert
  - !grant
    role: !group clients
    member: !host /data/test/cert-apps/vm-01
`

var authCertRolesPolicy = `
- !policy
  id: cert-apps
  body:
  - &variables
    - !variable database/username
    - !variable database/password
  - !group secrets-users
  - !permit
    role: !group secrets-users
    privilege: [ read, execute ]
    resource: *variables
  - !layer
  - !host
    id: vm-01
    annotations:
      authn-cert/cn: vm-01
  - !grant
    role: !layer
    member: !host vm-01
  - !grant
    member: !layer
    role: !group secrets-users
`

func TestAuthnCertIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_CERT")) != "true" {
		t.Skip("Skipping cert authn test (TEST_CERT != true)")
	}

	certFile := os.Getenv("CONJUR_AUTHN_CERT_FILE")
	keyFile := os.Getenv("CONJUR_AUTHN_CERT_KEY_FILE")
	caCert := os.Getenv("TEST_CERT_CA_CERT")
	applianceURL := os.Getenv("CONJUR_CERT_APPLIANCE_URL")
	adminLogin := os.Getenv("CONJUR_CERT_AUTHN_LOGIN")
	adminAPIKey := os.Getenv("CONJUR_CERT_AUTHN_API_KEY")
	sslCert := os.Getenv("CONJUR_CERT_SSL_CERTIFICATE")
	if certFile == "" || keyFile == "" || caCert == "" || applianceURL == "" || adminLogin == "" || adminAPIKey == "" {
		t.Fatal("CONJUR_AUTHN_CERT_FILE, CONJUR_AUTHN_CERT_KEY_FILE, TEST_CERT_CA_CERT, CONJUR_CERT_APPLIANCE_URL, CONJUR_CERT_AUTHN_LOGIN, and CONJUR_CERT_AUTHN_API_KEY must be set")
	}

	t.Setenv("CONJUR_IGNORE_UNTRUSTED_SSL", "true")
	admin, err := createCertAdminConjurClient(applianceURL, adminLogin, adminAPIKey, sslCert)
	require.NoError(t, err)

	require.NoError(t, setupCertAuthenticator(admin, authnCertPolicy, authCertRolesPolicy))
	require.NoError(t, admin.EnableAuthenticator("cert", certServiceID, true))
	require.NoError(t, admin.AddSecret("conjur/authn-cert/"+certServiceID+"/ca-cert", caCert))
	require.NoError(t, admin.AddSecret("data/test/cert-apps/database/username", "cert-secret"))

	t.Run("retrieves variable via cert authn", func(t *testing.T) {
		snap := setupCertSummonEnvironment(t, applianceURL, sslCert, certFile, keyFile)
		defer snap.RestoreEnv()

		retrieved, stderr, err := runSummonWithVar("data/test/cert-apps/database/username")
		require.NoError(t, err, "stderr: %s", stderr.String())
		assert.Equal(t, "cert-secret", retrieved)
	})

	t.Run("leaves netrc unchanged when credential storage mode is ReadOnly", func(t *testing.T) {
		snap := setupCertSummonEnvironment(t, applianceURL, sslCert, certFile, keyFile)
		defer snap.RestoreEnv()
		netrcPath := setupFileCredentialStorage(t)

		before, err := netrcDigest(netrcPath)
		require.NoError(t, err)

		retrieved, stderr, err := runSummonWithVar("data/test/cert-apps/database/username")
		require.NoError(t, err, "stderr: %s", stderr.String())
		assert.Equal(t, "cert-secret", retrieved)

		after, err := netrcDigest(netrcPath)
		require.NoError(t, err)
		assert.Equal(t, before, after, "netrc must not change under ReadOnly mode")
	})

	t.Run("invalid cert exits non-zero without secret leakage", func(t *testing.T) {
		badCert := writeTempFile(t, "bad-cert.pem", "not-a-cert")
		snap := setupCertSummonEnvironment(t, applianceURL, sslCert, badCert, keyFile)
		defer snap.RestoreEnv()

		_, stderr, err := runSummonWithVar("data/test/cert-apps/database/username")
		require.Error(t, err)
		assert.NotContains(t, stderr.String(), "cert-secret")
	})
}

func createCertAdminConjurClient(applianceURL, login, apiKey, sslCert string) (*conjurapi.Client, error) {
	config := conjurapi.Config{
		ApplianceURL: applianceURL,
		Account:      "conjur",
		SSLCert:      sslCert,
	}
	return conjurapi.NewClientFromKey(
		config,
		authn.LoginPair{Login: login, APIKey: apiKey},
	)
}

func setupCertAuthenticator(conjur *conjurapi.Client, authenticatorPolicy, rolesPolicy string) error {
	// Match conjur-api-go DefaultTestUtils.Setup: workload policy lives under data/test.
	emptyTestBranch := `
- !policy
  id: data
  body:
    - !policy test`

	if _, err := conjur.LoadPolicy(conjurapi.PolicyModePost, "root", strings.NewReader(emptyTestBranch)); err != nil {
		return err
	}

	if _, err := conjur.LoadPolicy(conjurapi.PolicyModePost, "data/test", strings.NewReader(rolesPolicy)); err != nil {
		return err
	}

	emptyBranch := `- !policy
  id: conjur/authn-cert`
	if _, err := conjur.LoadPolicy(conjurapi.PolicyModePost, "root", strings.NewReader(emptyBranch)); err != nil {
		return err
	}

	_, err := conjur.LoadPolicy(conjurapi.PolicyModePost, "conjur/authn-cert", strings.NewReader(authenticatorPolicy))
	return err
}

func setupCertSummonEnvironment(t *testing.T, applianceURL, sslCert, certFile, keyFile string) *envSnapshot {
	t.Helper()
	// Use os.Setenv (not t.Setenv) with ClearEnv/RestoreEnv. t.Setenv registers
	// cleanups that restore post-ClearEnv empty values after defer RestoreEnv,
	// which poisons PATH/CONJUR_* for later tests in the same package.
	savedPath := os.Getenv("PATH")
	snap := ClearEnv()
	os.Setenv("PATH", savedPath)
	os.Setenv("CONJUR_APPLIANCE_URL", applianceURL)
	os.Setenv("CONJUR_ACCOUNT", "conjur")
	os.Setenv("CONJUR_SSL_CERTIFICATE", sslCert)
	os.Setenv("CONJUR_IGNORE_UNTRUSTED_SSL", "true")
	os.Setenv("CONJUR_AUTHN_CERT_SERVICE_ID", certServiceID)
	os.Setenv("CONJUR_AUTHN_CERT_FILE", certFile)
	os.Setenv("CONJUR_AUTHN_CERT_KEY_FILE", keyFile)
	os.Setenv("CONJUR_AUTHN_CERT_HOST_ID", "data/test/cert-apps/vm-01")
	os.Setenv("HOME", t.TempDir())
	return snap
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := fmt.Sprintf("%s/%s", dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// netrcDigest and setupFileCredentialStorage are defined in credential_storage_integration_test.go.
