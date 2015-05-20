// Package cli defines the cauldron command line interface
package cauldron

import (
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/secretsyml"
	"io"
	"os"
)

var args []string = os.Args
var writer io.Writer = os.Stdout

type CLI struct {
	BackendName string
	Version     string
	FetchFn     secretsyml.Fetch
}

func NewCLI(backendName string, version string, fetchFn secretsyml.Fetch) *CLI {
	return &CLI{
		BackendName: backendName,
		Version:     version,
		FetchFn:     fetchFn,
	}
}

/*
Start defines and runs cauldron's command line interface
*/
func (c *CLI) Start() error {
	app := cli.NewApp()
	app.Name = "cauldron-" + c.BackendName
	app.Usage = "Expose secrets as environment variables with " + c.BackendName + " backend"
	app.Version = c.Version
	app.Writer = writer

	app.Commands = []cli.Command{
		CreateRunCommand(c.FetchFn),
	}
	return app.Run(args)
}
