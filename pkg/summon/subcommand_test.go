package summon

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSubcommand_SignalEscalation(t *testing.T) {
	// This test launches a child process that traps SIGTERM and ignores it,
	// then sends two SIGTERM signals to summon's signal channel. The first
	// SIGTERM is forwarded to the child (which ignores it). The second
	// SIGTERM triggers escalation to SIGKILL, which terminates the child
	// and allows runSubcommand to return.
	//
	// Without signal escalation, the child would run forever and
	// runSubcommand would never return — blocking deferred cleanup.

	// The child script:
	//   1. Traps SIGTERM (ignores it)
	//   2. Writes its PID to stdout so we know it started
	//   3. Sleeps for 60s (simulating a long-running process that won't die)
	script := `trap '' TERM; sleep 60`

	done := make(chan error, 1)
	go func() {
		done <- runSubcommand(
			[]string{"bash", "-c", script},
			os.Environ(),
		)
	}()

	// Give the child time to start and install its SIGTERM trap
	time.Sleep(200 * time.Millisecond)

	// First SIGTERM: forwarded to the child, which ignores it
	syscall.Kill(os.Getpid(), syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)

	// Child should still be running — runSubcommand should not have returned
	select {
	case <-done:
		t.Fatal("runSubcommand returned after the first SIGTERM; child should have ignored it")
	default:
		// expected
	}

	// Second SIGTERM: triggers escalation → SIGKILL on child
	syscall.Kill(os.Getpid(), syscall.SIGTERM)

	// runSubcommand should now return promptly
	select {
	case err := <-done:
		// The child was killed, so we expect a non-nil error (signal: killed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kill", "child should have been killed")
	case <-time.After(5 * time.Second):
		t.Fatal("runSubcommand did not return after second SIGTERM; signal escalation is broken")
	}
}
