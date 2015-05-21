package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

var DefaultProviderPath = "/usr/libexec/cauldron"

// Resolve a path to a provider
// Checks the CLI arg, environment and then default path
func resolveProvider(providerArg string) (string, error) {
	provider := ""
	if providerArg != "" {
		provider = providerArg
	}

	envArg := os.Getenv("CAULDRON_PROVIDER")
	if envArg != "" {
		provider = envArg
	}

	providers, _ := ioutil.ReadDir(DefaultProviderPath)
	if len(providers) == 1 {
		provider = fmt.Sprintf("%s/%s", DefaultProviderPath, providers[0].Name())
	} else if len(providers) > 1 {
		return "", fmt.Errorf("More than one provider found in %s, please specify one\n", DefaultProviderPath)
	}

	if provider == "" {
		return "", fmt.Errorf("Could not resolve a provider!")
	}
	return provider, nil
}
