package main

import (
	"testing"
)

// E2E test for the command line interface
func TestStart(t *testing.T) {
	yamlContent := "'AWS_PEM: !file $policy/aws/iam/user/robot/access_key_id'"
	CLIArgs = []string{"cauldron-testing", "run", "-p", "dummyProvider", "--yaml", yamlContent, "printenv", "AWS_PEM"}

	err := RunCLI()
	if err != nil {
		t.Error(err.Error())
	}

	//TODO: buf doesn't have content, find out why
}
