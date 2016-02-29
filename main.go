package main

import (
	"fmt"
	"os"
)

func main() {
	if err := RunCLI(); err != nil {
		fmt.Println(err.Error())
		os.Exit(-1)
	}
}
