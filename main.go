package main

import (
	"flag"
	"fmt"
	"github.com/conjurinc/cauldron/secretsyml"
)

func main() {
	var secretsFile string
	flag.StringVar(&secretsFile, "f", "secrets.yml", "Path to secrets.yml")
	flag.Parse()

	secrets, err := secretsyml.ParseFile(secretsFile)
	if err != nil {
		panic(err)
	}

	fmt.Println(secrets)
}
