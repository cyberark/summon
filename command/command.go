// Package cli defines the cauldron command line interface
package command

import (
	"github.com/codegangsta/cli"
	"github.com/conjurinc/cauldron/backend"
	"os"
)

/*
Start defines and runs cauldron's command line interface
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
