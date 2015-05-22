package command

import (
	"fmt"
	"github.com/codegangsta/cli"
	prov "github.com/conjurinc/cauldron/provider"
	"github.com/conjurinc/cauldron/secretsyml"
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

	provider, err := prov.ResolveProvider(c.String("provider"))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	out := runAction(
		c.Args(),
		provider,
		c.String("f"),
		c.String("yaml"),
		convertSubsToMap(c.StringSlice("D")),
		strings.Split(c.String("ignore"), ","),
	)

	fmt.Print(out)
}

// runAction encapsulates the logic of Action without cli Context for easier testing
func runAction(args []string, provider, filepath, yamlInline string, subs map[string]string, ignores []string) string {
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
		fmt.Println(err.Error())
		os.Exit(1)
	}

	env := os.Environ()
	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	// Run provider calls concurrently
	results := make(chan string, len(secrets))
	var wg sync.WaitGroup

	for key, spec := range secrets {
		wg.Add(1)
		go func(key string, spec secretsyml.SecretSpec) {
			var value string
			if spec.IsLiteral() {
				value = spec.Path
			} else {
				value, err = prov.CallProvider(provider, spec.Path)
				if err != nil {
					fmt.Println(err.Error())
					os.Exit(1)
				}
			}
			envvar := formatForEnv(key, value, spec, &tempFactory)
			results <- envvar
			wg.Done()
		}(key, spec)
	}
	wg.Wait()
	close(results)

	for envvar := range results {
		env = append(env, envvar)
	}

	return runSubcommand(args, env)
}

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
// On command exit, any tempfiles containing secrets are removed.
func runSubcommand(args []string, env []string) string {
	runner := exec.Command(args[0], args[1:]...)
	runner.Env = env
	out, err := runner.CombinedOutput()
	if err != nil {
		panic(err)
	}
	return string(out)
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) string {
	if spec.IsFile() {
		fname := tempFactory.Push(value)
		value = fname
	}

	return fmt.Sprintf("%s=%s", strings.ToUpper(key), value)
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
