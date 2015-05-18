package command

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/backend"
	"github.com/conjurinc/cauldron/secretsyml"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func CreateRunCommand(backend backend.Backend) cli.Command {
	cmd := cli.Command{
		Name:  "run",
		Usage: "Run cauldron",
	}
	cmd.Flags = []cli.Flag{
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
			secrets map[string]string
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
		case "\n":
			secrets, err = secretsyml.ParseFromString(yamlInline, subs)
		default:
			secrets, err = secretsyml.ParseFromFile(filepath, subs)
		}

		if err != nil {
			fmt.Errorf("error! %s", err)
		}

		env := os.Environ()
		for key, namespace := range secrets {
			namespaceNoPrefix := strings.Replace(namespace, "file ", "", 1)
			secretval, _ := backend.Fetch(namespaceNoPrefix)
			env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(key), secretval))
		}

		binary, lookErr := exec.LookPath(c.Args().First())
		if lookErr != nil {
			panic(lookErr)
		}

		execErr := syscall.Exec(binary, c.Args(), env)
		if execErr != nil {
			panic(execErr)
		}
	}

	return cmd
}

func convertSubsToMap(subs []string) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.Split(sub, "=")
		key, val := s[0], s[1]
		out[key] = val
	}
	return out
}
