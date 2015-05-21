package main

import (
	"github.com/codegangsta/cli"
	"io"
	"os"
)

var (
	CLIArgs   []string  = os.Args
	CLIWriter io.Writer = os.Stdout
)

/*
Start defines and runs cauldron's command line interface
*/
func RunCLI() error {
	app := cli.NewApp()
	app.Name = "cauldron"
	app.Usage = "Parse secrets.yml and export environment variables"
	app.Version = "0.1.0"
	app.Writer = CLIWriter

	app.Commands = []cli.Command{
		CreateRunCommand(),
	}
	return app.Run(CLIArgs)
}
