package provider

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPortableProviderPath(t *testing.T) {
	// CAVEAT: only works if no default installation within the test
	// environment exists! getDefaultProvider falls back to portable
	// only if no global install is found

	// GetDefaultPath() now tests that the directory exists, so we have to
	// create it for this test to pass.

	exec, _ := os.Executable()
	execDir := filepath.Dir(exec)
	dir := filepath.Join(execDir, "Providers")

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		// Handle the error
		fmt.Printf("TestDefaultPortableProviderPath: Error creating directory: %s\n", err.Error())
		return
	}
	defer os.RemoveAll(dir)

	defaultPath, err := GetDefaultPath()
	assert.Nil(t, err)
	assert.EqualValues(t, dir, defaultPath)
}

func TestDefaultPortableLibProviderPath(t *testing.T) {
	// This is the case for homebrew installations
	exec, _ := os.Executable()
	execDir := filepath.Dir(exec)
	baseDir := filepath.Dir(execDir)
	dir := filepath.Join(baseDir, "lib", "summon")

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		// Handle the error
		fmt.Printf("TestDefaultPortableLibProviderPath: Error creating directory: %s\n", err.Error())
		return
	}
	defer os.RemoveAll(dir)
	defaultPath, err := GetDefaultPath()
	assert.Nil(t, err)
	assert.EqualValues(t, dir, defaultPath)
}

func TestResolveEmptyProviderFallsBackToBuiltin(t *testing.T) {
	// No SUMMON_PROVIDER and no binary in the default dir — Resolve falls
	// back to the built-in Conjur provider. Without Conjur configuration the
	// init call returns an error, confirming the fallback path was taken.
	// Skip on machines that have a provider binary installed in the default dir
	// (e.g. summon-conjur via Homebrew), where Resolve correctly returns success.
	defaultPath, _ := GetDefaultPath()
	if entries, _ := os.ReadDir(defaultPath); len(entries) > 0 {
		t.Skip("default provider dir is non-empty on this machine; skipping fallback-to-builtin test")
	}

	t.Setenv("SUMMON_PROVIDER", "")
	t.Setenv("CONJUR_APPLIANCE_URL", "")
	t.Setenv("CONJUR_ACCOUNT", "")
	_, err := Resolve("")
	assert.Error(t, err)
}

func TestProviderResolutionOfAbsPath(t *testing.T) {
	expected := "/bin/bash"
	provider, err := Resolve(expected)

	assert.Nil(t, err)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		return
	}

	assert.Equal(t, expected, provider.Path())
}

func TestProviderResolutionOfRelPath(t *testing.T) {
	f, err := os.CreateTemp("", "")
	defer os.RemoveAll(f.Name())
	f.Chmod(0755)

	currentDir, err := os.Getwd()
	assert.Nil(t, err)
	if err != nil {
		return
	}

	relativePath, err := filepath.Rel(currentDir, f.Name())
	assert.Nil(t, err)
	if err != nil {
		return
	}

	provider, err := Resolve(relativePath)
	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.Equal(t, f.Name(), provider.Path())
}

func TestProviderResolutionViaEnvVarOfAbsPath(t *testing.T) {
	expected := "/bin/bash"
	os.Setenv("SUMMON_PROVIDER", expected)
	defer os.Unsetenv("SUMMON_PROVIDER")

	provider, err := Resolve("")

	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.Equal(t, expected, provider.Path())
}

func TestProviderResolutionViaEnvVarOfRelPath(t *testing.T) {
	f, err := os.CreateTemp("", "")
	defer os.RemoveAll(f.Name())
	f.Chmod(0755)

	currentDir, err := os.Getwd()
	assert.Nil(t, err)
	if err != nil {
		return
	}

	relativePath, err := filepath.Rel(currentDir, f.Name())
	assert.Nil(t, err)
	if err != nil {
		return
	}

	os.Setenv("SUMMON_PROVIDER", relativePath)
	defer os.Unsetenv("SUMMON_PROVIDER")

	provider, err := Resolve("")
	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.Equal(t, f.Name(), provider.Path())
}

