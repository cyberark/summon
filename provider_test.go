package main

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestResolveProvider(t *testing.T) {
	// Giving no provider
	provider, err := resolveProvider("")
	if err == nil {
		t.Error("No error thrown on empty provider")
	}

	// Pass it the provider, as a CLI arg
	expected := "/usr/bin/myprovider"
	provider, _ = resolveProvider(expected)
	if provider != expected {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, provider)
	}

	// Pass as environment variable
	expected = "/opt/providers/custom"
	os.Setenv("CAULDRON_PROVIDER", expected)
	provider, _ = resolveProvider("")
	os.Unsetenv("CAULDRON_PROVIDER")
	if provider != expected {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, provider)
	}

	// Check the provider path
	tempDir, _ := ioutil.TempDir("", "cauldrontest")
	defer os.RemoveAll(tempDir)
	DefaultProviderPath = tempDir

	// One executable
	f, err := ioutil.TempFile(DefaultProviderPath, "")
	provider, _ = resolveProvider("")
	if provider != f.Name() {
		t.Errorf("\nexpected\n%s\ngot\n%s", f.Name(), provider)
	}

	// Two executables
	f, err = ioutil.TempFile(DefaultProviderPath, "")
	provider, err = resolveProvider("")

	if err == nil {
		t.Error("Multiple providers in path did not throw an error!")
	}
}

func TestCallProvider(t *testing.T) {
	// Successful call to provider
	expected := "README.md"
	out, err := callProvider("ls", "README.md")
	if out != expected || err != nil {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, out)
	}
	// Unsuccessful call to provider
	expected = "No such file or directory"
	out, err = callProvider("ls", "README.notafile")
	if !strings.Contains(out, expected) || err == nil {
		t.Errorf("'%s' does not contain '%s'", out, expected)
	}
}
