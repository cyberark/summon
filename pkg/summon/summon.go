package summon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/pushtofile"
	"github.com/cyberark/summon/pkg/secretsyml"
)

// SubprocessConfig is an object that holds all the info needed to run
// a Summon instance
type SubprocessConfig struct {
	Args        []string
	Provider    string
	Filepath    string
	YamlInline  string
	Subs        []string
	Ignores     []string
	IgnoreAll   bool
	Environment string
	RecurseUp   bool
	FetchSecret secretFetcher
	// VerifyProvider, when set, is called before every exec of an external
	// provider binary; an error fails the fetch without running it.
	VerifyProvider func() error
	// Refresh enables daemon mode: secrets are re-fetched on an interval while
	// the subprocess runs. Nil means one-shot.
	Refresh *RefreshConfig
}

const envFileMagic = "@SUMMONENVFILE"
const summonEnvKeyName = "SUMMON_ENV"

// secretFetcher is function signature for fetching a secret
type secretFetcher func(ctx context.Context, id string) ([]byte, error)

// RunSubprocess encapsulates the logic of fetching secrets, executing the subprocess with the secrets injected.
func RunSubprocess(sc *SubprocessConfig) (int, error) {
	return runSubprocess(context.Background(), sc)
}

// runSubprocess is RunSubprocess with a context that, in daemon mode, stops
// the subprocess and ends the run when cancelled.
func runSubprocess(ctx context.Context, sc *SubprocessConfig) (int, error) {
	// Scrub CONJUR_* from this process on every return path (including fetch
	// errors). An explicit clear also runs immediately before exec so
	// credentials are gone while the child is running, not only after Wait.
	defer clearConjurEnvFromProcess()

	// Prepare substitutions map from command line arguments
	subs, err := convertSubsToMap(sc.Subs)
	if err != nil {
		return 0, err
	}

	// Optional recursive search for secrets file up the directory tree
	if sc.RecurseUp {
		currentDir, err := os.Getwd()
		if err != nil {
			return 0, err
		}
		sc.Filepath, err = findInParentTree(sc.Filepath, currentDir)
		if err != nil {
			return 0, err
		}
	}

	// Parse the secrets configuration from a file or inline YAML
	var config *secretsyml.ParsedConfig
	if sc.YamlInline == "" {
		slog.Debug("Loading summon configuration", "filename", sc.Filepath)
		config, err = secretsyml.ParseFromFile(sc.Filepath, sc.Environment, subs)
		if err != nil {
			return 0, fmt.Errorf("Unable to parse configuration from %s: %w", sc.Filepath, err)
		}
	} else {
		slog.Debug("Loading summon configuration from inline YAML")
		config, err = secretsyml.ParseFromString(sc.YamlInline, sc.Environment, subs)
		if err != nil {
			return 0, fmt.Errorf("Unable to parse configuration from inline YAML: %w", err)
		}
	}

	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	if sc.Refresh != nil {
		return runRefreshing(ctx, sc, config, &tempFactory)
	}

	fetched, err := fetchCycle(context.Background(), config, sc, &tempFactory)
	if err != nil {
		return 0, err
	}
	env := fetched.env

	if config.HasFileSecrets() {
		err = processResultsAndSetupFiles(fetched.fileResults, config.Files, sc, &tempFactory)
		if err != nil {
			return 0, err
		}
	}

	// Clear before exec so CONJUR_* are gone while the child runs.
	// filterConjurEnv remains defense in depth for the child env slice.
	clearConjurEnvFromProcess()
	err = runSubcommand(sc.Args, append(filterConjurEnv(os.Environ()), env...))
	if err != nil {
		return returnStatusOfError(err)
	}

	return 0, nil
}

// fetchedCycle is everything one fetch cycle produces before any file is
// written.
type fetchedCycle struct {
	// env is the env-injected secrets as "key=value" entries.
	env []string
	// args is the subprocess command with @SUMMONENVFILE resolved.
	args []string
	// envResults and fileResults are the raw provider results.
	envResults  []prov.Result
	fileResults []prov.Result
}

// fetchCycle fetches the env and file secrets and builds the subprocess env.
// Files are not written: callers decide how (one-shot or checksum-gated).
//
// Note: This implementation will cause duplicate calls to the provider if
// there are secrets needed for both env and files. We can optimize this in
// the future by calling the provider once and then splitting the results
// based on whether they're needed for env or files. We can do this by
// creating a Set of all the secret paths to fetch, calling the provider
// once with that set, and then processing the results to populate both env
// and files as needed.
func fetchCycle(ctx context.Context, config *secretsyml.ParsedConfig, sc *SubprocessConfig, tempFactory *TempFactory) (*fetchedCycle, error) {
	fetched := &fetchedCycle{env: []string{}}

	// Fetch secrets needed for environment variables
	if config.HasEnvSecrets() {
		envResults, err := fetchSecrets(ctx, config.EnvSecrets, sc, tempFactory)
		if err != nil {
			return nil, err
		}
		fetched.envResults = envResults
		fetched.env, err = processResultsAndSetupEnv(envResults, sc, tempFactory)
		if err != nil {
			return nil, err
		}
	}

	if config.HasFileSecrets() {
		fileResults, err := fetchSecrets(ctx, config.FileSecrets(), sc, tempFactory)
		if err != nil {
			return nil, err
		}
		fetched.fileResults = fileResults
	}

	fetched.args = sc.Args
	return fetched, nil
}

