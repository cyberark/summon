package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/summon"
	"github.com/urfave/cli"
)

// osExit is replaced in tests so Action can be exercised without ending the process.
var osExit = os.Exit

// refreshSupported reports whether --refresh can run on this OS. Solaris cannot
// track a child's process tree (no syscall.Getpgid), so a restart could leave
// orphans. Replaced in tests.
var refreshSupported = runtime.GOOS != "solaris"

// configureDebugLogging sets up slog to output debug-level messages to the given writer.
// It returns an error if the initial log write fails.
func configureDebugLogging(w io.Writer) error {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Verify the logger works by writing a startup message.
	// If this fails, the caller should abort rather than run silently without logging.
	if err := handler.Handle(
		context.Background(),
		slog.NewRecord(time.Now(), slog.LevelDebug, "Debug logging enabled", 0),
	); err != nil {
		return fmt.Errorf("failed to initialize debug logging: %w", err)
	}
	return nil
}

// configureLogging sets up slog to write to w at the level named by
// SUMMON_LOG_LEVEL (DEBUG, INFO, WARN or ERROR; empty means INFO).
func configureLogging(w io.Writer, levelName string) error {
	level := slog.LevelInfo
	switch strings.ToUpper(levelName) {
	case "":
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		return fmt.Errorf("SUMMON_LOG_LEVEL must be one of DEBUG, INFO, WARN, ERROR: %q", levelName)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	return nil
}

// Action is the runner for the main program logic
var Action = func(c *cli.Context) {
	if c.Bool("debug") {
		if err := configureDebugLogging(os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			osExit(127)
		}
	}

	// Suppress core dumps before secrets enter process memory.
	suppressCoreDumps()

	if !c.Args().Present() && !c.Bool("all-provider-versions") {
		fmt.Fprintln(os.Stderr, "Enter a subprocess to run!")
		osExit(127)
	}

	if c.Bool("all-provider-versions") {
		if err := runPrintProviderVersions(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			osExit(127)
		}
		return
	}

	var refresh *summon.RefreshConfig
	if c.Bool("refresh") {
		if !refreshSupported {
			fmt.Fprintln(os.Stderr, "refresh mode is not supported on solaris")
			osExit(127)
		}
		var err error
		if refresh, err = summon.RefreshConfigFromEnv(os.Getenv); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			osExit(127)
		}
		if err := hardenDaemon(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			osExit(127)
		}
		if !c.Bool("debug") {
			if err := configureLogging(os.Stderr, os.Getenv("SUMMON_LOG_LEVEL")); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				osExit(127)
			}
		}
	}

	providerArg := c.String("provider")

	resolve := prov.Resolve
	if refresh != nil {
		resolve = func(arg string) (prov.Provider, error) {
			return prov.ResolveDaemon(arg, refresh.Timeout)
		}
	}
	providerInstance, err := resolve(providerArg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		osExit(127)
	}

	slog.Debug("Using provider", "provider", providerInstance.Path())

	var verifyProvider func() error
	if v, ok := providerInstance.(interface{ Verify() error }); ok {
		verifyProvider = v.Verify
	}

	code, err := summon.RunSubprocess(&summon.SubprocessConfig{
		Args:           c.Args(),
		Environment:    c.String("environment"),
		Filepath:       c.String("f"),
		YamlInline:     c.String("yaml"),
		Ignores:        c.StringSlice("ignore"),
		IgnoreAll:      c.Bool("ignore-all"),
		RecurseUp:      c.Bool("up"),
		Subs:           c.StringSlice("D"),
		Provider:       providerInstance.Path(),
		FetchSecret:    providerInstance.FetchSecret,
		VerifyProvider: verifyProvider,
		Refresh:        refresh,
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		osExit(127)
	}

	osExit(code)
}

func runPrintProviderVersions() error {
	defaultPath, err := prov.GetDefaultPath()
	if err != nil {
		return err
	}
	output, err := printProviderVersions(defaultPath)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// providerVersionTimeout returns the deadline applied to each binary invoked
// during --all-provider-versions. It reads SUMMON_PROVIDER_VERSION_TIMEOUT
// (a Go duration string); invalid or unset values fall back to 5s.
func providerVersionTimeout() time.Duration {
	const defaultVersionTimeout = 5 * time.Second
	s := os.Getenv("SUMMON_PROVIDER_VERSION_TIMEOUT")
	if s == "" {
		return defaultVersionTimeout
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return defaultVersionTimeout
	}
	return d
}

// printProviderVersions returns a string of all provider versions
func printProviderVersions(providerPath string) (string, error) {
	var providerVersions bytes.Buffer

	fmt.Fprintf(&providerVersions, "Provider versions in %s:\n", providerPath)

	providers, err := prov.GetAllProviders(providerPath)
	if err != nil {
		return "", err
	}

	timeout := providerVersionTimeout()
	for _, provider := range providers {
		if providerPath == "" {
			fmt.Fprintf(&providerVersions, "%s: unknown version\n", provider)
			continue
		}
		providerBin, err := exec.LookPath(filepath.Join(providerPath, provider))
		if err != nil {
			fmt.Fprintf(&providerVersions, "%s: unknown version\n", provider)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		version, err := exec.CommandContext(ctx, providerBin, "--version").Output()
		cancel()
		if err != nil {
			fmt.Fprintf(&providerVersions, "%s: unknown version\n", provider)
			continue
		}

		fmt.Fprintf(&providerVersions, "%s version %s\n", provider, strings.TrimSpace(string(version)))
	}

	return providerVersions.String(), nil
}
