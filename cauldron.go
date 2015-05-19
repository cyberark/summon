package cauldron

import (
	"fmt"
	"github.com/conjurinc/cauldron/backend"
	"github.com/conjurinc/cauldron/command"
	"os"
)

type Cauldron struct {
	Name    string
	Fetcher backend.Fetch
}

func NewCauldron(name string, fetcher backend.Fetch) Cauldron {
	return Cauldron{
		Name:    name,
		Fetcher: fetcher,
	}
}

func (c Cauldron) Run() error {
	if c.Fetcher == nil {
		fmt.Println("You must specify a backend")
		os.Exit(1)
	}
	return command.Start("dummy", c.Fetcher)
}
