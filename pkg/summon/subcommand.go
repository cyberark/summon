package summon

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values. Since we have to
// clean up our temp directories, we remain resident and shuffle
// signals around to the child and back.
//
// Signal escalation: the first termination signal (SIGINT, SIGTERM,
// SIGHUP) is forwarded to the child as-is. A second termination
// signal force-kills the child with SIGKILL, ensuring that
// runSubcommand always returns and deferred cleanup can run.
func runSubcommand(command []string, env []string) error {
	binary, lookupErr := exec.LookPath(command[0])
	if lookupErr != nil {
		return lookupErr
	}

	runner := exec.Command(binary, command[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = os.Stdout
	runner.Stderr = os.Stderr
	runner.Env = env

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel)
	defer signal.Stop(signalChannel)

	if startErr := runner.Start(); startErr != nil {
		return startErr
	}

	done := make(chan struct{})

	// Forward all signals to the child process, with escalation for
	// termination signals: a second SIGINT/SIGTERM/SIGHUP sends SIGKILL.
	go func() {
		receivedTermination := false
		for {
			select {
			case sig := <-signalChannel:
				if isTermSignal(sig) {
					if receivedTermination {
						// Second termination signal: force-kill the child
						runner.Process.Signal(syscall.SIGKILL)
						return
					}
					receivedTermination = true
				}
				runner.Process.Signal(sig)
			case <-done:
				return
			}
		}
	}()

	waitErr := runner.Wait()
	close(done)

	return waitErr
}

// isTermSignal returns true for signals that request process termination.
func isTermSignal(sig os.Signal) bool {
	switch sig {
	case syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP:
		return true
	}
	return false
}
