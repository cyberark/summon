package cauldron

import (
	"github.com/codegangsta/cli"
	"io"
	"os"
)

var args []string = os.Args
var writer io.Writer = os.Stdout

type CLI struct {
	Provider string
}

func NewCLI(provider string) *CLI {
	return &CLI{
		Provider: provider,
	}
}

/*
Start defines and runs cauldron's command line interface
*/
func (c *CLI) Start() error {
	app := cli.NewApp()
	app.Name = "cauldron"
	app.Usage = "Expose secrets as environment variables with provider: " + c.Provider
	app.Version = "0.1.0"
	app.Writer = writer

	app.Commands = []cli.Command{
		CreateRunCommand(c.Provider),
	}
	return app.Run(args)
}
