//go:build !linux && !windows

// Package command implements the Summon CLI (flags, Action, and startup hooks).
package command

import "syscall"

// defaultApplyCoreDumpLimit sets RLIMIT_CORE soft and hard limits to zero on
// non-Linux Unix systems (darwin, BSD, Solaris).
func defaultApplyCoreDumpLimit() error {
	return syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{Cur: 0, Max: 0})
}
