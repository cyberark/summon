package command

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureWarnings redirects the warning writer and forces slog to ERROR so a
// passing assertion proves the warning bypasses SUMMON_LOG_LEVEL.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origErr, origLog, origGap := stderr, slog.Default(), hardeningGap
	origLimit, origDump := applyCoreDumpLimit, applyNotDumpable
	t.Cleanup(func() {
		stderr, hardeningGap = origErr, origGap
		applyCoreDumpLimit, applyNotDumpable = origLimit, origDump
		slog.SetDefault(origLog)
	})
	stderr = &buf
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})))
	return &buf
}

func TestSuppressCoreDumps_PrintsNothing(t *testing.T) {
	buf := captureWarnings(t)
	hardeningGap = func() string { return "should never appear" }
	applyCoreDumpLimit = func() error { return nil }
	suppressCoreDumps()
	applyCoreDumpLimit = func() error { return errors.New("eperm") }
	suppressCoreDumps()
	assert.Empty(t, buf.String())
}

func TestHardenDaemon_WarnsOnceWithPlatformText(t *testing.T) {
	buf := captureWarnings(t)
	hardeningGap = func() string { return "gap text" }
	applyCoreDumpLimit = func() error { return nil }
	applyNotDumpable = func() error { return nil }

	require.NoError(t, hardenDaemon())
	assert.Equal(t, "summon: warning: gap text\n", buf.String())
}

func TestHardenDaemon_NoWarningWhenNoGap(t *testing.T) {
	buf := captureWarnings(t)
	hardeningGap = func() string { return "" }
	applyCoreDumpLimit = func() error { return nil }
	applyNotDumpable = func() error { return nil }

	require.NoError(t, hardenDaemon())
	assert.Empty(t, buf.String())
}

func TestHardenDaemon_FailureIsFatalAndSilent(t *testing.T) {
	buf := captureWarnings(t)
	hardeningGap = func() string { return "gap text" }
	applyNotDumpable = func() error { return nil }
	applyCoreDumpLimit = func() error { return errors.New("hook failed") }

	err := hardenDaemon()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")
	assert.Empty(t, buf.String())
}
