//go:build windows

// Package command implements the Summon CLI (flags, Action, and startup hooks).
package command

// defaultApplyCoreDumpLimit is a silent no-op on Windows (no RLIMIT_CORE);
// --refresh reports the gap once via platformHardeningGap.
func defaultApplyCoreDumpLimit() error { return nil }
