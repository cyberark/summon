// Package cli defines the cauldron command line interface
package command

import (
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/backend"
	"os"
)

/*
Start parses user input and returns
* filepath: Path to secrets.yml
* yaml: string literal of secrets.yml
* subs: map of variables to be substituted in secrets.yml
* ignores: list of paths to ignore if fetch error occurs
*/
func Start(backendName string, backend backend.Backend) error {
	app := cli.NewApp()
	app.Name = "cauldron-" + backendName
	app.Usage = "Expose secrets as environment variables with " + backendName + " backend"
	app.Version = "0.1.0"

	app.Commands = []cli.Command{
		CreateRunCommand(backend),
	}
	return app.Run(os.Args)
}