func TestProviderResolutionViaDefaultPathWithOneProvider(t *testing.T) {
	// Create a temp dir with a single executable binary and point the default
	// path override at it. Resolve("") should return that ExternalProvider.
	dir := t.TempDir()
	binary := filepath.Join(dir, "summon-conjur")
	f, err := os.Create(binary)
	assert.Nil(t, err)
	f.Close()
	err = os.Chmod(binary, 0755)
	assert.Nil(t, err)

	t.Setenv("SUMMON_PROVIDER", "")
	t.Setenv("SUMMON_PROVIDER_PATH", dir)

	provider, err := Resolve("")
	assert.Nil(t, err)
	if err != nil {
		return
	}
	assert.Equal(t, binary, provider.Path())
}

func TestProviderResolutionViaDefaultPathWithMultipleProviders(t *testing.T) {
	// Create a temp dir with two binaries. Resolve("") should return an error
	// because more than one provider is ambiguous.
	dir := t.TempDir()
	for _, name := range []string{"summon-conjur", "summon-aws"} {
		binary := filepath.Join(dir, name)
		f, err := os.Create(binary)
		assert.Nil(t, err)
		f.Close()
		os.Chmod(binary, 0755)
	}

	t.Setenv("SUMMON_PROVIDER", "")
	t.Setenv("SUMMON_PROVIDER_PATH", dir)

	_, err := Resolve("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "More than one provider found")
}

// ---- Resolve (provider instance) ----

func TestResolve(t *testing.T) {
	t.Run("returns ExternalProvider instance for an external provider path", func(t *testing.T) {
		// Use /bin/ls as a stand-in for any executable external provider.
		provider, err := Resolve("/bin/ls")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "/bin/ls", provider.Path())
	})

	t.Run("returns error for builtin when Conjur is not fully configured", func(t *testing.T) {
		for _, envVar := range []string{"CONJUR_APPLIANCE_URL", "CONJUR_ACCOUNT"} {
			t.Setenv(envVar, "")
		}
		_, err := Resolve("conjur")
		assert.Error(t, err)
	})

	t.Run("case-insensitive CONJUR routes to builtin", func(t *testing.T) {
		for _, envVar := range []string{"CONJUR_APPLIANCE_URL", "CONJUR_ACCOUNT"} {
			t.Setenv(envVar, "")
		}
		_, err := Resolve("CONJUR")
		assert.Error(t, err)
	})

	t.Run("mixed-case Conjur routes to builtin", func(t *testing.T) {
		for _, envVar := range []string{"CONJUR_APPLIANCE_URL", "CONJUR_ACCOUNT"} {
			t.Setenv(envVar, "")
		}
		_, err := Resolve("Conjur")
		assert.Error(t, err)
	})
}

func TestProviderCall(t *testing.T) {
	arg := "provider.go"
	out, err := Call("ls", arg)

	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.Equal(t, out, arg)
}

func TestProviderCallWithExecutionError(t *testing.T) {
	err := os.Setenv("LC_ALL", "C")
	assert.Nil(t, err)

	out, err := Call("ls", "README.notafile")

	assert.Empty(t, out)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "No such file or directory")
}

