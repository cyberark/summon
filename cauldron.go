package cauldron

import (
	"fmt"
	"github.com/conjurinc/cauldron/command"
	"os"
)

type Backend interface {
	Fetch(string) (string, error)
}

type Cauldron struct {
	Name    string
	Backend Backend
}

func NewCauldron(name string, backend Backend) Cauldron {
	return Cauldron{
		Name:    name,
		Backend: backend,
	}
}

func (c Cauldron) Run() error {
	if c.Backend == nil {
		fmt.Println("You must specify a backend")
		os.Exit(1)
	}
	return command.Start("dummy", c.Backend)
}
