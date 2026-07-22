// Package command implements the Summon CLI (flags, Action, and startup hooks).
package command

import "log/slog"

// applyCoreDumpLimit is the platform-specific core-dump suppression hook.
// Tests may replace it to exercise the failure path on any GOOS.
var applyCoreDumpLimit = defaultApplyCoreDumpLimit

// suppressCoreDumps attempts to disable core dumps for this process.
// On failure it logs at Info level and continues; startup must not abort.
// EPERM is normal in containers that restrict Setrlimit at the host level.
func suppressCoreDumps() {
	if err := applyCoreDumpLimit(); err != nil {
		slog.Info("Failed to suppress core dumps via Setrlimit; continuing", "error", err)
	}
}
