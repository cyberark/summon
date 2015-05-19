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
func Start(backendName string, version string, fetcher backend.Fetch) error {
	app := cli.NewApp()
	app.Name = "cauldron-" + backendName
	app.Usage = "Expose secrets as environment variables with " + backendName + " backend"
	app.Version = version

	app.Commands = []cli.Command{
		CreateRunCommand(fetcher),
	}
	return app.Run(os.Args)
}
