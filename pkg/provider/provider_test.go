package provider

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	tempDir, _ := ioutil.TempDir("", "summontest")
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
	f, err := ioutil.TempFile("", "")
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
	f, err := ioutil.TempFile("", "")
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
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	f, err := ioutil.TempFile(tempDir, "")
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
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	os.Setenv("SUMMON_PROVIDER_PATH", tempDir)
	defer os.Setenv("SUMMON_PROVIDER_PATH", "")
	defaultPath, _ := GetDefaultPath()

	f, err := ioutil.TempFile(defaultPath, "")
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
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	defaultPath := tempDir

	// Create 2 exes in provider path
	f1, _ := ioutil.TempFile(defaultPath, "")
	f2, _ := ioutil.TempFile(defaultPath, "")
	defer os.RemoveAll(f1.Name())
	defer os.RemoveAll(f2.Name())

	_, err := Resolve("")

	assert.NotNil(t, err)
}

func TestProviderResolutionViaOverrideDefaultPathWithMultipleProviders(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	os.Setenv("SUMMON_PROVIDER_PATH", tempDir)
	defer os.Setenv("SUMMON_PROVIDER_PATH", "")
	defaultPath, _ := GetDefaultPath()

	// Create 2 exes in provider path
	f1, _ := ioutil.TempFile(defaultPath, "")
	f2, _ := ioutil.TempFile(defaultPath, "")
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
