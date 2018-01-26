package command

import (
	"os"
	"os/exec"
)

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
func runSubcommand(command []string, env []string) error {
	runner := exec.Command(command[0], command[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = os.Stdout
	runner.Stderr = os.Stderr
	runner.Env = env

	return runner.Run()
}
