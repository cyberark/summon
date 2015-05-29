package command

import (
	"bytes"
	"fmt"
	"github.com/codegangsta/cli"
	prov "github.com/conjurinc/cauldron/provider"
	"github.com/conjurinc/cauldron/secretsyml"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var Action = func(c *cli.Context) {
	if !c.Args().Present() {
		fmt.Println("Enter a subprocess to run!")
		os.Exit(1)
	}

	provider, err := prov.Resolve(c.String("provider"))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	out, err := runAction(
		c.Args(),
		provider,
		c.String("f"),
		c.String("yaml"),
		convertSubsToMap(c.StringSlice("D")),
		c.StringSlice("ignore"),
	)

	if err != nil {
		fmt.Println(out + ": " + err.Error())
		os.Exit(1)
	}
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

	env := os.Environ()
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
			return envvar.string, envvar.error
		}
	}

	return runSubcommand(args, env)
}

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
// On command exit, any tempfiles containing secrets are removed.
func runSubcommand(args []string, env []string) (string, error) {
	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)
	runner := exec.Command(args[0], args[1:]...)
	runner.Stderr = io.MultiWriter(os.Stderr, &stdErr)
	runner.Stdout = io.MultiWriter(os.Stdout, &stdOut)
	runner.Env = env

	err := runner.Run()
	return stdOut.String(), err
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

// convertSubsToMap converts the list of substitutions passed in via
// command line to a map
func convertSubsToMap(subs []string) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.Split(sub, "=")
		key, val := s[0], s[1]
		out[key] = val
	}
	return out
}
