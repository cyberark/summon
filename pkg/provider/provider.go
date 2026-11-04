package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cyberark/summon/internal/memzero"
	"github.com/cyberark/summon/pkg/conjurprovider"
	"github.com/cyberark/summon/pkg/secretsyml"
)

// Provider is the interface for a resolved, initialized secrets provider.
// Resolve returns a Provider ready to fetch secrets — callers pass FetchSecret
// directly to summon.SubprocessConfig and use Path for logging and
// interactive-mode dispatch.
type Provider interface {
	// FetchSecret retrieves the secret identified by varID.
	FetchSecret(ctx context.Context, varID string) ([]byte, error)
	// Path returns the provider's identifier: "conjur (built-in)" for the
	// built-in provider or the absolute binary path for external providers.
	Path() string
}

// maxWaitDelay caps how long cmd.Wait lingers after a provider was killed
// because something outside its process group still holds its output pipes.
const maxWaitDelay = 2 * time.Second

// ExternalProvider wraps an external binary and satisfies the Provider
// interface. FetchSecret shells out to the binary via Call.
type ExternalProvider struct {
	path string
	// identity is recorded at Resolve time in daemon mode only. When nil
	// (one-shot mode) Verify is a no-op.
	identity *fileIdentity
}

// Verify re-checks, in daemon mode, that the provider binary and its
// directory still match what Resolve recorded and are not group/world
// writable. Callers must invoke it before every exec of the binary. It is a
// no-op for providers resolved with Resolve.
func (ep *ExternalProvider) Verify() error {
	if ep.identity == nil {
		return nil
	}
	return verifyIdentity(ep.path, ep.identity)
}

// FetchSecret calls the external provider binary with secretID as the sole
// argument and returns its stdout as a byte slice.
// Cancelling ctx kills the provider process (see Call). The binary is
// verified before the exec.
func (ep *ExternalProvider) FetchSecret(ctx context.Context, secretID string) ([]byte, error) {
	if err := ep.Verify(); err != nil {
		return nil, err
	}
	s, err := Call(ctx, ep.path, secretID)
	return []byte(s), err
}

// Path returns the absolute path to the external provider binary.
func (ep *ExternalProvider) Path() string { return ep.path }

// IsBuiltinTarget returns true when the given provider argument should route to
// the built-in Conjur provider rather than an external binary:
//   - providerArg == "conjur" (case-insensitive)                         (explicit built-in flag)
//   - providerArg == ""  AND  SUMMON_PROVIDER == "conjur" (case-insensitive)  (env var names built-in)
//
// When providerArg is empty and SUMMON_PROVIDER is unset or empty, this returns
// false so that Resolve can check the default provider directory first and only
// fall back to the built-in if no external binary is found there.
func IsBuiltinTarget(providerArg string) bool {
	if strings.EqualFold(providerArg, conjurprovider.BuiltinProviderName) {
		return true
	}
	if providerArg == "" {
		envProvider := os.Getenv("SUMMON_PROVIDER")
		return strings.EqualFold(envProvider, conjurprovider.BuiltinProviderName)
	}
	return false
}

// Resolve resolves a provider argument to an initialized Provider instance.
//
// For the built-in Conjur provider it loads Conjur configuration, constructs
// a ConjurProvider client, and returns it. Configuration or authentication
// failures are returned as plain errors from conjurprovider.New().
//
// For external providers it validates the binary path and returns an
// ExternalProvider wrapping that path.
func Resolve(providerArg string) (Provider, error) {
	return resolve(providerArg, false, 0)
}

// ResolveDaemon is Resolve for --refresh mode. For an external provider it
// additionally refuses a binary, directory or ancestor that is group/world-writable
// or owned by another user, execs the symlink-resolved path, and records
// the binary's identity so ExternalProvider.Verify can detect replacement
// before each of the many later execs.
//
// fetchTimeout additionally caps the built-in Conjur provider's HTTP timeout
// (rounded up to whole seconds; the library only takes seconds), so that the
// parts of a fetch the library cannot cancel through a context (token refresh)
// are bounded too. It never raises a timeout the operator configured. Zero
// leaves the default.
func ResolveDaemon(providerArg string, fetchTimeout time.Duration) (Provider, error) {
	return resolve(providerArg, true, fetchTimeout)
}

