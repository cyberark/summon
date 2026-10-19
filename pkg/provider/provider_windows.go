//go:build windows

// Package provider implements provider resolution and invocation for summon.
// This file contains Windows-specific stubs for process group management.
package provider

import (
	"os"
	"os/exec"
	"time"
)

// isProviderFile always returns true on Windows. Windows does not set execute
// bits in os.FileMode, so info.Mode()&0111 is always 0 for regular files —
// checking it would silently exclude every binary in the provider directory.
// Any non-directory file in a provider directory is assumed to be a provider.
func isProviderFile(_ os.FileInfo) bool { return true }

// setCallProcessGroup on Windows cannot kill a process group (Setpgid and
// syscall.Kill(-pgid, sig) do not exist): cancelling kills the provider
// process only, and WaitDelay makes cmd.Wait return even if a child it left
// behind keeps the output pipes open.
func setCallProcessGroup(cmd *exec.Cmd, timeout time.Duration) {
	if timeout > 0 {
		cmd.WaitDelay = min(timeout/2, maxWaitDelay)
	}
}
