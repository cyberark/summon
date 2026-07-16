//go:build integration

package conjurprovider

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	conjurapi "github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jwtServiceID = "test"

var jwtAuthenticatorPolicy = `
- !policy
  id: test
  body:
  - !webservice
  - !variable public-keys
  - !variable issuer
  - !variable audience
  - !variable token-app-property
  - !variable identity-path
  - !webservice status
  - !group authenticatable
  - !permit
    role: !group authenticatable
    privilege: [ read, authenticate ]
    resource: !webservice
  - !grant
    role: !group authenticatable
    member: !host /data/test/jwt-apps/workload@example.com
`

var jwtRolePolicy = `
- !policy
  id: jwt-apps
  body:
  - !host
    id: workload@example.com
    annotations:
      authn-jwt/test/sub: test-workload
`

func jwtVariablePolicy(variableID, hostID string) string {
	return fmt.Sprintf(`
- !variable %s

- !permit
  role: !host %s
  privilege: [ read, execute ]
  resource: !variable %s
`, variableID, hostID, variableID)
}

func setupJWTAuthenticator(t *testing.T, conjur *conjurapi.Client, publicKeys string, saas bool) func() {
	t.Helper()

	var roleCleanup func()
	var err error
	if saas {
		adminUser := os.Getenv("CONJUR_AUTHN_LOGIN")
		require.NotEmpty(t, adminUser)

		emptyTestBranch := fmt.Sprintf(`
- !policy
  id: test
  owner: !user /%s`, adminUser)

		_, err = conjur.LoadPolicy(conjurapi.PolicyModePatch, "data", strings.NewReader(emptyTestBranch))
		require.NoError(t, err)

		_, err = conjur.LoadPolicy(conjurapi.PolicyModePut, "data/test", strings.NewReader(`--- []`))
		require.NoError(t, err)

		roleCleanup, err = loadPolicy(conjur, "data/test", jwtRolePolicy)
		require.NoError(t, err)
	} else {
		dataTestBranch := `
- !policy
  id: data
  body:
  - !policy test`

		_, err = conjur.LoadPolicy(conjurapi.PolicyModePost, "root", strings.NewReader(dataTestBranch))
		require.NoError(t, err)

		roleCleanup, err = loadPolicy(conjur, "data/test", jwtRolePolicy)
		require.NoError(t, err)

		authnBranch := `
- !policy
  id: conjur/authn-jwt`

		_, err = conjur.LoadPolicy(conjurapi.PolicyModePost, "root", strings.NewReader(authnBranch))
		require.NoError(t, err)
	}

	authnCleanup, err := loadPolicy(conjur, "conjur/authn-jwt", jwtAuthenticatorPolicy)
	require.NoError(t, err)

	jwks := "{\"type\":\"jwks\",\"value\":" + publicKeys + "}"
	require.NoError(t, conjur.AddSecret("conjur/authn-jwt/test/public-keys", jwks))
	require.NoError(t, conjur.AddSecret("conjur/authn-jwt/test/issuer", "jwt-server"))
	require.NoError(t, conjur.AddSecret("conjur/authn-jwt/test/audience", "conjur"))
	require.NoError(t, conjur.AddSecret("conjur/authn-jwt/test/token-app-property", "email"))
	require.NoError(t, conjur.AddSecret("conjur/authn-jwt/test/identity-path", "data/test/jwt-apps"))
	require.NoError(t, conjur.EnableAuthenticator("jwt", jwtServiceID, true))

	return func() {
		authnCleanup()
		roleCleanup()
	}
}

func createSaaSAdminConjurClient(applianceURL, account, token string) (*conjurapi.Client, error) {
	config := conjurapi.Config{
		ApplianceURL: applianceURL,
		Account:      account,
		Environment:  conjurapi.EnvironmentSaaS,
	}
	return conjurapi.NewClientFromToken(config, token)
}

func isSaaSApplianceURL(applianceURL string) bool {
	parsed, err := url.Parse(applianceURL)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	return conjurapi.ConjurCloudRegexp.MatchString(strings.ToLower(parsed.Hostname()))
}