func resolve(providerArg string, daemon bool, fetchTimeout time.Duration) (Provider, error) {
	if IsBuiltinTarget(providerArg) {
		cp, err := conjurprovider.New(fetchTimeout)
		if err != nil {
			return nil, err
		}
		return cp, nil
	}

	provider := providerArg

	if provider == "" {
		provider = os.Getenv("SUMMON_PROVIDER")
	}

	if provider == "" {
		defaultPath, err := GetDefaultPath()
		if err != nil && !errors.Is(err, ErrNoProviderDir) {
			return nil, err
		}
		// A missing provider directory just means no external provider is
		// installed; fall through to the built-in Conjur provider below.
		if err == nil {
			providers, _ := GetAllProviders(defaultPath)
			if len(providers) == 1 {
				provider = providers[0]
			} else if len(providers) > 1 {
				return nil, fmt.Errorf("more than one provider found in %s, please specify one with --provider or $SUMMON_PROVIDER", defaultPath)
			} else {
				slog.Warn("provider directory contains no executable providers, falling back to built-in Conjur provider", "path", defaultPath)
			}
		}
	}

	if provider == "" {
		// No external provider found in the default directory — fall back to the
		// built-in Conjur provider.
		cp, err := conjurprovider.New(fetchTimeout)
		if err != nil {
			return nil, err
		}
		return cp, nil
	}

	provider, err := expandPath(provider)
	if err != nil {
		return nil, err
	}

	if _, err = os.Stat(provider); err != nil {
		return nil, err
	}

	ep := &ExternalProvider{path: provider}
	if daemon {
		if ep.identity, err = captureIdentity(provider); err != nil {
			return nil, err
		}
		// Exec the path that was verified, not the (swappable) symlink.
		ep.path = ep.identity.execPath(ep.path)
	}
	return ep, nil
}

// Call shells out to a provider and return its output
// If call succeeds, stdout is returned with no error
// If call fails, "" is return with error containing stderr
//
// When ctx is done the provider is killed (on Unix its whole process group)
// and Wait returns even if a grandchild still holds the output pipes; the
// error then wraps ctx.Err(). The SUMMON_PROVIDER_TIMEOUT limit applies on
// top of ctx's own deadline.
func Call(ctx context.Context, provider, specPath string) (string, error) {
	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)
	providerBin, err := exec.LookPath(provider)
	if err != nil {
		return "", fmt.Errorf("provider binary not found: %w", err)
	}
	timeout := callTimeout()
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, providerBin, specPath)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	setCallProcessGroup(cmd, timeout)
	err = cmd.Run()

	if err != nil {
		if ctx.Err() != nil {
			// The caller's context ended (its own deadline or a cancel), not
			// the provider's own time limit.
			return "", fmt.Errorf("provider %q aborted: %w", provider, ctx.Err())
		}
		if callCtx.Err() == context.DeadlineExceeded || errors.Is(err, exec.ErrWaitDelay) {
			// Either the context deadline fired (direct child was killed but a
			// grandchild may still be running) or WaitDelay force-closed the pipe
			// because a grandchild outlived the direct child. Both cases mean the
			// provider exceeded its wall-clock budget.
			return "", providerTimeoutError(provider, timeout)
		}
		return "", &CallError{Err: err, Stderr: strings.TrimSpace(stdErr.String())}
	}

	// Extract the result before zeroing the buffer. stdOut.String() copies the
	// underlying bytes into a new string allocation; clear zeroes the original
	// buffer bytes to narrow the window during which the raw secret value
	// sits in the process heap.
	result := strings.TrimSpace(stdOut.String())
	memzero.Clear(stdOut.Bytes())
	return result, nil
}

// CallError is a failed provider invocation. Error() includes the provider's
// stderr (unchanged one-shot behavior); Redacted() omits it, for long-lived
// logs where provider output may echo secret material or credentials.
type CallError struct {
	Err    error  // the exec error, e.g. "exit status 1"
	Stderr string // trimmed provider stderr; may be empty
}

func (e *CallError) Error() string {
	if e.Stderr == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + e.Stderr
}

func (e *CallError) Unwrap() error { return e.Err }

// Redacted returns the error text without the provider's stderr.
func (e *CallError) Redacted() string { return e.Err.Error() }

