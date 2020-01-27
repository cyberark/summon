package command

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/codegangsta/cli"
	prov "github.com/cyberark/summon/provider"
	"github.com/cyberark/summon/secretsyml"
)

// ActionConfig is an object that holds all the info needed to run
// a Summon instance
type ActionConfig struct {
	Args                 []string
	Provider             string
	Filepath             string
	YamlInline           string
	Subs                 map[string]string
	Ignores              []string
	IgnoreAll            bool
	Environment          string
	RecurseUp            bool
	ShowProviderVersions bool
}

const ENV_FILE_MAGIC = "@SUMMONENVFILE"
const SUMMON_ENV_KEY_NAME = "SUMMON_ENV"

// Action is the runner for the main program logic
var Action = func(c *cli.Context) {
	if !c.Args().Present() && !c.Bool("all-provider-versions") {
		fmt.Println("Enter a subprocess to run!")
		os.Exit(127)
	}

	provider, err := prov.Resolve(c.String("provider"))
	// It's okay to not throw this error here, because `Resolve()` throws an
	// error if there are multiple unspecified providers. `all-provider-versions`
	// doesn't care about this and just looks in the default provider dir
	if err != nil && !c.Bool("all-provider-versions") {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	err = runAction(&ActionConfig{
		Args:                 c.Args(),
		Provider:             provider,
		Environment:          c.String("environment"),
		Filepath:             c.String("f"),
		YamlInline:           c.String("yaml"),
		Ignores:              c.StringSlice("ignore"),
		IgnoreAll:            c.Bool("ignore-all"),
		RecurseUp:            c.Bool("-u"),
		ShowProviderVersions: c.Bool("all-provider-versions"),
		Subs:                 convertSubsToMap(c.StringSlice("D")),
	})

	code, err := returnStatusOfError(err)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	os.Exit(code)
}

// runAction encapsulates the logic of Action without cli Context for easier testing
func runAction(ac *ActionConfig) error {
	var (
		secrets secretsyml.SecretsMap
		err     error
	)

	if ac.ShowProviderVersions {
		output, err := printProviderVersions(prov.DefaultPath)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	}

	if ac.RecurseUp {
		currentDir, err := os.Getwd()
		err = walkFn(&ac.Filepath, currentDir)
		if err != nil {
			return err
		}
	}

	switch ac.YamlInline {
	case "":
		secrets, err = secretsyml.ParseFromFile(ac.Filepath, ac.Environment, ac.Subs)
	default:
		secrets, err = secretsyml.ParseFromString(ac.YamlInline, ac.Environment, ac.Subs)
	}

	if err != nil {
		return err
	}

	var env []string
	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	type Result struct {
		string
		error
	}

	// Run provider calls concurrently
	results := make(chan Result, len(secrets))
	var wg sync.WaitGroup

	for key, spec := range secrets {
		wg.Add(1)
		go func(key string, spec secretsyml.SecretSpec) {
			var value string
			if spec.IsVar() {
				value, err = prov.Call(ac.Provider, spec.Path)
				if err != nil {
					results <- Result{key, err}
					wg.Done()
					return
				}
			} else {
				// If the spec isn't a variable, use its value as-is
				value = spec.Path
			}

			// Set a default value if the provider didn't return one for the item
			if value == "" && spec.DefaultValue != "" {
				value = spec.DefaultValue
			}

			envvar := formatForEnv(key, value, spec, &tempFactory)
			results <- Result{envvar, nil}
			wg.Done()
		}(key, spec)
	}
	wg.Wait()
	close(results)

EnvLoop:
	for envvar := range results {
		if envvar.error == nil {
			env = append(env, envvar.string)
		} else {
			if ac.IgnoreAll {
				continue EnvLoop
			}

			for i := range ac.Ignores {
				if ac.Ignores[i] == envvar.string {
					continue EnvLoop
				}
			}
			return fmt.Errorf("Error fetching variable %v: %v", envvar.string, envvar.error.Error())
		}
	}

	// Append environment variable if one is specified
	if ac.Environment != "" {
		env = append(env, fmt.Sprintf("%s=%s", SUMMON_ENV_KEY_NAME, ac.Environment))
	}

	setupEnvFile(ac.Args, env, &tempFactory)

	return runSubcommand(ac.Args, append(os.Environ(), env...))
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) string {
	if spec.IsFile() {
		fname := tempFactory.Push(value)
		value = fname
	}

	return fmt.Sprintf("%s=%s", key, value)
}

func joinEnv(env []string) string {
	return strings.Join(env, "\n") + "\n"
}

func walkFn(file *string, path string) error {
	for {
		joinedPath := filepath.Join(path, *file)
		if _, err := os.Stat(joinedPath); err == nil {
			// File found - store the current filepath
			*file = joinedPath
			return nil
		} else if os.IsNotExist(err) {
			// Move up to parent dir
			path = filepath.Dir(path)
		} else {
			return fmt.Errorf("unable to locate file specified")
		}
	}
}

// scans arguments for the magic string; if found,
// creates a tempfile to which all the environment mappings are dumped
// and replaces the magic string with its path.
// Returns the path if so, returns an empty string otherwise.
func setupEnvFile(args []string, env []string, tempFactory *TempFactory) string {
	var envFile = ""

	for i, arg := range args {
		idx := strings.Index(arg, ENV_FILE_MAGIC)
		if idx >= 0 {
			if envFile == "" {
				envFile = tempFactory.Push(joinEnv(env))
			}
			args[i] = strings.Replace(arg, ENV_FILE_MAGIC, envFile, -1)
		}
	}

	return envFile
}

// convertSubsToMap converts the list of substitutions passed in via
// command line to a map
func convertSubsToMap(subs []string) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.SplitN(sub, "=", 2)
		key, val := s[0], s[1]
		out[key] = val
	}
	return out
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

// printProviderVersions returns a string of all provider versions
func printProviderVersions(providerPath string) (string, error) {
	var providerVersions bytes.Buffer

	providers, err := prov.GetAllProviders(providerPath)
	if err != nil {
		return "", err
	}

	for _, provider := range providers {
		version, err := exec.Command(path.Join(providerPath, provider), "--version").Output()
		if err != nil {
			providerVersions.WriteString(fmt.Sprintf("%s: unknown version\n", provider))
			continue
		}

		versionString := fmt.Sprintf("%s", version)
		versionString = strings.TrimSpace(versionString)

		providerVersions.WriteString(fmt.Sprintf("%s version %s\n", provider, versionString))
	}
	return providerVersions.String(), nil
}
