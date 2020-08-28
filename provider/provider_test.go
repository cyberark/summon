package provider

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoProviderReturnsError(t *testing.T) {
	// Point to a tempdir to avoid pollution from dev env
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	DefaultPath = tempDir

	_, err := Resolve("")
	assert.NotNil(t, err)
}

func TestProviderResolutionOfAbsPath(t *testing.T) {
	expected := "/bin/bash"
	provider, err := Resolve(expected)

	assert.Nil(t, err)
	if err != nil {
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
	tempDir, _ := ioutil.TempDir("", "summontest")
	defer os.RemoveAll(tempDir)
	DefaultPath = tempDir

	f, err := ioutil.TempFile(DefaultPath, "")
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
	DefaultPath = tempDir

	// Create 2 exes in provider path
	f1, _ := ioutil.TempFile(DefaultPath, "")
	f2, _ := ioutil.TempFile(DefaultPath, "")
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

	pathToTest := filepath.Join(choppedPathTo, "internal", "command", "testversions")

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

	pathToTest := filepath.Join(choppedPathTo, "internal", "command", "testversions")

	_, err = GetAllProviders(filepath.Join(pathToTest, "makebelievedir"))
	assert.NotNil(t, err)
}
