package command

import (
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestHardenDaemon(t *testing.T) {
	origLimit, origDump := applyCoreDumpLimit, applyNotDumpable
	defer func() { applyCoreDumpLimit, applyNotDumpable = origLimit, origDump }()

	t.Run("ok", func(t *testing.T) {
		dumpable := false
		applyCoreDumpLimit = func() error { return nil }
		applyNotDumpable = func() error { dumpable = true; return nil }

		require.NoError(t, hardenDaemon())
		assert.True(t, dumpable)
	})

	t.Run("core limit failure is fatal", func(t *testing.T) {
		applyCoreDumpLimit = func() error { return errors.New("eperm") }
		applyNotDumpable = func() error { return nil }

		err := hardenDaemon()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "eperm")
	})

	t.Run("dumpable failure is fatal", func(t *testing.T) {
		applyCoreDumpLimit = func() error { return nil }
		applyNotDumpable = func() error { return errors.New("prctl failed") }

		err := hardenDaemon()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prctl failed")
	})
}

func TestAction_RefreshExitsWhenHardeningFails(t *testing.T) {
	origLimit, origDump, origExit := applyCoreDumpLimit, applyNotDumpable, osExit
	defer func() { applyCoreDumpLimit, applyNotDumpable, osExit = origLimit, origDump, origExit }()

	applyCoreDumpLimit = func() error { return errors.New("eperm") }
	applyNotDumpable = func() error { return nil }
	exitCode := -1
	osExit = func(code int) {
		exitCode = code
		panic("osExit")
	}

	app := cli.NewApp()
	app.Flags = Flags
	set := flag.NewFlagSet("test", 0)
	for _, f := range Flags {
		f.Apply(set)
	}
	require.NoError(t, set.Parse([]string{"--refresh", "cmd"}))
	ctx := cli.NewContext(app, set, nil)

	assert.PanicsWithValue(t, "osExit", func() { Action(ctx) })
	assert.Equal(t, 127, exitCode)
}
