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

// azureRolesPolicyTemplate generates the Conjur host and variable policy for
// Azure MSI authn. variableID is unique per test run to avoid cross-test pollution.
func azureRolesPolicyTemplate(subscriptionID, resourceGroup, variableID string) string {
	return fmt.Sprintf(`
- !host
  id: azureVM
  annotations:
    authn-azure/subscription-id: %q
    authn-azure/resource-group: %q

- !variable %s

- !permit
  role: !host azureVM
  privilege: [ read, execute ]
  resource: !variable %s
`, subscriptionID, resourceGroup, variableID, variableID)
}

var azureAuthnPolicy = `
- !policy
  id: conjur/authn-azure/test
  body:
  - !webservice

  - !variable
    id: provider-uri

  - !group apps

  - !permit
    role: !group apps
    privilege: [ read, authenticate ]
    resource: !webservice

  - !grant
    role: !group apps
    member: !host /azureVM
`

// TestAuthnAzureIntegration verifies Azure MSI authn end-to-end, guarded by TEST_AZURE.
func TestAuthnAzureIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_AZURE")) != "true" {
		t.Skip("Skipping Azure MSI authn test (TEST_AZURE != true)")
	}

	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	resourceGroup := os.Getenv("AZURE_RESOURCE_GROUP")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	if subscriptionID == "" || resourceGroup == "" || tenantID == "" {
		t.Fatal("AZURE_SUBSCRIPTION_ID, AZURE_RESOURCE_GROUP, and AZURE_TENANT_ID must be set to run this test")
	}

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")

	// Use admin credentials for policy provisioning; tests invoke summon using
	// the CONJUR_AUTHN_* env vars, which are set to the test host identity.
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via Azure MSI authn", func(t *testing.T) {
		variableIdentifier := fmt.Sprintf("ci/test/%s/db-password", generateRandomString(8))
		rolesPolicy := azureRolesPolicyTemplate(subscriptionID, resourceGroup, variableIdentifier)

		rootCleanup, err := loadPolicy(conjur, "root", rolesPolicy)
		require.NoError(t, err)
		defer rootCleanup()

		authnCleanup, err := loadPolicy(conjur, "root", azureAuthnPolicy)
		require.NoError(t, err)
		defer authnCleanup()

		require.NoError(t, conjur.AddSecret("conjur/authn-azure/test/provider-uri",
			fmt.Sprintf("https://sts.windows.net/%s/", os.Getenv("AZURE_TENANT_ID"))))
		require.NoError(t, conjur.EnableAuthenticator("azure", "test", true))

		secretValue, err := addSecretWithRandomValue(conjur, variableIdentifier)
		require.NoError(t, err)

		t.Run("wrong service-id and host-id returns error", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "azure")
			os.Setenv("CONJUR_SERVICE_ID", "nonexistent-service")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", "invalid/host-id")

			_, _, err := runSummonWithVar(variableIdentifier)
			assert.Error(t, err)
		})

		t.Run("valid authn-azure configuration retrieves secret", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "azure")
			os.Setenv("CONJUR_SERVICE_ID", "test")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", "azureVM")

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)

			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})
	})
}
