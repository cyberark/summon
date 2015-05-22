package provider

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

var DefaultProviderPath = "/usr/libexec/cauldron"

// Resolves a path to a provider
// Checks the CLI arg, environment and then default path
func ResolveProvider(providerArg string) (string, error) {
	provider := ""
	if providerArg != "" {
		provider = providerArg
	}

	envArg := os.Getenv("CAULDRON_PROVIDER")
	if envArg != "" {
		provider = envArg
	}

	if provider == "" {
		providers, _ := ioutil.ReadDir(DefaultProviderPath)
		if len(providers) == 1 {
			provider = fullPath(providers[0].Name())
		} else if len(providers) > 1 {
			return "", fmt.Errorf("More than one provider found in %s, please specify one\n", DefaultProviderPath)
		}
	}

	if provider == "" {
		return "", fmt.Errorf("Could not resolve a provider!")
	}
	return provider, nil
}

// Shell out to a provider and return its output
func CallProvider(provider, specPath string) (string, error) {
	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)
	cmd := exec.Command(provider, specPath)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()

	if err != nil {
		return stdErr.String(), err
	}

	return strings.TrimSpace(stdOut.String()), nil
}

// Given a non-absolute path, returns a path to executable prefixed with DefaultProviderPath
func fullPath(provider string) string {
	if path.IsAbs(provider) {
		println("abs!")
		return provider
	}
	return path.Join(DefaultProviderPath, provider)
}
