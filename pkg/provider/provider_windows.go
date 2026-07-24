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

// setCallProcessGroup is a no-op on Windows. Process groups (Setpgid) and the
// associated syscall.Kill(-pgid, sig) are not available on Windows; cmd.Wait
// is not expected to block there in the same way.
func setCallProcessGroup(_ *exec.Cmd, _ time.Duration) {}
