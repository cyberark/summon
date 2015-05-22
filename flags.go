package main

import (
	"github.com/codegangsta/cli"
)

var CLIFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "p, provider",
		Usage: "Path to provider for fetching secrets",
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
		Usage: "Ignore the specified key if is isn't accessible or doesn’t exist",
	},
}
