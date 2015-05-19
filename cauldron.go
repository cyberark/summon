package cauldron

import (
	"fmt"
	"github.com/conjurinc/cauldron/backend"
	"github.com/conjurinc/cauldron/command"
	"os"
)

type Cauldron struct {
	Name    string
	Version string
	Fetcher backend.Fetch
}

func NewCauldron(name string, version string, fetcher backend.Fetch) Cauldron {
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
