package main

import (
	"fmt"
	"github.com/conjurinc/cauldron"
)

func ExampleFetch(secret string) (string, error) {
	return "dummy", nil
}

func main() {
	c := cauldron.NewCLI("example", "0.1.0", ExampleFetch)
	err := c.Start()

	if err != nil {
		fmt.Errorf("%s", err)
	}
}
