package main

import (
	"fmt"
	"github.com/conjurinc/cauldron"
)

type ExampleBackend struct{}

func (e ExampleBackend) Fetch(secret string) (string, error) {
	return "dummy", nil
}

func main() {
	c := cauldron.NewCauldron("dummy", ExampleBackend{})
	err := c.Run()

	if err != nil {
		fmt.Errorf("%s", err)
	}
}
