package summon

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
}

const envFileMagic = "@SUMMONENVFILE"
const summonEnvKeyName = "SUMMON_ENV"

// secretFetcher is function signature for fetching a secret
type secretFetcher func(string) ([]byte, error)

// RunSubprocess encapsulates the logic of fetching secrets, executing the subprocess with the secrets injected.
func RunSubprocess(sc *SubprocessConfig) (int, error) {
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
	switch sc.YamlInline {
	case "":
		slog.Debug("Loading summon configuration", "filename", sc.Filepath)
		config, err = secretsyml.ParseFromFile(sc.Filepath, sc.Environment, subs)
		if err != nil {
			return 0, fmt.Errorf("Unable to parse configuration from %s: %w", sc.Filepath, err)
		}
	default:
		slog.Debug("Loading summon configuration from inline YAML")
		config, err = secretsyml.ParseFromString(sc.YamlInline, sc.Environment, subs)
		if err != nil {
			return 0, fmt.Errorf("Unable to parse configuration from inline YAML: %w", err)
		}
	}

	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	// Note: This implementation will cause duplicate calls to the provider if
	// there are secrets needed for both env and files. We can optimize this in
	// the future by calling the provider once and then splitting the results
	// based on whether they're needed for env or files. We can do this by
	// creating a Set of all the secret paths to fetch, calling the provider
	// once with that set, and then processing the results to populate both env
	// and files as needed.

	env := []string{}
	// Fetch secrets needed for environment variables
	if config.HasEnvSecrets() {
		envResults, err := fetchSecrets(config.EnvSecrets, sc, &tempFactory)
		if err != nil {
			return 0, err
		}
		env, err = processResultsAndSetupEnv(envResults, sc, &tempFactory)
		if err != nil {
			return 0, err
		}
	}

	if config.HasFileSecrets() {
		fileResults, err := fetchSecrets(config.FileSecrets(), sc, &tempFactory)
		if err != nil {
			return 0, err
		}
		err = processResultsAndSetupFiles(fileResults, config.Files, sc, &tempFactory)
		if err != nil {
			return 0, err
		}
	}

	err = runSubcommand(sc.Args, append(os.Environ(), env...))
	if err != nil {
		return returnStatusOfError(err)
	}

	return 0, nil
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
EnvLoop:
	for _, envvar := range results {
		if envvar.Error == nil {
			env[envvar.Key] = envvar.Value
		} else {
			if sc.IgnoreAll {
				continue EnvLoop
			}

			for i := range sc.Ignores {
				if sc.Ignores[i] == envvar.Key {
					continue EnvLoop
				}
			}
			slog.Debug("Error fetching secret", "name", envvar.Key, "error", envvar.Error)
			return nil, fmt.Errorf("Error fetching secret: %v", envvar.Error.Error())
		}
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