// Result is the outcome of fetching a single secret from the provider.
// It pairs the resolved Value with the Key used to request it, plus an
// Error for failed fetches. The Error field is needed here (but absent
// from filetemplates.Secret) because callers inspect it for retry and
// ignore logic before secrets reach the template-rendering stage.
type Result struct {
	Key   string // The secret's identifier (environment variable name or alias).
	Value string // The resolved secret content returned by the provider.
	Error error  // Non-nil when the provider failed to fetch this secret.
}

// ErrInteractiveModeNotSupported is returned when a provider does not support interactive mode
var ErrInteractiveModeNotSupported = errors.New("interactive mode not supported")

const (
	// defaultProviderTimeout is the fallback deadline for external provider
	// binary invocations (both Call and CallInteractiveMode).
	defaultProviderTimeout = 60 * time.Second

	// providerTimeoutEnvVar is the canonical timeout for external provider
	// binaries. It is unrelated to CONJUR_HTTP_TIMEOUT, which conjur-api-go
	// uses as an HTTP client timeout for the built-in Conjur provider.
	providerTimeoutEnvVar = "SUMMON_PROVIDER_TIMEOUT"

	// legacyInteractiveTimeoutEnvVar is the deprecated stream-mode fallback
	// for provider binary timeouts (integer seconds). Prefer SUMMON_PROVIDER_TIMEOUT.
	legacyInteractiveTimeoutEnvVar = "CONJUR_HTTP_TIMEOUT"
)

// callTimeout returns the deadline for one-shot provider.Call invocations.
// Only SUMMON_PROVIDER_TIMEOUT is consulted (Go duration); otherwise 60s.
// Call never reads CONJUR_HTTP_TIMEOUT (that path previously had no timeout).
func callTimeout() time.Duration {
	return resolveProviderTimeout(false)
}

// streamTimeout returns the whole-session deadline for CallInteractiveMode.
// Prefer SUMMON_PROVIDER_TIMEOUT (Go duration); else CONJUR_HTTP_TIMEOUT
// (positive integer seconds, deprecated); else 60s.
func streamTimeout() time.Duration {
	return resolveProviderTimeout(true)
}

func resolveProviderTimeout(forStream bool) time.Duration {
	if d, ok := parseSummonProviderTimeout(); ok {
		return d
	}
	if forStream {
		if d, ok := parseLegacyStreamTimeout(); ok {
			return d
		}
	}
	return defaultProviderTimeout
}