// clearConjurEnvFromProcess unsets every environment variable whose key starts
// with "CONJUR_" or is in conjurEnvDenyList in the current process.
// Safe to call more than once.
func clearConjurEnvFromProcess() {
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "CONJUR_") {
			_ = os.Unsetenv(key)
			continue
		}
		for _, deny := range conjurEnvDenyList {
			if key == deny {
				_ = os.Unsetenv(key)
				break
			}
		}
	}
}

// conjurEnvDenyList contains exact variable names that must be stripped from the
// child process environment in addition to any CONJUR_-prefixed variables.
// These are auth-related variables that do not share the CONJUR_ prefix.
var conjurEnvDenyList = []string{
	"JWT_TOKEN_PATH",
	"CONJURRC",
}

// filterConjurEnv returns a copy of env with Conjur auth variables removed.
// Strips all CONJUR_-prefixed entries plus every key in conjurEnvDenyList.
func filterConjurEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CONJUR_") {
			continue
		}
		denied := false
		for _, deny := range conjurEnvDenyList {
			if key == deny {
				denied = true
				break
			}
		}
		if !denied {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// findInParentTree recursively searches for secretsFile starting at leafDir and in the
// directories above leafDir until it is found or the root of the file system is reached.
// If found, returns the absolute path to the file.
func findInParentTree(secretsFile string, leafDir string) (string, error) {
	if filepath.IsAbs(secretsFile) {
		return "", fmt.Errorf(
			"file specified (%s) is an absolute path: will not recurse up", secretsFile)
	}

	for {
		joinedPath := filepath.Join(leafDir, secretsFile)

		_, err := os.Stat(joinedPath)

		if err != nil {
			// If the file is not present, we just move up one level and run the next loop
			// iteration
			if os.IsNotExist(err) {
				upOne := filepath.Dir(leafDir)
				if upOne == leafDir {
					return "", fmt.Errorf(
						"unable to locate file specified (%s): reached root of file system", secretsFile)
				}

				leafDir = upOne
				continue
			}

			// If we have an unexpected error, we fail-fast
			return "", fmt.Errorf("unable to locate file specified (%s): %s", secretsFile, err)
		}

		// If there's no error, we found the file so we return it
		return joinedPath, nil
	}
}

// convertSubsToMap converts the list of substitutions passed in via
// command line to a map
func convertSubsToMap(subs []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.SplitN(sub, "=", 2)
		if len(s) < 2 {
			return nil, fmt.Errorf("invalid substitution format: %q (expected key=value)", sub)
		}
		key, val := s[0], s[1]
		out[key] = val
	}
	return out, nil
}

// processResultsAndSetupEnv processes provider results, populates the environment map,
// and sets up the environment file. It handles error cases with ignore logic.
func processResultsAndSetupEnv(results []prov.Result, sc *SubprocessConfig, tempFactory *TempFactory) ([]string, error) {
	env := make(map[string]string)
	// Process results from the provider and add them to the environment
	for _, envvar := range results {
		if envvar.Error == nil {
			env[envvar.Key] = envvar.Value
			continue
		}

		if sc.IgnoreAll || slices.Contains(sc.Ignores, envvar.Key) {
			continue
		}

		slog.Debug("Error fetching secret", "name", envvar.Key, "error", envvar.Error)
		return nil, fmt.Errorf("Error fetching secret: %w", envvar.Error)
	}

	// Append environment variable if one is specified
	if sc.Environment != "" {
		env[summonEnvKeyName] = sc.Environment
	}

	// Setup the environment file
	_, err := setupEnvFile(sc.Args, env, tempFactory)
	if err != nil {
		return nil, fmt.Errorf("Error creating %s: %v", envFileMagic, err)
	}

	// Convert env map to slice of strings in "key=value" format for exec.Command
	var e []string
	for k, v := range env {
		e = append(e, fmt.Sprintf("%s=%s", k, v))
	}

	return e, nil
}

// scans arguments for the magic string; if found,
// creates a tempfile to which all the environment mappings are dumped
// and replaces the magic string with its path.
// Returns the path if so, returns an empty string otherwise. Also
// returns any error encountered during the process.
func setupEnvFile(args []string, env map[string]string, tempFactory *TempFactory) (string, error) {
	var envFile = ""
	var err error

	for i, arg := range args {
		found := strings.Contains(arg, envFileMagic)
		if !found {
			continue
		}

		if envFile == "" {
			envFile, err = tempFactory.Push(joinEnv(env))
			if err != nil {
				return "", err
			}
		}
		args[i] = strings.ReplaceAll(arg, envFileMagic, envFile)
	}

	return envFile, nil
}

func processResultsAndSetupFiles(results []prov.Result, filesConfig []secretsyml.FileConfig, sc *SubprocessConfig, tempFactory *TempFactory) error {
	for _, file := range filesConfig {
		if err := file.Validate(); err != nil {
			return err
		}

		err := createFile(file, results, sc, tempFactory)
		if err != nil {
			return err
		}
	}

	return nil
}

func createFile(fileConfig secretsyml.FileConfig, results []prov.Result, sc *SubprocessConfig, tempFactory *TempFactory) error {
	secretFile := pushtofile.SecretFile{
		FileConfig: fileConfig,
		Ignores:    sc.Ignores,
		IgnoreAll:  sc.IgnoreAll,
	}

	filePath, err := secretFile.Write(results)
	if err != nil {
		return fmt.Errorf("error writing secret file for path %s: %v", fileConfig.Path, err)
	}
	tempFactory.AddFile(filePath)
	return nil
}
