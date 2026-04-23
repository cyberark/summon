//go:build integration

package conjurprovider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gcpRolesPolicyTemplate(projectID, variableID string) string {
	return fmt.Sprintf(`
- !host
  id: test-app
  annotations:
    authn-gcp/project-id: %q

- !variable %s

- !permit
  role: !host test-app
  privilege: [ read, execute ]
  resource: !variable %s
`, projectID, variableID, variableID)
}

var gcpAuthnPolicy = `
- !policy
  id: conjur/authn-gcp
  body:
  - !webservice

  - !group apps

  - !permit
    role: !group apps
    privilege: [ read, authenticate ]
    resource: !webservice

  - !grant
    role: !group apps
    member: !host /test-app
`

// TestAuthnGCPIntegration verifies GCP metadata authn end-to-end, guarded by TEST_GCP.
func TestAuthnGCPIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_GCP")) != "true" {
		t.Skip("Skipping GCP metadata authn test (TEST_GCP != true)")
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	prefetchedToken := os.Getenv("GCP_ID_TOKEN")
	if projectID == "" || prefetchedToken == "" {
		t.Fatal("GCP_PROJECT_ID and GCP_ID_TOKEN must be set to run this test")
	}

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")

	// Use admin credentials for policy provisioning; tests invoke summon using
	// the CONJUR_AUTHN_* env vars, which are set to the test host identity.
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via GCP metadata authn", func(t *testing.T) {
		variableIdentifier := fmt.Sprintf("ci/test/%s/db-password", generateRandomString(8))
		rolesPolicy := gcpRolesPolicyTemplate(projectID, variableIdentifier)

		rootCleanup, err := loadPolicy(conjur, "root", rolesPolicy)
		require.NoError(t, err)
		defer rootCleanup()

		authnCleanup, err := loadPolicy(conjur, "root", gcpAuthnPolicy)
		require.NoError(t, err)
		defer authnCleanup()

		require.NoError(t, conjur.EnableAuthenticator("gcp", "", true))

		secretValue, err := addSecretWithRandomValue(conjur, variableIdentifier)
		require.NoError(t, err)

		t.Run("wrong host-id returns error", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "gcp")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", "invalid/host-id")

			_, _, err := runSummonWithVar(variableIdentifier)
			assert.Error(t, err)
		})

		t.Run("valid authn-gcp configuration with pre-fetched token retrieves secret", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "gcp")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", "test-app")
			os.Setenv("CONJUR_AUTHN_JWT_TOKEN", prefetchedToken)

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)

			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})
	})
}
