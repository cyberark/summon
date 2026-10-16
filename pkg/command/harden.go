package command

import (
	"fmt"
	"io"
	"os"
)

// stderr is where the hardening warning goes; tests replace it.
var stderr io.Writer = os.Stderr

// hardeningGap describes what --refresh hardening cannot do on this platform
// (empty when nothing is missing). Tests may replace it.
var hardeningGap = platformHardeningGap

// applyNotDumpable is the platform-specific hook that marks the process
// non-dumpable. Tests may replace it to exercise the failure path on any GOOS.
var applyNotDumpable = defaultApplyNotDumpable

// hardenDaemon applies the process hardening required for a long-lived
// --refresh run. Unlike one-shot mode, failure is fatal: a daemon holds
// credentials for its whole lifetime, so it must not run with core dumps
// enabled.
//
// When the platform cannot do everything, it then prints one warning line
// straight to stderr (not via slog, so SUMMON_LOG_LEVEL=ERROR cannot hide it).
//
// On Linux, PR_SET_DUMPABLE=0 also blocks same-uid ptrace and makes
// /proc/<pid>/mem and /proc/<pid>/environ root-only. That matters because
// os.Unsetenv only updates Go's copy of the environment; the kernel keeps
// exposing the original environ block (including any CONJUR_* values the
// process was started with) through /proc/<pid>/environ. Operators who cannot
// accept that should inject credentials by file (CONJUR_AUTHN_JWT_TOKEN_PATH
// style) rather than environment variables.
func hardenDaemon() error {
	if err := applyCoreDumpLimit(); err != nil {
		return fmt.Errorf("daemon mode requires core dump suppression: %w", err)
	}
	if err := applyNotDumpable(); err != nil {
		return fmt.Errorf("daemon mode requires a non-dumpable process: %w", err)
	}
	if gap := hardeningGap(); gap != "" {
		fmt.Fprintf(stderr, "summon: warning: %s\n", gap)
	}
	return nil
}
