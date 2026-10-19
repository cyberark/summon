package command

import (
	"bytes"
	"flag"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestRefreshFlagAliases(t *testing.T) {
	for _, alias := range []string{"--refresh", "--daemon", "--sidecar"} {
		t.Run(alias, func(t *testing.T) {
			var got bool
			app := cli.NewApp()
			app.Flags = Flags
			app.Action = func(c *cli.Context) { got = c.Bool("refresh") }

			require.NoError(t, app.Run([]string{"summon", alias, "cmd"}))

			assert.True(t, got)
		})
	}

	t.Run("off by default", func(t *testing.T) {
		got := true
		app := cli.NewApp()
		app.Flags = Flags
		app.Action = func(c *cli.Context) { got = c.Bool("refresh") }

		require.NoError(t, app.Run([]string{"summon", "cmd"}))

		assert.False(t, got)
	})
}

func TestConfigureLogging(t *testing.T) {
	original := slog.Default().Handler()
	defer slog.SetDefault(slog.New(original))

	t.Run("default level is INFO", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, configureLogging(&buf, ""))

		slog.Debug("debug-line")
		slog.Info("info-line")

		assert.NotContains(t, buf.String(), "debug-line")
		assert.Contains(t, buf.String(), "info-line")
	})

	t.Run("WARN suppresses INFO", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, configureLogging(&buf, "warn"))

		slog.Info("info-line")
		slog.Warn("warn-line")

		assert.NotContains(t, buf.String(), "info-line")
		assert.Contains(t, buf.String(), "warn-line")
	})

	t.Run("DEBUG shows everything", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, configureLogging(&buf, "DEBUG"))

		slog.Debug("debug-line")

		assert.Contains(t, buf.String(), "debug-line")
	})

	t.Run("unknown level is rejected", func(t *testing.T) {
		err := configureLogging(&bytes.Buffer{}, "LOUD")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "SUMMON_LOG_LEVEL")
	})
}

func TestAction_RefreshRejectsInvalidSettings(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"interval below minimum": {"SUMMON_REFRESH_INTERVAL": "1s"},
		"bad restart flag":       {"SUMMON_RESTART_ON_ROTATION": "sometimes"},
		"bad log level":          {"SUMMON_LOG_LEVEL": "LOUD"},
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			originalExit := osExit
			defer func() { osExit = originalExit }()
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
		})
	}
}

func TestAction_RefreshUnsupportedPlatform(t *testing.T) {
	originalSupported, originalExit, originalStderr := refreshSupported, osExit, os.Stderr
	defer func() { refreshSupported, osExit, os.Stderr = originalSupported, originalExit, originalStderr }()
	refreshSupported = false
	exitCode := -1
	osExit = func(code int) {
		exitCode = code
		panic("osExit")
	}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	app := cli.NewApp()
	app.Flags = Flags
	set := flag.NewFlagSet("test", 0)
	for _, f := range Flags {
		f.Apply(set)
	}
	require.NoError(t, set.Parse([]string{"--refresh", "cmd"}))
	ctx := cli.NewContext(app, set, nil)

	assert.PanicsWithValue(t, "osExit", func() { Action(ctx) })
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, 127, exitCode)
	assert.Contains(t, string(out), "refresh mode is not supported on solaris")
}
