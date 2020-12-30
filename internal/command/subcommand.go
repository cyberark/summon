package command

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values. Since we have to
// clean up our temp directories, we remain resident and shuffle
// signals around to the chld and back
func runSubcommand(
	command []string,
	env []string,
	Stdin io.Reader,
	Stdout io.Writer,
	Stderr io.Writer,
) error {
	binary, lookupErr := exec.LookPath(command[0])
	if lookupErr != nil {
		return lookupErr
	}

	runner := exec.Command(binary, command[1:]...)

	if Stdin == nil {
		Stdin = os.Stdin
	}
	if Stdout == nil {
		Stdout = os.Stdout
	}
	if Stderr == nil {
		Stderr = os.Stderr
	}

	runner.Stdin = Stdin
	runner.Stdout = Stdout
	runner.Stderr = Stderr
	runner.Env = env

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel)

	if startErr := runner.Start(); startErr != nil {
		return startErr
	}

	// Forward all signals to the child process
	go func() {
		for {
			receivedSignal := <-signalChannel
			runner.Process.Signal(receivedSignal)
		}
	}()

	if waitErr := runner.Wait(); waitErr != nil {
		runner.Process.Signal(syscall.SIGKILL)
		return waitErr
	}

	return nil
}
