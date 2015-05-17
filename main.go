package main

import (
	"fmt"
	"github.com/conjurinc/cauldron/command"
	"github.com/conjurinc/cauldron/secretsyml"
)

func main() {
	filepath, yamlInline, subs, _ := command.Run("dummy")
	if yamlInline != "" {
		secrets, err := secretsyml.ParseFromString(yamlInline, subs)
		if err != nil {
			panic(err)
		}
		fmt.Println(secrets)
		return
	}

	secrets, err := secretsyml.ParseFromFile(filepath, subs)
	if err != nil {
		panic(err)
	}
	fmt.Println(secrets)
}
