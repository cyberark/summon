package command

import (
	"fmt"
	"github.com/codegangsta/cli"
	prov "github.com/conjurinc/summon/provider"
	"github.com/conjurinc/summon/secretsyml"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

const ENV_FILE_MAGIC = "@SUMMONENVFILE"

var Action = func(c *cli.Context) {
	if !c.Args().Present() {
		fmt.Println("Enter a subprocess to run!")
		os.Exit(127)
	}

	provider, err := prov.Resolve(c.String("provider"))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	out, err := runAction(
		c.Args(),
		provider,
		c.String("f"),
		c.String("yaml"),
		convertSubsToMap(c.StringSlice("D")),
		c.StringSlice("ignore"),
	)

	code, err := returnStatusOfError(err)

	if err != nil {
		fmt.Println(out + ": " + err.Error())
		os.Exit(127)
	}

	os.Exit(code)
}

// runAction encapsulates the logic of Action without cli Context for easier testing
func runAction(args []string, provider, filepath, yamlInline string, subs map[string]string, ignores []string) (string, error) {
	var (
		secrets secretsyml.SecretsMap
		err     error
	)

	switch yamlInline {
	case "":
		secrets, err = secretsyml.ParseFromFile(filepath, subs)
	default:
		secrets, err = secretsyml.ParseFromString(yamlInline, subs)
	}

	if err != nil {
		return "", err
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
				value, err = prov.Call(provider, spec.Path)
				if err != nil {
					results <- Result{key, err}
					wg.Done()
					return
				}
			} else {
				// If the spec isn't a variable, use its value as-is
				value = spec.Path
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
			for i := range ignores {
				if ignores[i] == envvar.string {
					continue EnvLoop
				}
			}
			return "Error fetching variable " + envvar.string, envvar.error
		}
	}

	setupEnvFile(args, env, &tempFactory)

	return runSubcommand(args, append(os.Environ(), env...))
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
