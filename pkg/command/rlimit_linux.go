//go:build linux

// Package command implements the Summon CLI (flags, Action, and startup hooks).
package command

import "syscall"

// defaultApplyCoreDumpLimit sets RLIMIT_CORE soft and hard limits to zero on Linux.
func defaultApplyCoreDumpLimit() error {
	limit := &syscall.Rlimit{Cur: 0, Max: 0}
	return syscall.Setrlimit(syscall.RLIMIT_CORE, limit)
}
