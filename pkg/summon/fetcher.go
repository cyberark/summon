package summon

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
)

// fetchSecrets encapsulates the logic of fetching secrets from the provider, including filtering non-variable secrets,
// handling results from the provider, and falling back to non-interactive mode if necessary.
func fetchSecrets(secrets secretsyml.SecretsMap, sc *SubprocessConfig, tempFactory *TempFactory) ([]prov.Result, error) {
	var results []prov.Result

	slog.Debug("Fetching secrets", "count", len(secrets), "provider", sc.Provider)

	// Filter out non variables
	filteredResults, filteredSecrets := filterNonVariables(secrets, tempFactory)
	results = append(results, filteredResults...)

	// Call provider with no arguments
	resultsCh, errorsCh, cleanup := prov.CallInteractiveMode(sc.Provider, filteredSecrets)
	defer cleanup()

	// This extracts the logic of handling results from provider interactive mode
	resultsFromProvider, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, tempFactory)
	results = append(results, resultsFromProvider...)

	if err != nil {
		results = nonInteractiveProviderFallback(secrets, sc, tempFactory)
	}

	return results, nil
}

func filterNonVariables(secrets secretsyml.SecretsMap, tempFactory *TempFactory) ([]prov.Result, secretsyml.SecretsMap) {
	filteredSecrets := make(secretsyml.SecretsMap)
	results := []prov.Result{}

	for key, spec := range secrets {
		if spec.IsVar() {
			filteredSecrets[key] = spec
		} else {
			k, v, err := formatForEnv(key, spec.Path, spec, tempFactory)
			var result prov.Result
			if err != nil {
				result = prov.Result{Key: key, Value: "", Error: err}
			} else {
				result = prov.Result{Key: k, Value: v, Error: nil}
			}
			results = append(results, result)
		}
	}

	return results, filteredSecrets
}

func handleResultsFromProvider(resultsCh chan prov.Result, errorsCh chan error,
	filteredSecrets secretsyml.SecretsMap, tempFactory *TempFactory) (results []prov.Result, err error) {
	for {
		select {
		case result, ok := <-resultsCh:
			if !ok {
				return results, nil
			}

			spec := filteredSecrets[result.Key]

			// Set a default value if the provider didn't return one for the item
			if result.Value == "" && spec.DefaultValue != "" {
				result.Value = spec.DefaultValue
			}
			k, v, err := formatForEnv(result.Key, result.Value, spec, tempFactory)
			if err != nil {
				result = prov.Result{Key: result.Key, Value: "", Error: err}
			} else {
				result = prov.Result{Key: k, Value: v, Error: nil}
			}
			results = append(results, result)

		// Fallback to the old implementation if either provider doesn't support interactive mode or an error occured
		case err = <-errorsCh:
			return nil, err
		}
	}
}

func nonInteractiveProviderFallback(secrets secretsyml.SecretsMap, sc *SubprocessConfig, tempFactory *TempFactory) []prov.Result {
	results := make(chan prov.Result, len(secrets))
	var wg sync.WaitGroup

	for key, spec := range secrets {
		wg.Add(1)
		go func(key string, spec secretsyml.SecretSpec) {
			defer wg.Done()

			var value string
			if spec.IsVar() {
				slog.Debug("Fetching secret", "name", key, "provider", sc.Provider)
				valueBytes, err := sc.FetchSecret(spec.Path)
				if err != nil {
					results <- prov.Result{Key: key, Value: "", Error: err}
					return
				}
				value = string(valueBytes)
				clear(valueBytes)
			} else {
				// If the spec isn't a variable, use its value as-is
				value = spec.Path
			}

			// Set a default value if the provider didn't return one for the item
			if value == "" && spec.DefaultValue != "" {
				value = spec.DefaultValue
			}

			k, v, err := formatForEnv(key, value, spec, tempFactory)
			if err != nil {
				results <- prov.Result{Key: key, Value: "", Error: err}
			} else {
				results <- prov.Result{Key: k, Value: v, Error: nil}
			}
		}(key, spec)
	}
	wg.Wait()
	close(results)

	resultsSlice := make([]prov.Result, 0, len(secrets))
	for result := range results {
		resultsSlice = append(resultsSlice, result)
	}
	return resultsSlice
}

func returnStatusOfError(err error) (int, error) {
	if eerr, ok := err.(*exec.ExitError); ok {
		if ws, ok := eerr.Sys().(syscall.WaitStatus); ok {
			if ws.Exited() {
				return ws.ExitStatus(), nil
			}
		}
	}
	return 0, err
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) (string, string, error) {
	if spec.IsFile() {
		fname, err := tempFactory.Push(value)
		if err != nil {
			return "", "", err
		}
		value = fname
	}

	return key, value, nil
}

func joinEnv(env map[string]string) string {
	var envs []string
	for k, v := range env {
		if strings.ContainsAny(v, " \t\n\r\"'\\") {
			v = strconv.Quote(v)
		}
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	// Sort to ensure predictable results
	sort.Strings(envs)

	return strings.Join(envs, "\n") + "\n"
}
