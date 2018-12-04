package command

import (
	"os/exec"
	"syscall"
)

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
func runSubcommand(command []string, env []string) error {
	binary, lookupErr := exec.LookPath(command[0])
	if lookupErr != nil {
		return lookupErr
	}

	return syscall.Exec(binary, command, env)
}
