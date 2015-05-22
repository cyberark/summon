package command

import (
	"bytes"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/provider"
	"github.com/conjurinc/cauldron/secretsyml"
	"os"
	"os/exec"
	"strings"
)

var tempfiles []string

var Action = func(c *cli.Context) {
	var (
		secrets secretsyml.SecretsMap
		err     error
	)

	if !c.Args().Present() {
		println("Enter a subprocess to run!")
		os.Exit(1)
	}

	filepath := c.String("f")
	yamlInline := c.String("yaml")
	subs := convertSubsToMap(c.StringSlice("D"))
	// ignore := strings.Split(c.String("ignore"), ",")

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

	prov, err := provider.ResolveProvider(c.String("provider"))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	erred := false
	env := os.Environ()
	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	for key, spec := range secrets {
		value, err := provider.CallProvider(prov, spec.Path)
		if err != nil {
			fmt.Println(value)
			os.Exit(1)
		}
		envvar, err := formatForEnv(key, value, spec, &tempFactory)
		if err != nil {
			erred = true
			fmt.Printf("%s: %s\n", key, err.Error())
		}
		env = append(env, envvar)
	}

	// Only print output of the command if no errors have occurred
	output := runSubcommand(c.Args(), env)
	if !erred {
		fmt.Print(output)
	} else {
		os.Exit(1)
	}
}

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
// On command exit, any tempfiles containing secrets are removed.
func runSubcommand(args []string, env []string) string {
	cmdOutput := &bytes.Buffer{}
	runner := exec.Command(args[0], args[1:]...)
	runner.Env = env
	runner.Stdout = cmdOutput
	err := runner.Start()
	if err != nil {
		panic(err)
	}
	runner.Wait()
	for _, path := range tempfiles {
		fmt.Println(path)
		os.Remove(path)
	}

	return string(cmdOutput.Bytes())
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) (string, error) {
	if spec.IsFile() {
		fname, err := tempFactory.Push(value)
		if err != nil {
			return "", err
		}
		value = fname
	}

	return fmt.Sprintf("%s=%s", strings.ToUpper(key), value), nil
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
