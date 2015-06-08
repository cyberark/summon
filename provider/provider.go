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

var DefaultPath = "/usr/libexec/summon"

// Resolve resolves a filepath to a provider
// Checks the CLI arg, environment and then default path
func Resolve(providerArg string) (string, error) {
	provider := ""
	if providerArg != "" {
		provider = providerArg
	}

	envArg := os.Getenv("SUMMON_PROVIDER")
	if envArg != "" {
		provider = envArg
	}

	if provider == "" {
		providers, _ := ioutil.ReadDir(DefaultPath)
		if len(providers) == 1 {
			provider = fullPath(providers[0].Name())
		} else if len(providers) > 1 {
			return "", fmt.Errorf("More than one provider found in %s, please specify one\n", DefaultPath)
		}
	}

	if provider == "" {
		return "", fmt.Errorf("Could not resolve a provider!")
	}
	return provider, nil
}

// Call shells out to a provider and return its output
// If call succeeds, stdout is returned with no error
// If call fails, "" is return with error containing stderr
func Call(provider, specPath string) (string, error) {
	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)
	cmd := exec.Command(provider, specPath)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()

	if err != nil {
		return "", fmt.Errorf(stdErr.String())
	}

	return strings.TrimSpace(stdOut.String()), nil
}

// Given a non-absolute path, returns a path to executable prefixed with DefaultPath
func fullPath(provider string) string {
	if path.IsAbs(provider) {
		return provider
	}
	return path.Join(DefaultPath, provider)
}