func parseSummonProviderTimeout() (time.Duration, bool) {
	timeoutStr, ok := os.LookupEnv(providerTimeoutEnvVar)
	if !ok || timeoutStr == "" {
		return 0, false
	}
	d, err := time.ParseDuration(timeoutStr)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

func parseLegacyStreamTimeout() (time.Duration, bool) {
	timeoutStr, ok := os.LookupEnv(legacyInteractiveTimeoutEnvVar)
	if !ok || timeoutStr == "" {
		return 0, false
	}
	secs, err := strconv.Atoi(timeoutStr)
	if err != nil || secs <= 0 {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}

func providerTimeoutError(provider string, timeout time.Duration) error {
	return fmt.Errorf("provider %q timed out after %s", provider, timeout)
}

// CallInteractiveMode calls a provider without passing any arguments. It then constantly fetches
// secrets from its stdout. It returns a channel of results, a channel of errors and a cleanup function.
//
// When ctx is done the provider is killed (on Unix its whole process group)
// and an error wrapping ctx.Err() is sent on the errors channel.
func CallInteractiveMode(ctx context.Context, provider string, secrets secretsyml.SecretsMap) (chan Result, chan error, func()) {
	resultsCh := make(chan Result)
	errorsCh := make(chan error, 1)
	// done is closed by the returned cleanup func. Every send to errorsCh goes
	// through sendErr so no goroutine blocks forever once the receiver is gone.
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }
	sendErr := func(err error) {
		// Prefer delivering into the buffer so the first error always arrives.
		select {
		case errorsCh <- err:
			return
		default:
		}
		select {
		case errorsCh <- err:
		case <-done:
		}
	}
	timeout := streamTimeout()
	ctxTimeout, ctxCancel := context.WithTimeout(ctx, timeout)
	// ended is the error to report once ctxTimeout is done: the caller's
	// context error, or the provider time limit.
	ended := func() error {
		if ctx.Err() != nil {
			return fmt.Errorf("provider %q aborted: %w", provider, ctx.Err())
		}
		return providerTimeoutError(provider, timeout)
	}

	providerBin, lookErr := exec.LookPath(provider)
	if lookErr != nil {
		sendErr(fmt.Errorf("provider binary not found: %w", lookErr))
		return resultsCh, errorsCh, func() { closeDone(); ctxCancel() }
	}

	cmd := exec.CommandContext(ctxTimeout, providerBin)
	setCallProcessGroup(cmd, timeout)

	// Get a pipe to the command's stdinPipe
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		errorsCh <- err
		return resultsCh, errorsCh, func() { closeDone(); ctxCancel() }
	}
	// Get a pipe to the command's stdoutPipe
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		errorsCh <- err
		return resultsCh, errorsCh, func() {
			closeDone()
			stdinPipe.Close()
			ctxCancel()
		}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		errorsCh <- err
		return resultsCh, errorsCh, func() {
			closeDone()
			stdinPipe.Close()
			stdoutPipe.Close()
			ctxCancel()
		}
	}

	// doWait calls cmd.Wait() exactly once across both cleanup and the
	// context-watching goroutine below. cmd.Wait() is required for WaitDelay
	// to activate: without it the Go runtime never force-closes the I/O
	// pipes, so a grandchild holding the stdout pipe open causes the reader
	// goroutine to block indefinitely past the deadline.
	//
	// cmd.Wait() returns "exec: not started" when Process is nil (Start
	// failed), which is harmless — we discard the error. This ensures Wait
	// is always attempted regardless of call-site ordering.
	var waitOnce sync.Once
	doWait := func() {
		waitOnce.Do(func() {
			_ = cmd.Wait()
		})
	}

	cleanup := func() {
		closeDone()
		stdinPipe.Close()
		stdoutPipe.Close()
		stderrPipe.Close()
		ctxCancel()
		doWait()
	}

	err = cmd.Start()

	if err != nil {
		errorsCh <- err
		return resultsCh, errorsCh, cleanup
	}

	// When the context ends, exec.CommandContext kills the provider (its whole
	// process group on Unix, see setCallProcessGroup). Call cmd.Wait()
	// immediately so WaitDelay can force-close the stdout pipe if a
	// grandchild outside the group holds it open — otherwise the reader goroutine below blocks
	// indefinitely, preventing handleResultsFromProvider from returning and
	// cleanup() from ever being reached.
	go func() {
		<-ctxTimeout.Done()
		doWait()
	}()

	secretEnvVarCh := make(chan string, len(secrets))

	// This goroutine sends the paths of the secrets to the stdin of a secrets provider
	go func() {
		for key, spec := range secrets {
			slog.Debug("Fetching secret", "name", key, "provider", provider)
			_, err := fmt.Fprintln(stdinPipe, spec.Path)
			if err != nil {
				sendErr(ErrInteractiveModeNotSupported)
				break
			}
			secretEnvVarCh <- key
		}
	}()

	// This goroutine reads from the stdoutPipe of a secrets provider and sends the results to the results channel.
	// After all secrets are read, it closes the results channel
	go func() {
		defer close(resultsCh)
		reader := bufio.NewReader(stdoutPipe)
		index := 0

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if ctxTimeout.Err() != nil {
					sendErr(ended())
					break
				}
				if err == io.EOF {
					break
				}
				sendErr(err)
				continue
			}

			line = strings.TrimRight(line, "\r\n")
			decoded, err := base64.StdEncoding.DecodeString(line)
			if err != nil {
				sendErr(fmt.Errorf("failed to decode base64 string: %w", err))
				continue
			}

			var keyVal string
			select {
			case keyVal = <-secretEnvVarCh:
			case <-ctxTimeout.Done():
				sendErr(ended())
				memzero.Clear(decoded)
				return
			case <-done:
				memzero.Clear(decoded)
				return
			}
			res := Result{
				Key:   keyVal,
				Value: string(decoded),
				Error: nil,
			}
			memzero.Clear(decoded)
			select {
			case resultsCh <- res:
			case <-ctxTimeout.Done():
				sendErr(ended())
				return
			case <-done:
				return
			}
			index++
			if index >= len(secrets) {
				break
			}

		}
		if index == 0 && ctxTimeout.Err() == nil {
			sendErr(ErrInteractiveModeNotSupported)
		}

	}()
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			sendErr(errors.New(line))
		}
	}()
	return resultsCh, errorsCh, cleanup
}

