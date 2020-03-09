package command

import (
	"github.com/codegangsta/cli"
)

var Flags = []cli.Flag{
	cli.StringFlag{
		Name:  "p, provider",
		Usage: "Path to provider for fetching secrets",
	},
	cli.StringFlag{
		Name:  "e, environment",
		Usage: "Specify section/environment to parse from secrets.yaml",
	},
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
		Usage: "Ignore the specified key if is isn't accessible or doesn't exist",
	},
	cli.BoolFlag{
		Name:  "ignore-all, I",
		Usage: "Ignore inaccessible or missing keys",
	},
	cli.BoolFlag{
		Name:  "all-provider-versions, V",
		Usage: "List of all of the providers in the default path and their versions(if they have the --version tag)",
	},
}
