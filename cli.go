package main

import (
	"github.com/codegangsta/cli"
	"github.com/conjurinc/summon/command"
	"io"
	"os"
)

var (
	CLIArgs   []string  = os.Args
	CLIWriter io.Writer = os.Stdout
)

/*
Start defines and runs summon's command line interface
*/
func RunCLI() error {
	app := cli.NewApp()
	app.Name = "summon"
	app.Usage = "Parse secrets.yml and export environment variables"
	app.Version = VERSION
	app.Writer = CLIWriter
	app.Flags = command.Flags
	app.Action = command.Action

	return app.Run(CLIArgs)
}