// Given a provider name, it returns a path to executable prefixed with DefaultPath. If
// the provider has any other pattern (eg. `./provider-name`, `/foo/provider-name`), the
// parameter is assumed to be a path to the provider and not just a name.
func expandPath(provider string) (string, error) {
	// Base returns just the last path segment.
	// If it's different, that means it's a (rel or abs) path
	if filepath.Base(provider) != provider {
		return filepath.Abs(provider)
	}

	defaultPath, err := GetDefaultPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(defaultPath, provider), nil
}

// windowsProgramFilesDir returns programW6432 (the value of %ProgramW6432%), or
// a drive-absolute fallback when it is unset. The fallback is a literal:
// filepath.Join("C:", "Program Files") would be the drive-relative "C:Program Files".
func windowsProgramFilesDir(programW6432 string) string {
	if programW6432 != "" {
		return programW6432
	}
	return `C:\Program Files`
}

// ErrNoProviderDir is returned (wrapped) by GetDefaultPath when no provider
// directory exists. Resolve treats it as "no external provider installed".
var ErrNoProviderDir = errors.New("no provider directory found")

func GetDefaultPath() (string, error) {
	pathOverride := os.Getenv("SUMMON_PROVIDER_PATH")

	if pathOverride != "" {
		return pathOverride, nil
	}

	dir := "/usr/local/lib/summon"

	if runtime.GOOS == "windows" {
		// Try to get the appropriate "Program Files" directory but if one doesn't
		// exist, use a hardcoded value we think should be right.
		programFilesDir := windowsProgramFilesDir(os.Getenv("ProgramW6432"))

		dir = filepath.Join(programFilesDir, "Cyberark Conjur", "Summon", "Providers")
	}

	// found default installation directory
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	// Enable portable installation with Providers dir next to executable
	// if the direcotries above were not found

	// eg ~/brew/bin/summon
	exec, _ := os.Executable()

	// eg ~/brew/bin
	execDir := filepath.Dir(exec)

	// eg ~/brew/bin/Providers
	providersDir := filepath.Join(execDir, "Providers")

	if _, err := os.Stat(providersDir); err == nil {
		return providersDir, nil
	}

	// Homebrew installs summon-conjur to ~/brew/lib/summon

	// Dir removes the last element in a path, so can be used to go
	// up the file tree, not just splitting file from path.

	// eg ~/brew
	baseDir := filepath.Dir(execDir)

	// libDir = ~/brew/lib/summon
	libDir := filepath.Join(baseDir, "lib", "summon")

	if _, err := os.Stat(libDir); err == nil {
		return libDir, nil
	}

	return "", fmt.Errorf("%w. Please set the "+
		"environment variable SUMMON_PROVIDER_PATH to the directory "+
		"containing providers.\n"+
		"Provider paths searched: \n"+
		"	/usr/local/lib/summon\n"+
		"	${summon bin dir}/Providers,\n"+
		"	${summon bin dir}/../lib/summon\n"+
		"	Environment Variable: SUMMON_PROVIDER_PATH\n"+
		"	C:\\Program Files\\Cyberark Conjur\\Summon\\Providers", ErrNoProviderDir)
}

// GetAllProviders returns the names of provider files in providerDir.
// Directories are always skipped. Whether a non-directory entry qualifies as a
// provider is determined by isProviderFile, which is platform-specific:
// on Unix it requires at least one execute bit; on Windows all regular files
// qualify because Windows does not expose execute bits via FileMode.
func GetAllProviders(providerDir string) ([]string, error) {
	entries, err := os.ReadDir(providerDir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsDir() {
			slog.Debug("Skipping provider directory entry", "name", entry.Name(), "reason", "is a directory")
			continue
		}
		// Use os.Stat (follows symlinks) so a symlink to a directory or
		// non-executable target is correctly excluded rather than passing
		// the Lstat-mode check that always returns 0777 for symlinks.
		info, err := os.Stat(filepath.Join(providerDir, entry.Name()))
		if err != nil {
			slog.Debug("Skipping provider directory entry", "name", entry.Name(), "reason", "not readable", "error", err)
			continue
		}
		if info.IsDir() {
			slog.Debug("Skipping provider directory entry", "name", entry.Name(), "reason", "resolves to a directory")
			continue
		}
		if !isProviderFile(info) {
			slog.Debug("Skipping provider directory entry", "name", entry.Name(), "reason", "not executable")
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}
