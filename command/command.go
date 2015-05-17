// Package cli defines the cauldron command line interface
package command

import (
	"github.com/codegangsta/cli"
	"os"
	"strings"
)

var filepath, yamlInline string
var subs map[string]string
var ignore []string

/*
Run parses user input and returns
* filepath: Path to secrets.yml
* yaml: string literal of secrets.yml
* subs: map of variables to be substituted in secrets.yml
* ignores: list of paths to ignore if fetch error occurs
*/
func Run(backend string) (string, string, map[string]string, []string) {
	app := cli.NewApp()
	app.Name = "cauldron-" + backend
	app.Usage = "Expose secrets as environment variables with " + backend + " backend"
	app.Version = "0.1.0"
	app.Flags = []cli.Flag{
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
		cli.StringFlag{
			Name:  "ignore, i",
			Usage: "Ignore the specified key if is isn't accessible or doesn’t exist",
		},
	}

	app.Action = func(c *cli.Context) {
		filepath = c.String("f")
		subs = convertSubsToMap(c.StringSlice("D"))
		yamlInline = c.String("yaml")
		ignore = strings.Split(c.String("ignore"), ",")
	}
	app.Run(os.Args)
	return filepath, yamlInline, subs, ignore
}

func convertSubsToMap(subs []string) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.Split(sub, "=")
		key, val := s[0], s[1]
		out[key] = val
	}
	return out
}
