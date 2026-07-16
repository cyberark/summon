//go:build !linux

// Package command implements the Summon CLI (flags, Action, and startup hooks).
package command

// defaultApplyCoreDumpLimit is a no-op outside Linux; core dump suppression is Linux-only.
func defaultApplyCoreDumpLimit() error {
	return nil
}
