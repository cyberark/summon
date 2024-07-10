package main

import (
	"fmt"
	"io"
	"os"

	"github.com/cyberark/summon/pkg/command"
	"github.com/cyberark/summon/pkg/summon"
	"github.com/urfave/cli"
)

var (
	// CLIArgs holds the arguments passed on from the CLI flags
	CLIArgs = os.Args
	// CLIWriter is the pointer to the output sink of Summon
	CLIWriter io.Writer = os.Stdout
)

// RunCLI starts defines and runs summon's command line interface
func RunCLI() error {
	app := cli.NewApp()
	app.Name = "summon"
	app.Usage = "Parse secrets.yml and export environment variables"
	app.Version = summon.FullVersionName
	app.Writer = CLIWriter
	app.Flags = command.Flags
	app.Action = command.Action

	return app.Run(CLIArgs)
}

func main() {
	if err := RunCLI(); err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
}
