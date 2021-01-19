package command

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	StdIn                io.Reader
	StdOut               io.Writer
	StdErr               io.Writer
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

const EnvFileMagic = "@SUMMONENVFILE"
const DockerArgsMagic = "@SUMMONDOCKERARGS"
const SummonEnvKeyName = "SUMMON_ENV"

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
		RecurseUp:            c.Bool("up"),
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
		ac.Filepath, err = findInParentTree(ac.Filepath, currentDir)
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

	env := make(map[string]string)
	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	type Result struct {
		key   string
		value string
		error
	}

	// Run provider calls concurrently
	results := make(chan Result, len(secrets))
	var wg sync.WaitGroup

	var dockerArgs []string
	var dockerArgsMutex sync.Mutex

	for key, spec := range secrets {
		wg.Add(1)
		go func(key string, spec secretsyml.SecretSpec) {
			var value string
			if spec.IsVar() {
				value, err = prov.Call(ac.Provider, spec.Path)
				if err != nil {
					results <- Result{key, "", err}
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

			k, v := formatForEnv(key, value, spec, &tempFactory)

			// Generate @SUMMONDOCKERARGS
			dockerArgsMutex.Lock()
			defer dockerArgsMutex.Unlock()
			if spec.IsFile() {
				dockerArgs = append(dockerArgs, "--volume", v+":"+v)
			}
			dockerArgs = append(dockerArgs, "--env", k)

			results <- Result{k, v, nil}
			wg.Done()
		}(key, spec)
	}
	wg.Wait()
	close(results)

EnvLoop:
	for envvar := range results {
		if envvar.error == nil {
			env[envvar.key] = envvar.value
		} else {
			if ac.IgnoreAll {
				continue EnvLoop
			}

			for i := range ac.Ignores {
				if ac.Ignores[i] == fmt.Sprintf("%s=%s", envvar.key, envvar.value) {
					continue EnvLoop
				}
			}
			return fmt.Errorf("Error fetching variable %v: %v", envvar.key, envvar.error.Error())
		}
	}

	// Append environment variable if one is specified
	if ac.Environment != "" {
		env[SummonEnvKeyName] = ac.Environment
	}

	setupEnvFile(ac.Args, env, &tempFactory)

	// Setup Docker args
	var argsWithDockerArgs []string
	for _, arg := range ac.Args {
		// Replace entire argument
		if arg == DockerArgsMagic {
			// Replace argument with slice of docker options
			argsWithDockerArgs = append(argsWithDockerArgs, dockerArgs...)
			continue
		}

		// Replace argument substring
		idx := strings.Index(arg, DockerArgsMagic)
		if idx >= 0 {
			// Replace substring in argument with slice of docker options
			argsWithDockerArgs = append(
				argsWithDockerArgs,
				strings.Replace(arg, DockerArgsMagic, strings.Join(dockerArgs, " "), -1),
			)
			continue
		}

		argsWithDockerArgs = append(argsWithDockerArgs, arg)
	}
	ac.Args = argsWithDockerArgs

	var e []string
	for k, v := range env {
		e = append(e, fmt.Sprintf("%s=%s", k, v))
	}

	return runSubcommand(
		ac.Args,
		append(os.Environ(), e...),
		ac.StdIn,
		ac.StdOut,
		ac.StdErr,
	)
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) (string, string) {
	if spec.IsFile() {
		fname := tempFactory.Push(value)
		value = fname
	}

	return key, value
}

func joinEnv(env map[string]string) string {
	var envs []string
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	// Sort to ensure predictable results
	sort.Strings(envs)

	return strings.Join(envs, "\n") + "\n"
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

// scans arguments for the envfile magic string; if found,
// creates a tempfile to which all the environment mappings are dumped
// and replaces the magic string with its path.
// Returns the path if so, returns an empty string otherwise.
func setupEnvFile(args []string, env map[string]string, tempFactory *TempFactory) string {
	var envFile = ""

	for i, arg := range args {
		idx := strings.Index(arg, EnvFileMagic)
		if idx >= 0 {
			if envFile == "" {
				envFile = tempFactory.Push(joinEnv(env))
			}
			args[i] = strings.Replace(arg, EnvFileMagic, envFile, -1)
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
		version, err := exec.Command(filepath.Join(providerPath, provider), "--version").Output()
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
