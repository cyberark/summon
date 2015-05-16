package main

import (
	"fmt"
	"github.com/conjurinc/cauldron/secretsyml"
)

func main() {
	secrets, err := secretsyml.ParseFile("./secrets.yml")
	if err != nil {
		panic(err)
	}

	fmt.Println(secrets)
}
