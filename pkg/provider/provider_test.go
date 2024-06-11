package provider

import (
	"fmt"
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

func TestNoProviderReturnsError(t *testing.T) {
	// Point to a tempdir to avoid pollution from dev env
	tempDir, _ := os.MkdirTemp("", "summontest")
	defer os.RemoveAll(tempDir)

	_, err := Resolve("")
	assert.NotNil(t, err)
}

func TestProviderResolutionOfAbsPath(t *testing.T) {
	expected := "/bin/bash"
	provider, err := Resolve(expected)

	assert.Nil(t, err)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		return
	}

	assert.EqualValues(t, provider, expected)
}

func TestProviderResolutionOfRelPath(t *testing.T) {
	f, err := os.CreateTemp("", "")
	defer os.RemoveAll(f.Name())
	f.Chmod(755)

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

	assert.EqualValues(t, provider, f.Name())
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

	assert.EqualValues(t, provider, expected)
}

func TestProviderResolutionViaEnvVarOfRelPath(t *testing.T) {
	f, err := os.CreateTemp("", "")
	defer os.RemoveAll(f.Name())
	f.Chmod(755)

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

	assert.EqualValues(t, provider, f.Name())
}

func TestProviderResolutionViaDefaultPathWithOneProvider(t *testing.T) {
	// Create tmpdir with single executable file
	tempDir, _ := os.MkdirTemp("", "summontest")
	defer os.RemoveAll(tempDir)
	f, err := os.CreateTemp(tempDir, "")
	defer os.RemoveAll(f.Name())
	f.Chmod(755)

	// DefaultPath is no longer a global, so use
	// the env var instead.
	os.Setenv("SUMMON_PROVIDER_PATH", tempDir)

	provider, err := Resolve("")

	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.EqualValues(t, provider, f.Name())
}

func TestProviderResolutionViaOverrideDefaultPathWithOneProvider(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "summontest")
	defer os.RemoveAll(tempDir)
	os.Setenv("SUMMON_PROVIDER_PATH", tempDir)
	defer os.Setenv("SUMMON_PROVIDER_PATH", "")
	defaultPath, _ := GetDefaultPath()

	f, err := os.CreateTemp(defaultPath, "")
	defer os.RemoveAll(f.Name())
	f.Chmod(755)
	provider, err := Resolve("")

	assert.Nil(t, err)
	if err != nil {
		return
	}

	assert.EqualValues(t, provider, f.Name())
}

func TestProviderResolutionViaDefaultPathWithMultipleProviders(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "summontest")
	defer os.RemoveAll(tempDir)
	defaultPath := tempDir

	// Create 2 exes in provider path
	f1, _ := os.CreateTemp(defaultPath, "")
	f2, _ := os.CreateTemp(defaultPath, "")
	defer os.RemoveAll(f1.Name())
	defer os.RemoveAll(f2.Name())

	_, err := Resolve("")

	assert.NotNil(t, err)
}

func TestProviderResolutionViaOverrideDefaultPathWithMultipleProviders(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "summontest")
	defer os.RemoveAll(tempDir)
	os.Setenv("SUMMON_PROVIDER_PATH", tempDir)
	defer os.Setenv("SUMMON_PROVIDER_PATH", "")
	defaultPath, _ := GetDefaultPath()

	// Create 2 exes in provider path
	f1, _ := os.CreateTemp(defaultPath, "")
	f2, _ := os.CreateTemp(defaultPath, "")
	defer os.RemoveAll(f1.Name())
	defer os.RemoveAll(f2.Name())

	_, err := Resolve("")

	assert.NotNil(t, err)
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
        echo $(echo -n $line | base64)
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
