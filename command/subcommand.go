package command

import (
	"bytes"
	"io"
	"os"
	"os/exec"
)

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
func runSubcommand(command []string, env []string) (string, error) {
	var stdOut bytes.Buffer

	runner := exec.Command(command[0], command[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = io.MultiWriter(os.Stdout, &stdOut)
	runner.Stderr = os.Stderr
	runner.Env = env

	err := runner.Run()
	return stdOut.String(), err
}