// TestAuthnJWTIntegration verifies authn-jwt end-to-end against OSS Conjur.
func TestAuthnJWTIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_SAAS")) == "true" {
		t.Skip("Skipping OSS JWT test when TEST_SAAS=true")
	}

	publicKeys := os.Getenv("PUBLIC_KEYS")
	jwtToken := os.Getenv("JWT")
	if publicKeys == "" || jwtToken == "" {
		t.Skip("Skipping JWT authn test (PUBLIC_KEYS or JWT not set)")
	}

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")

	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via JWT authn", func(t *testing.T) {
		authnCleanup := setupJWTAuthenticator(t, conjur, publicKeys, false)
		defer authnCleanup()

		variableIdentifier := fmt.Sprintf("ci/test/%s/db-password", generateRandomString(8))
		varCleanup, err := loadPolicy(conjur, "root", jwtVariablePolicy(variableIdentifier, "data/test/jwt-apps/workload@example.com"))
		require.NoError(t, err)
		defer varCleanup()

		secretValue, err := addSecretWithRandomValue(conjur, variableIdentifier)
		require.NoError(t, err)

		t.Run("missing service ID returns configuration error", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			// Service ID normally implies jwt authn; with it omitted, set authn type
			// explicitly so conjur-api-go validates JWT config instead of .netrc fallback.
			os.Setenv("CONJUR_AUTHN_TYPE", "jwt")
			os.Setenv("CONJUR_AUTHN_JWT_TOKEN", jwtToken)

			_, stderr, err := runSummonWithVar(variableIdentifier)
			assert.Error(t, err)
			errText := stderr.String()
			assert.Contains(t, errText, "Must specify a ServiceID when using jwt",
				"expected missing JWT service ID config error; stderr: %s", errText)
			assert.NotContains(t, errText, jwtToken)
			assert.NotContains(t, errText, secretValue)
		})

		t.Run("missing JWT token returns authentication error", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", jwtServiceID)

			_, stderr, err := runSummonWithVar(variableIdentifier)
			assert.Error(t, err)
			assert.NotContains(t, stderr.String(), jwtToken)
		})

		t.Run("valid JWT configuration retrieves secret via env token", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", jwtServiceID)
			os.Setenv("CONJUR_AUTHN_JWT_TOKEN", jwtToken)

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)
			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})

		t.Run("valid JWT configuration retrieves secret via JWT_TOKEN_PATH", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", jwtServiceID)
			os.Unsetenv("CONJUR_AUTHN_JWT_TOKEN")

			tokenFile := fmt.Sprintf("%s/jwt-token", t.TempDir())
			require.NoError(t, os.WriteFile(tokenFile, []byte(jwtToken), 0o600))
			os.Setenv("JWT_TOKEN_PATH", tokenFile)

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)
			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})
	})
}

// TestAuthnJWTSaaSIntegration verifies authn-jwt against an ephemeral Secrets Manager SaaS tenant.
func TestAuthnJWTSaaSIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_SAAS")) != "true" {
		t.Skip("Skipping SaaS JWT authn test (TEST_SAAS != true)")
	}

	publicKeys := os.Getenv("PUBLIC_KEYS")
	jwtToken := os.Getenv("JWT")
	if publicKeys == "" || jwtToken == "" {
		t.Fatal("PUBLIC_KEYS and JWT must be set to run SaaS JWT integration tests")
	}

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")
	login := os.Getenv("CONJUR_AUTHN_LOGIN")
	token := os.Getenv("CONJUR_AUTHN_TOKEN")
	if applianceURL == "" || login == "" || token == "" {
		t.Fatal("CONJUR_APPLIANCE_URL, CONJUR_AUTHN_LOGIN, and CONJUR_AUTHN_TOKEN must be set for SaaS tests")
	}
	if !isSaaSApplianceURL(applianceURL) {
		t.Fatalf("CONJUR_APPLIANCE_URL does not look like a Secrets Manager SaaS URL: %s", applianceURL)
	}

	conjur, err := createSaaSAdminConjurClient(applianceURL, account, token)
	require.NoError(t, err)

	t.Run("retrieves variable via JWT authn on SaaS", func(t *testing.T) {
		authnCleanup := setupJWTAuthenticator(t, conjur, publicKeys, true)
		defer authnCleanup()

		variableIdentifier := fmt.Sprintf("data/test/ci/test/%s/db-password", generateRandomString(8))
		varCleanup, err := loadPolicy(conjur, "data/test", jwtVariablePolicy(
			strings.TrimPrefix(variableIdentifier, "data/test/"),
			"jwt-apps/workload@example.com",
		))
		require.NoError(t, err)
		defer varCleanup()

		secretValue, err := addSecretWithRandomValue(conjur, variableIdentifier)
		require.NoError(t, err)

		snap := setupTestEnvironment(t, applianceURL, account, "", "")
		defer snap.RestoreEnv()
		os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", jwtServiceID)
		os.Setenv("CONJUR_AUTHN_JWT_TOKEN", jwtToken)

		retrieved, stderr, err := runSummonWithVar(variableIdentifier)
		require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
		assert.Equal(t, secretValue, retrieved)

		t.Run("retrieves variable via JWT_TOKEN_PATH on SaaS", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_JWT_SERVICE_ID", jwtServiceID)
			os.Unsetenv("CONJUR_AUTHN_JWT_TOKEN")

			tokenFile := fmt.Sprintf("%s/jwt-token", t.TempDir())
			require.NoError(t, os.WriteFile(tokenFile, []byte(jwtToken), 0o600))
			os.Setenv("JWT_TOKEN_PATH", tokenFile)

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)
			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})
	})
}
