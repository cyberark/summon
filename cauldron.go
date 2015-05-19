package cauldron

import (
	"fmt"
	"github.com/conjurinc/cauldron/command"
	"github.com/conjurinc/cauldron/secretsyml"
	"os"
)

type Cauldron struct {
	Name    string
	Version string
	Fetcher secretsyml.Fetch
}

func NewCauldron(name string, version string, fetcher secretsyml.Fetch) Cauldron {
	return Cauldron{
		Name:    name,
		Version: version,
		Fetcher: fetcher,
	}
}

func (c Cauldron) Run() error {
	if c.Fetcher == nil {
		fmt.Println("You must specify a backend")
		os.Exit(1)
	}
	return command.Start(c.Name, c.Version, c.Fetcher)
}
