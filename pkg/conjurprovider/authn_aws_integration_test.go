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

// awsRolePolicyTemplate generates the IAM role policy using the AWS account ID,
// IAM role name, and a unique variable identifier to avoid cross-test pollution.
func awsRolePolicyTemplate(awsAccountID, roleName, variableID string) string {
	return fmt.Sprintf(`
- !host %s/%s

- !variable %s
- !permit
  role: !host %s/%s
  privilege: [ read, execute ]
  resource: !variable %s
`, awsAccountID, roleName, variableID, awsAccountID, roleName, variableID)
}

// awsAuthnPolicyTemplate generates the authn-iam policy for the given host path.
func awsAuthnPolicyTemplate(awsAccountID, roleName string) string {
	return fmt.Sprintf(`
- !policy
  id: conjur/authn-iam/test
  body:
    - !webservice

    - !group clients

    - !permit
      role: !group clients
      privilege: [ read, authenticate ]
      resource: !webservice

    - !grant
      role: !group clients
      member: !host /%s/%s
`, awsAccountID, roleName)
}

// TestAuthnAWSIntegration verifies AWS IAM authn end-to-end, guarded by TEST_AWS.
func TestAuthnAWSIntegration(t *testing.T) {
	if strings.ToLower(os.Getenv("TEST_AWS")) != "true" {
		t.Skip("Skipping AWS IAM authn test (TEST_AWS != true)")
	}

	awsAccountID := os.Getenv("CONJUR_AWS_ACCOUNT_ID")
	awsRoleName := os.Getenv("CONJUR_AWS_ROLE_NAME")
	if awsAccountID == "" || awsRoleName == "" {
		t.Fatal("CONJUR_AWS_ACCOUNT_ID and CONJUR_AWS_ROLE_NAME must be set to run this test")
	}

	applianceURL := os.Getenv("CONJUR_APPLIANCE_URL")
	account := os.Getenv("CONJUR_ACCOUNT")

	// Use admin credentials for policy provisioning; tests invoke summon using
	// the CONJUR_AUTHN_* env vars, which are set to the test host identity.
	conjur, err := createAdminConjurClient(applianceURL, account)
	require.NoError(t, err)

	t.Run("retrieves variable via AWS IAM authn", func(t *testing.T) {
		variableIdentifier := fmt.Sprintf("ci/test/%s/db-password", generateRandomString(8))
		rolePolicy := awsRolePolicyTemplate(awsAccountID, awsRoleName, variableIdentifier)
		authnPolicy := awsAuthnPolicyTemplate(awsAccountID, awsRoleName)
		hostID := fmt.Sprintf("%s/%s", awsAccountID, awsRoleName)

		rootCleanup, err := loadPolicy(conjur, "root", rolePolicy)
		require.NoError(t, err)
		defer rootCleanup()

		authnCleanup, err := loadPolicy(conjur, "root", authnPolicy)
		require.NoError(t, err)
		defer authnCleanup()

		require.NoError(t, conjur.EnableAuthenticator("iam", "test", true))

		secretValue, err := addSecretWithRandomValue(conjur, variableIdentifier)
		require.NoError(t, err)

		t.Run("wrong service-id and host-id returns error", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "iam")
			os.Setenv("CONJUR_SERVICE_ID", "nonexistent-service")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", "invalid/host-id")

			_, _, err := runSummonWithVar(variableIdentifier)
			assert.Error(t, err)
		})

		t.Run("valid authn-iam configuration retrieves secret", func(t *testing.T) {
			snap := setupTestEnvironment(t, applianceURL, account, "", "")
			defer snap.RestoreEnv()
			os.Setenv("CONJUR_AUTHN_TYPE", "iam")
			os.Setenv("CONJUR_SERVICE_ID", "test")
			os.Setenv("CONJUR_AUTHN_JWT_HOST_ID", hostID)

			retrieved, stderr, err := runSummonWithVar(variableIdentifier)

			require.NoError(t, err, "summon should exit 0; stderr: %s", stderr.String())
			assert.Equal(t, secretValue, retrieved)
		})
	})
}
