package command

import (
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func TestPrintProviderVersions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test.")
	}

	t.Run("printProviderVersions should return a string of all of the providers in the defaultPath", func(t *testing.T) {
		pathTo, err := os.Getwd()
		assert.NoError(t, err)
		pathToTest := filepath.Join(pathTo, "testversions")

		//test1 - regular formating and appending of version # to string
		//test2 - chopping off of trailing newline
		//test3 - failed `--version` call
		output, err := printProviderVersions(pathToTest)
		assert.NoError(t, err)

		expected := fmt.Sprintf(`Provider versions in %s:
testprovider version 1.2.3
testprovider-noversionsupport: unknown version
testprovider-trailingnewline version 3.2.1
`, pathToTest)

		assert.Equal(t, expected, output)
	})
}

func TestConfigureDebugLogging(t *testing.T) {
	// Save the original default logger and restore it after the test
	originalHandler := slog.Default().Handler()
	defer slog.SetDefault(slog.New(originalHandler))

	t.Run("debug messages are written when debug logging is enabled", func(t *testing.T) {
		var buf bytes.Buffer
		err := configureDebugLogging(&buf)

		assert.NoError(t, err)

		slog.Debug("test debug message")

		output := buf.String()
		assert.Contains(t, output, "test debug message")
		assert.Contains(t, output, "level=DEBUG")
	})

	t.Run("debug messages are suppressed with default logger", func(t *testing.T) {
		var buf bytes.Buffer
		// Reset to default (Info level) logger writing to our buffer
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

		slog.Debug("this should not appear")

		assert.Empty(t, buf.String())
	})

	t.Run("returns error when writer fails", func(t *testing.T) {
		err := configureDebugLogging(&failWriter{})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize debug logging")
	})
}

// failWriter is an io.Writer that always returns an error.
type failWriter struct{}

func (fw *failWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func TestAction_CallsSuppressCoreDumpsBeforeArgCheck(t *testing.T) {
	originalApply := applyCoreDumpLimit
	originalExit := osExit
	defer func() {
		applyCoreDumpLimit = originalApply
		osExit = originalExit
	}()

	called := false
	applyCoreDumpLimit = func() error {
		called = true
		return nil
	}

	exitCode := -1
	// Production osExit is os.Exit (does not return). Panic here so Action can
	// return control to the test after the missing-args path.
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
	ctx := cli.NewContext(app, set, nil)

	assert.PanicsWithValue(t, "osExit", func() {
		Action(ctx)
	})
	assert.True(t, called, "suppressCoreDumps should run before the missing-args exit")
	assert.Equal(t, 127, exitCode)
}