func TestProviderCallWithBadPath(t *testing.T) {
	err := os.Setenv("LC_ALL", "C")
	assert.Nil(t, err)
	if err != nil {
		return
	}

	out, err := Call("/etc/passwd", "foo")

	assert.Empty(t, out)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestGetAllProviders(t *testing.T) {
	pathTo, err := os.Getwd()
	assert.Nil(t, err)
	if err != nil {
		return
	}

	choppedPathTo := strings.TrimSuffix(pathTo, "/provider")
	assert.Equal(t, choppedPathTo, pathTo[0:len(pathTo)-9])

	pathToTest := filepath.Join(choppedPathTo, "command", "testversions")

	output, err := GetAllProviders(pathToTest)
	assert.Nil(t, err)
	if err != nil {
		return
	}

	expected := make([]string, 3)
	expected[0] = "testprovider"
	expected[1] = "testprovider-noversionsupport"
	expected[2] = "testprovider-trailingnewline"

	assert.EqualValues(t, output, expected)
}

func TestGetAllProvidersWithBadPath(t *testing.T) {
	pathTo, err := os.Getwd()
	assert.Nil(t, err)
	if err != nil {
		return
	}

	choppedPathTo := strings.TrimSuffix(pathTo, "/provider")
	assert.Equal(t, choppedPathTo, pathTo[0:len(pathTo)-9])

	pathToTest := filepath.Join(choppedPathTo, "command", "testversions")

	_, err = GetAllProviders(filepath.Join(pathToTest, "makebelievedir"))
	assert.NotNil(t, err)
}

func TestCallInteractiveMode(t *testing.T) {
	t.Run("provider command fails to execute", func(t *testing.T) {
		provider := "/non/existent/command"
		secrets := secretsyml.SecretsMap{
			"key1": secretsyml.SecretSpec{Path: "provider.go"},
		}

		_, errorsCh, cleanup := CallInteractiveMode(provider, secrets)
		defer cleanup()

		select {
		case err := <-errorsCh:
			assert.Error(t, err)
		case <-time.After(1 * time.Second):
			assert.Fail(t, "Timeout waiting for error")
		}
	})

	t.Run("provider command executes successfully", func(t *testing.T) {
		provider, err := createMockProvider()
		assert.NoError(t, err)
		secrets := secretsyml.SecretsMap{
			"key1": secretsyml.SecretSpec{Path: "provider.go"},
		}

		resultsCh, errorsCh, cleanup := CallInteractiveMode(provider, secrets)
		defer cleanup()

		select {
		case result := <-resultsCh:
			assert.NotNil(t, result)
			assert.Nil(t, result.Error)
			assert.Equal(t, "provider.go", result.Value)

		case err := <-errorsCh:
			assert.Fail(t, "Unexpected error: %v", err)
		case <-time.After(1 * time.Second):
			assert.Fail(t, "Timeout waiting for result")
		}
	})

	t.Run("provider command executes successfully with multiple secrets", func(t *testing.T) {
		provider, err := createMockProvider()
		assert.NoError(t, err)
		defer os.Remove(provider)
		secrets := secretsyml.SecretsMap{
			"key1": secretsyml.SecretSpec{Path: "provider.go"},
			"key2": secretsyml.SecretSpec{Path: "provider2.go"},
			"key3": secretsyml.SecretSpec{Path: "provider3.go"},
		}
		results := make(map[string]string)

		resultsCh, errorsCh, cleanup := CallInteractiveMode(provider, secrets)
		defer cleanup()

		for i := 0; i < len(secrets); i++ {
			select {
			case result := <-resultsCh:
				assert.NotNil(t, result)
				assert.Nil(t, result.Error)
				results[result.Key] = result.Value
			case err := <-errorsCh:
				assert.Fail(t, "Unexpected error: %v", err)
			case <-time.After(1 * time.Second):
				assert.Fail(t, "Timeout waiting for result")
			}
		}

		assert.Equal(t, len(secrets), len(results))
		assert.Equal(t, "provider.go", results["key1"])
		assert.Equal(t, "provider2.go", results["key2"])
		assert.Equal(t, "provider3.go", results["key3"])
	})

	t.Run("provider command executes successfully with large number of secrets", func(t *testing.T) {
		provider, err := createMockProvider()
		assert.NoError(t, err)
		defer os.Remove(provider)

		numResults := 500
		secrets := make(secretsyml.SecretsMap, numResults)
		for i := range numResults {
			key := fmt.Sprintf("key%d", i+1)
			path := generateRandomString(100) // Generate a random string of length 100 chars
			secrets[key] = secretsyml.SecretSpec{Path: path}
		}
		results := make(map[string]string)

		resultsCh, errorsCh, cleanup := CallInteractiveMode(provider, secrets)
		defer cleanup()

		for range numResults {
			select {
			case result := <-resultsCh:
				assert.NotNil(t, result)
				assert.Nil(t, result.Error)
				results[result.Key] = result.Value
			case err := <-errorsCh:
				assert.Fail(t, "Unexpected error: %v", err)
			case <-time.After(1 * time.Second):
				assert.Fail(t, "Timeout waiting for result")
			}
		}

		assert.Equal(t, numResults, len(results))
		for i := range numResults {
			key := fmt.Sprintf("key%d", i+1)
			expectedValue := secrets[key].Path
			assert.Equal(t, expectedValue, results[key], "Mismatch for key %s", key)
		}
	})
}

// Mocks the behaviour of a summon provider. The provider reads a list of secrets from stdin
// and outputs the base64 encoded values to the stdout
func createMockProvider() (string, error) {
	// Create a temporary file to act as the mock provider
	tmpfile, err := os.CreateTemp("", "mockprovider")
	if err != nil {
		return "", err
	}

	// Write a script to the temporary file that outputs multiple base64 encoded strings
	script := `#!/bin/bash
    while read -r line; do
        echo $(echo -n $line | base64 -w 0)
    done`
	if _, err := tmpfile.Write([]byte(script)); err != nil {
		return "", err
	}
	if err := tmpfile.Close(); err != nil {
		return "", err
	}

	// Make the file executable
	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

func generateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
	ret := make([]byte, n)
	for i := range n {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		ret[i] = letters[num.Int64()]
	}

	return string(ret)
}

func TestInteractiveModeTimeout(t *testing.T) {
	defaultTimeout := time.Duration(defaultInteractiveTimeout) * time.Second

	tests := []struct {
		envValue string
		expected time.Duration
	}{
		{"", defaultTimeout},
		{"30", 30 * time.Second},
		{"0", defaultTimeout},
		{"-10", defaultTimeout},
		{"abc", defaultTimeout},
	}

	for _, testCase := range tests {
		t.Run(testCase.envValue, func(t *testing.T) {
			os.Setenv(interactiveTimeoutEnvVar, testCase.envValue)
			defer os.Unsetenv(interactiveTimeoutEnvVar)
			assert.Equal(t, testCase.expected, interactiveModeTimeout())
		})
	}
}

func TestIsBuiltinTarget(t *testing.T) {
	tests := []struct {
		name        string
		providerArg string
		envProvider string
		expected    bool
	}{
		// Explicit "conjur" arg (case-insensitive) always routes to built-in.
		{
			name:        "providerArg=conjur always routes to built-in",
			providerArg: "conjur",
			expected:    true,
		},
		{
			name:        "providerArg=CONJUR routes to built-in (case-insensitive)",
			providerArg: "CONJUR",
			expected:    true,
		},
		{
			name:        "providerArg=Conjur routes to built-in (mixed case)",
			providerArg: "Conjur",
			expected:    true,
		},

		// Empty providerArg: only routes to built-in when SUMMON_PROVIDER
		// explicitly names the built-in. Unset/empty env defers to the
		// default-directory lookup in Resolve.
		{
			name:        "providerArg empty and SUMMON_PROVIDER unset defers to dir lookup",
			providerArg: "",
			envProvider: "",
			expected:    false,
		},
		{
			name:        "providerArg empty and SUMMON_PROVIDER=conjur routes to built-in",
			providerArg: "",
			envProvider: "conjur",
			expected:    true,
		},
		{
			name:        "providerArg empty and SUMMON_PROVIDER=CONJUR routes to built-in (case-insensitive)",
			providerArg: "",
			envProvider: "CONJUR",
			expected:    true,
		},

		// FR-004: external-provider cases
		{
			name:        "providerArg=/path/to/binary routes to external",
			providerArg: "/path/to/binary",
			expected:    false,
		},
		{
			name:        "providerArg=summon-conjur routes to external",
			providerArg: "summon-conjur",
			expected:    false,
		},
		{
			name:        "providerArg empty and SUMMON_PROVIDER=summon-conjur routes to external",
			providerArg: "",
			envProvider: "summon-conjur",
			expected:    false,
		},
		{
			name:        "providerArg empty and SUMMON_PROVIDER=/abs/path routes to external",
			providerArg: "",
			envProvider: "/usr/local/bin/summon-conjur",
			expected:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUMMON_PROVIDER", tc.envProvider)

			got := IsBuiltinTarget(tc.providerArg)

			assert.Equal(t, tc.expected, got)
		})
	}
}
