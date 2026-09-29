//go:build !windows && !solaris

package summon

import (
	"os"
	"os/exec"
	"testing"
)

// TestSetProcessGroupAlwaysSetpgid verifies that setProcessGroup always sets
// Setpgid: true regardless of whether stdin is a terminal. The terminal
// foreground grant is handled separately by grantTerminalForeground.
func TestSetProcessGroupAlwaysSetpgid(t *testing.T) {
	for _, name := range []string{"with pipe stdin", "with default stdin"} {
		t.Run(name, func(t *testing.T) {
			if name == "with pipe stdin" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				defer w.Close()
				origStdin := os.Stdin
				os.Stdin = r
				t.Cleanup(func() { os.Stdin = origStdin })
			}

			cmd := &exec.Cmd{}
			setProcessGroup(cmd)

			if cmd.SysProcAttr == nil {
				t.Fatal("SysProcAttr is nil")
			}
			if !cmd.SysProcAttr.Setpgid {
				t.Error("expected Setpgid=true")
			}
			if cmd.SysProcAttr.Foreground {
				t.Error("Foreground should not be set by setProcessGroup")
			}
		})
	}
}

// TestIsStdinTerminalWithPipe verifies isStdinTerminal returns false when stdin
// is a pipe (the common case in CI and scripted invocations).
func TestIsStdinTerminalWithPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	if isStdinTerminal() {
		t.Error("isStdinTerminal should return false for a pipe")
	}
}
