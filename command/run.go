package command

import (
	"bytes"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/backend"
	"github.com/conjurinc/cauldron/secretsyml"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

var tempfiles []string

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
			envvar, err := fetchToEnviron(key, namespace, backend)
			if err != nil {
				fmt.Println(err.Error())
				break
			}
			env = append(env, envvar)
		}

		cmdOutput := &bytes.Buffer{}
		runner := exec.Command(c.Args()[0], c.Args()[1:]...)
		runner.Env = env
		runner.Stdout = cmdOutput
		err = runner.Start()
		if err != nil {
			panic(err)
		}
		runner.Wait()
		for _, path := range tempfiles {
			fmt.Println(path)
			os.Remove(path)
		}

		fmt.Print(string(cmdOutput.Bytes()))
	}

	return cmd
}

func fetchToEnviron(key string, namespace string, backend backend.Backend) (string, error) {
	namespaceNoPrefix := strings.Replace(namespace, "file ", "", -1)
	secretval, err := backend.Fetch(namespaceNoPrefix)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(namespace, "file ") {
		f, err := ioutil.TempFile("", "cauldron")
		f.Write([]byte(secretval))
		defer f.Close()

		if err != nil {
			return "", err
		}
		secretval = f.Name()
		tempfiles = append(tempfiles, secretval)
	}

	return fmt.Sprintf("%s=%s", strings.ToUpper(key), secretval), nil
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
