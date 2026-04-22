//go:build integration

package conjurprovider

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"

	conjurapi "github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/cyberark/conjur-api-go/conjurapi/authn"
)

// PackageName is the name of the binary under test. Integration tests invoke
// the pre-built summon binary at /usr/local/bin/summon via RunCommand.
const PackageName = "summon"

// summonBin is the path to the pre-built summon binary inside the test container.
const summonBin = "/usr/local/bin/summon"

// RunCommand invokes the pre-built summon binary with the given arguments and
// returns stdout, stderr, and any execution error.
func RunCommand(arg ...string) (stdout strings.Builder, stderr strings.Builder, err error) {
	cmd := exec.Command(summonBin, arg...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return
}

// runProgramCommand runs an arbitrary program (e.g. bash) with the given
// arguments. Used for steps that need to invoke something other than summon.
func runProgramCommand(name string, arg ...string) (stdout strings.Builder, stderr strings.Builder, err error) {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return
}

// runSummonWithVar is a convenience wrapper that builds a summon invocation
// to retrieve variablePath into an env var named "SECRET", then runs
// `printenv SECRET` as the subprocess. Returns the printed secret value
// (without trailing newline), stderr, and any error.
func runSummonWithVar(variablePath string, extraArgs ...string) (string, strings.Builder, error) {
	yamlSpec := fmt.Sprintf("SECRET: !var %s", variablePath)
	args := append(extraArgs, "--yaml", yamlSpec, "--", "printenv", "SECRET")
	stdout, stderr, err := RunCommand(args...)
	return strings.TrimRight(stdout.String(), "\n"), stderr, err
}

// createAdminConjurClient creates a Conjur API client using admin credentials
// (CONJUR_ADMIN_LOGIN / CONJUR_ADMIN_API_KEY). Used for policy provisioning in
// integration tests; the test host identity is used for summon invocations.
func createAdminConjurClient(applianceURL, account string) (*conjurapi.Client, error) {
	adminLogin := os.Getenv("CONJUR_ADMIN_LOGIN")
	adminAPIKey := os.Getenv("CONJUR_ADMIN_API_KEY")
	return createConjurClient(applianceURL, account, adminLogin, adminAPIKey)
}

// createConjurClient creates an admin Conjur API client from explicit credentials.
func createConjurClient(applianceURL, account, login, apiKey string) (*conjurapi.Client, error) {
	config := conjurapi.Config{
		ApplianceURL: applianceURL,
		Account:      account,
	}
	return conjurapi.NewClientFromKey(
		config,
		authn.LoginPair{Login: login, APIKey: apiKey},
	)
}

// loadPolicy loads a policy into the given Conjur branch and returns a cleanup
// function that empties the branch on test teardown.
func loadPolicy(conjur *conjurapi.Client, branch, policy string) (func(), error) {
	_, err := conjur.LoadPolicy(
		conjurapi.PolicyModePost,
		branch,
		strings.NewReader(policy),
	)
	if err != nil {
		return nil, err
	}

	return func() {
		conjur.LoadPolicy( //nolint:errcheck
			conjurapi.PolicyModePut,
			branch,
			strings.NewReader(""),
		)
	}, nil
}

// setupVariablePolicy loads a root policy that declares one or more variables
// and grants the test host (host/ci/summon/test-host) read/execute access.
func setupVariablePolicy(conjur *conjurapi.Client, variableIdentifiers ...string) (func(), error) {
	policyBuilder := strings.Builder{}
	for _, identifier := range variableIdentifiers {
		policyBuilder.WriteString(fmt.Sprintf("- !variable %s\n", identifier))
	}
	// Grant the CI test host access to each declared variable
	for _, identifier := range variableIdentifiers {
		policyBuilder.WriteString(fmt.Sprintf(`- !permit
  role: !host ci/summon/test-host
  privilege: [ read, execute ]
  resource: !variable %s
`, identifier))
	}
	return loadPolicy(conjur, "root", policyBuilder.String())
}

// addSecretWithRandomValue sets a random value for a Conjur variable and
// returns the generated value.
func addSecretWithRandomValue(conjur *conjurapi.Client, variableIdentifier string) (string, error) {
	secretValue := fmt.Sprintf("secret-value-%s", generateRandomString(12))
	if err := conjur.AddSecret(variableIdentifier, secretValue); err != nil {
		return "", err
	}
	return secretValue, nil
}

// generateRandomString returns a random alphanumeric string of length n.
func generateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
	ret := make([]byte, n)
	for i := range n {
		ret[i] = letters[rand.Intn(len(letters))]
	}
	return string(ret)
}

// setupTestEnvironment clears env and sets Conjur connection variables.
// Returns an envSnapshot so the caller can restore the original environment.
func setupTestEnvironment(t *testing.T, applianceURL, account, login, apiKey string) *envSnapshot {
	t.Helper()
	savedPath := os.Getenv("PATH") // capture before ClearEnv removes it
	snap := ClearEnv()
	os.Setenv("PATH", savedPath)
	os.Setenv("CONJUR_APPLIANCE_URL", applianceURL)
	os.Setenv("CONJUR_ACCOUNT", account)
	os.Setenv("CONJUR_IGNORE_UNTRUSTED_SSL", "true")
	if login != "" {
		os.Setenv("CONJUR_AUTHN_LOGIN", login)
	}
	if apiKey != "" {
		os.Setenv("CONJUR_AUTHN_API_KEY", apiKey)
	}
	os.Setenv("HOME", "/root")
	return snap
}

type envSnapshot struct {
	env []string
}

func splitEq(s string) (string, string) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// ClearEnv unsets all environment variables and returns a snapshot so they can
// be restored via RestoreEnv.
func ClearEnv() *envSnapshot {
	snap := os.Environ()
	for _, s := range snap {
		k, _ := splitEq(s)
		os.Unsetenv(k)
	}
	return &envSnapshot{env: snap}
}

// RestoreEnv restores the environment to the state captured by ClearEnv.
func (e *envSnapshot) RestoreEnv() {
	// Clear current env first
	for _, s := range os.Environ() {
		k, _ := splitEq(s)
		os.Unsetenv(k)
	}
	for _, s := range e.env {
		k, v := splitEq(s)
		os.Setenv(k, v)
	}
}
