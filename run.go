package main

import (
	"bytes"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/secretsyml"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

var tempfiles []string

func CreateRunCommand() cli.Command {
	cmd := cli.Command{
		Name:  "run",
		Usage: "Run cauldron",
	}
	cmd.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "p, provider",
			Usage: "Path to provider for fetching secrets",
		},
		cli.StringFlag{
			Name:  "f",
			Value: "secrets.yml",
			Usage: "Path to secrets.yml",
		},
		cli.StringSliceFlag{
			Name:  "D",
			Value: &cli.StringSlice{},
			Usage: "var=value causes substitution of value to $var",
		},
		cli.StringFlag{
			Name:  "yaml",
			Usage: "secrets.yml as a literal string",
		},
		cli.StringSliceFlag{
			Name:  "ignore, i",
			Value: &cli.StringSlice{},
			Usage: "Ignore the specified key if is isn't accessible or doesn’t exist",
		},
	}
	cmd.Action = func(c *cli.Context) {
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

		provider, err := resolveProvider(c.String("provider"))
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		erred := false
		env := os.Environ()
		for key, spec := range secrets {
			value, err := callProvider(provider, spec.Path)
			if err != nil {
				fmt.Println(value)
				os.Exit(1)
			}
			envvar, err := formatForEnv(key, value, spec)
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

	return cmd
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
func formatForEnv(key string, value string, spec secretsyml.SecretSpec) (string, error) {
	if spec.IsFile {
		f, err := ioutil.TempFile("", "cauldron")
		f.Write([]byte(value))
		defer f.Close()

		if err != nil {
			return "", err
		}
		value = f.Name()
		tempfiles = append(tempfiles, value)
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
