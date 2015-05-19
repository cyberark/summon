package main

import (
	"fmt"
	"github.com/conjurinc/cauldron"
)

func ExampleFetch(secret string) (string, error) {
	return "dummy", nil
}

func main() {
	c := cauldron.NewCauldron("example", ExampleFetch)
	err := c.Run()

	if err != nil {
		fmt.Errorf("%s", err)
	}
}
