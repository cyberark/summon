//go:build !solaris

package summon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cyberark/summon/pkg/conjurprovider"
	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/pushtofile"
	"github.com/cyberark/summon/pkg/secretsyml"
)

// retryInterval is the delay before the first retry after a failed poll. Each
// further consecutive failure doubles it (see backoffDelay), capped at the
// regular poll interval so a failing provider is never polled more often than
// a healthy one would be late.
var retryInterval = time.Second

// backoffDelay returns the wait before the next poll after the given number of
// consecutive failures: base, 2*base, 4*base, ... capped at ceiling.
func backoffDelay(failures uint, base, ceiling time.Duration) time.Duration {
	d := base
	for i := uint(1); i < failures && d < ceiling; i++ {
		d *= 2
	}
	return min(d, ceiling)
}

// jitter spreads d by up to ±10% so that many Summon instances that failed
// together do not retry in lockstep.
func jitter(d time.Duration) time.Duration {
	spread := int64(d / 10)
	if spread <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(2*spread+1)-spread)
}

// refreshClock is the refresher's clock (replaceable in tests).
var refreshClock = time.Now

// exitCodeOutageLimit is returned when the outage limit terminates the
// subprocess, distinguishing it from a child crash or a setup failure.
const exitCodeOutageLimit = 2

// exitCodeAuthFailed is returned when the subprocess is terminated after
// SUMMON_MAX_AUTH_FAILURES consecutive polls were rejected with HTTP 401.
const exitCodeAuthFailed = 3

// refresher runs the subprocess and keeps its secrets fresh: it polls the
// provider on an interval, rewrites changed secret files, and restarts the
// subprocess when secrets change.
type refresher struct {
	ctx    context.Context
	sc     *SubprocessConfig
	config *secretsyml.ParsedConfig
	rc     *RefreshConfig
	root   *TempFactory

	// Baseline of the last applied poll: per-key checksums of env secrets and
	// per-file checksums of rendered file content.
	keySums  map[string]checksum
	fileSums map[string]string
	// fileNonEmpty records which file secrets last had a non-empty value.
	fileNonEmpty map[string]bool

	// ignored lists the secrets --ignore/--ignore-all dropped at startup;
	// pollConfig is config without them (see ignoredAtStartup).
	ignored    []string
	pollConfig *secretsyml.ParsedConfig

	// failures counts consecutive failed polls for the retry backoff only.
	failures uint
	// authFailures counts consecutive polls the server rejected with HTTP 401.
	authFailures int
	// outageStart is when the first failed poll since the last fully
	// successful one began; zero while healthy. pollStart is the start of the
	// poll in progress. now is the clock (replaceable in tests).
	outageStart time.Time
	pollStart   time.Time
	now         func() time.Time

	child       *supervisedChild // only touched by the refresh loop goroutine
	tree        treeGuard        // shared with the signal forwarder
	cycleTF     *TempFactory     // owns the temp files of the running child's env
	terminating atomic.Bool
}

type fileWriteError struct {
	path string
	err  error
}

// pollOutcome tells the loop what to do after a poll.
type pollOutcome struct {
	done   bool // leave the loop with code/err
	code   int
	err    error
	failed bool // retry sooner than the regular interval
}

// beforeStartChildHook runs right before the first child is started; tests use
// it to deliver a signal in that window.
var beforeStartChildHook = func() {}

func runRefreshing(ctx context.Context, sc *SubprocessConfig, config *secretsyml.ParsedConfig, root *TempFactory) (code int, err error) {
	// A stop signal cancels ctx, which makes the loop stop the child.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	r := &refresher{
		ctx:      ctx,
		sc:       sc,
		config:   config,
		rc:       sc.Refresh,
		root:     root,
		fileSums: map[string]string{},
		now:      refreshClock,

		fileNonEmpty: map[string]bool{},
	}
	defer func() {
		if r.cycleTF != nil {
			r.cycleTF.Cleanup()
		}
	}()

	// The startup fetch seeds the baseline; the first poll diffs against it.
	// It has no timeout and fails hard, like a one-shot run.
	cycleSC := r.cycleConfig(true)
	tf := newCycleTempFactory(root)
	fetched, err := fetchCycle(context.Background(), config, cycleSC, tf)
	if err != nil {
		tf.Cleanup()
		return 0, err
	}
	r.cycleTF = tf
	r.ignored = ignoredAtStartup(config, sc, fetched)
	r.pollConfig = withoutSecrets(config, r.ignored)
	if r.keySums, err = checksumResults(fetched.envResults, config.EnvSecrets); err != nil {
		return 0, err
	}
	r.recordFileValues(fetched.fileResults)
	if _, writeErrs := r.writeFiles(fetched.fileResults, true); len(writeErrs) > 0 {
		return 0, fmt.Errorf("error writing secret file for path %s: %v", writeErrs[0].path, writeErrs[0].err)
	}

	// The built-in provider holds its credentials in memory. An external
	// provider is re-invoked on every poll and still needs them in the
	// environment, so they stay until Summon exits.
	if sc.Provider == conjurprovider.BuiltinProviderPath {
		clearConjurEnvFromProcess()
	}
	if r.rc.RestartOnRotation {
		warnIfGracefulStopUnavailable()
	}

	// Like one-shot mode, handle signals before the child exists: a stop
	// signal in between must not kill Summon and orphan the child.
	done := make(chan struct{})
	defer close(done)
	forwardSignals(&r.tree, &r.terminating, stop, done)

	beforeStartChildHook()
	if ctx.Err() != nil {
		return 0, nil
	}
	child, err := startChild(fetched.args, childEnv(fetched.env), r.rc.TermGracePeriod, &r.tree)
	if err != nil {
		return 0, err
	}
	r.child = child
	// Whatever aborts the loop, the subprocess tree must not outlive Summon.
	defer func() {
		if v := recover(); v != nil {
			perr := recoveredPanic("refresh loop", v)
			slog.Error("Refresh loop aborted by an unexpected panic; terminating subprocess", "error", perr.Error())
			r.child.stop()
			code, err = 0, fmt.Errorf("refresh loop aborted: %w", perr)
		}
	}()

	slog.Info("Refresh enabled",
		"interval", r.rc.Interval, "restart_on_rotation", r.rc.RestartOnRotation)
	return r.loop()
}

// cycleConfig returns a per-cycle copy of the config: @SUMMONENVFILE is
// resolved in Args in place, so every cycle starts from the original args.
//
// --ignore/--ignore-all apply to the startup fetch only. A secret that was
// ignored at startup (it failed) keeps that state for the whole run: later
// polls do not fetch it, so it never counts as a failed poll, never blocks
// change detection of the others and is never rewritten, even if it would
// start succeeding. Every other secret that fails on a later poll fails the
// poll and the last-known-good one is kept.
func (r *refresher) cycleConfig(startup bool) *SubprocessConfig {
	cycle := *r.sc
	cycle.Args = slices.Clone(r.sc.Args)
	if !startup {
		cycle.Ignores, cycle.IgnoreAll = nil, false
		// A panic while polling is a failed poll, not a crash.
		cycle.FetchSecret = recoverFetch(r.sc.FetchSecret)
	}
	return &cycle
}

func childEnv(env []string) []string {
	return append(filterConjurEnv(os.Environ()), env...)
}

func (r *refresher) loop() (int, error) {
	timer := time.NewTimer(r.rc.Interval)
	defer timer.Stop()
	for {
		child := r.child
		select {
		case <-child.exited:
			child.tree.close()
			restoreTerminalForeground()
			return returnStatusOfError(child.err)
		case <-r.ctx.Done():
			child.stop()
			// Report the child's own exit code; death by our stop signal is a
			// clean shutdown.
			return stoppedChildStatus(child.err)
		case <-timer.C:
			next := r.rc.Interval
			outcome := r.poll()
			if outcome.done {
				return outcome.code, outcome.err
			}
			if outcome.failed {
				next = min(jitter(backoffDelay(r.failures, retryInterval, r.rc.Interval)), r.rc.Interval)
			}
			timer.Reset(next)
		}
	}
}

// loggableError returns err's text for the always-on logs. External provider
// stderr can echo credentials or secret material, so it is only included when
// debug logging is enabled.
func loggableError(err error) string {
	var ce *prov.CallError
	if errors.As(err, &ce) && !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return strings.Replace(err.Error(), ce.Error(), ce.Redacted(), 1)
	}
	return err.Error()
}

// poll runs one Fetching → Diffing → Writing → Restarting cycle.
func (r *refresher) poll() pollOutcome {
	r.pollStart = r.now()
	fetched, tf, err := r.fetchWithTimeout()
	if err != nil {
		if r.ctx.Err() != nil {
			return pollOutcome{}
		}
		slog.Warn("Secrets refresh failed; subprocess keeps last-known-good secrets", "error", loggableError(err))
		return r.failure(err)
	}

	sums, err := checksumResults(fetched.envResults, r.config.EnvSecrets)
	if err != nil {
		tf.Cleanup()
		slog.Warn("Secrets refresh failed; subprocess keeps last-known-good secrets", "error", err)
		return r.failure(nil)
	}
	if emptied := r.emptiedKeys(sums, fetched.fileResults); len(emptied) > 0 {
		tf.Cleanup()
		slog.Warn("Secrets refresh failed: provider returned an empty value for secrets that were non-empty; subprocess keeps last-known-good secrets",
			"variables", emptied)
		return r.failure(nil)
	}
	r.recordFileValues(fetched.fileResults)
	changedKeys := diffKeys(r.keySums, sums)

	written, writeErrs := r.writeFiles(fetched.fileResults, false)
	logWriteErrors(writeErrs)
	cycleOK := len(writeErrs) == 0

	if len(changedKeys) == 0 && len(written) == 0 {
		slog.Debug("Poll complete: no change")
		tf.Cleanup()
		return r.cycleDone(cycleOK)
	}
	r.keySums = sums
	slog.Info("Secrets changed", "changed_variables", changedKeys, "updated_files", written)

	if !r.rc.RestartOnRotation {
		slog.Info("Restart on rotation is disabled; subprocess left running")
		tf.Cleanup()
		return r.cycleDone(cycleOK)
	}
	return r.restart(fetched, tf, cycleOK)
}

// logWriteErrors logs failed secret file writes. Their errors can wrap
// provider stderr, so they go through loggableError.
func logWriteErrors(errs []fileWriteError) {
	for _, we := range errs {
		slog.Error("Secret file write failed", "path", we.path, "error", loggableError(we.err))
	}
}

// emptiedKeys returns, sorted, the secrets whose last-known-good value was
// non-empty but which the poll returned empty. A partial or empty provider
// answer must not be applied; a secret that was empty at startup is not
// reported until it has had a non-empty value.
func (r *refresher) emptiedKeys(sums map[string]checksum, fileResults []prov.Result) []string {
	empty := sumOf("")
	var keys []string
	for k, sum := range sums {
		if prev, ok := r.keySums[k]; ok && prev != empty && sum == empty {
			keys = append(keys, k)
		}
	}
	for _, res := range fileResults {
		if res.Error == nil && res.Value == "" && r.fileNonEmpty[res.Key] {
			keys = append(keys, res.Key)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// recordFileValues remembers which file secrets are currently non-empty.
func (r *refresher) recordFileValues(results []prov.Result) {
	for _, res := range results {
		if res.Error == nil {
			r.fileNonEmpty[res.Key] = res.Value != ""
		}
	}
}

// restart stops the running subprocess tree and relaunches it with the env
// built from the triggering poll.
func (r *refresher) restart(fetched *fetchedCycle, tf *TempFactory, cycleOK bool) pollOutcome {
	old := r.child
	slog.Info("Restarting subprocess")
	// An old child that is already gone exited on its own (crashed or quit);
	// stop() would hide that, so look before stopping.
	select {
	case <-old.exited:
		logChildExitBeforeRestart(old.err)
	default:
	}
	if old.stop() {
		slog.Warn("Subprocess did not stop within the grace period; force-killed",
			"grace_period", r.rc.TermGracePeriod)
	}
	r.cycleTF.Cleanup()
	r.cycleTF = nil

	if r.terminating.Load() {
		// Summon itself was told to stop while restarting: do not relaunch.
		tf.Cleanup()
		// Death by our own stop signal is a clean shutdown (see loop).
		code, _ := stoppedChildStatus(old.err)
		return pollOutcome{done: true, code: code}
	}

	child, err := startChild(fetched.args, childEnv(fetched.env), r.rc.TermGracePeriod, &r.tree)
	if err != nil {
		tf.Cleanup()
		return pollOutcome{done: true, err: fmt.Errorf("restart failed: %w", err)}
	}
	r.child = child
	r.cycleTF = tf
	slog.Info("Subprocess restarted")
	return r.cycleDone(cycleOK)
}

// cycleDone closes a cycle: a fully successful one ends any outage.
func (r *refresher) cycleDone(ok bool) pollOutcome {
	if !ok {
		return r.failure(nil)
	}
	r.failures = 0
	r.authFailures = 0
	r.outageStart = time.Time{}
	return pollOutcome{}
}

// failure records a failed cycle and terminates the subprocess once the
// outage has lasted SUMMON_MAX_REFRESH_OUTAGE (wall clock only; the number of
// retries does not count) or SUMMON_MAX_AUTH_FAILURES consecutive polls were
// answered with HTTP 401, whichever comes first. fetchErr is the fetch error
// of the poll, nil for failures that are not provider errors.
func (r *refresher) failure(fetchErr error) pollOutcome {
	r.failures++
	if conjurprovider.IsUnauthorized(fetchErr) {
		r.authFailures++
	} else {
		r.authFailures = 0
	}
	if limit := r.rc.MaxAuthFailures; limit > 0 && r.authFailures >= limit {
		slog.Error("Conjur rejected the host credentials with HTTP 401 on consecutive refresh polls; "+
			"the host's API key may have been rotated. Terminating subprocess",
			"consecutive_http_401", r.authFailures, "limit", limit)
		r.child.stop()
		return pollOutcome{done: true, code: exitCodeAuthFailed}
	}
	if r.outageStart.IsZero() {
		r.outageStart = r.pollStart
	}
	if outage := r.now().Sub(r.outageStart); r.rc.MaxOutage > 0 && outage >= r.rc.MaxOutage {
		slog.Error("Maximum refresh outage reached; terminating subprocess",
			"outage", outage.Round(time.Millisecond), "limit", r.rc.MaxOutage)
		r.child.stop()
		return pollOutcome{done: true, code: exitCodeOutageLimit}
	}
	return pollOutcome{failed: true}
}

// fetchWithTimeout runs one fetch cycle bounded by SUMMON_REFRESH_TIMEOUT. The
// timeout and a stop signal (r.ctx) cancel the context the fetch runs under:
// an external provider is killed and the built-in client's request is aborted
// (see the provider and conjurprovider packages). The temp files of a failed
// or aborted fetch are removed before it returns.
func (r *refresher) fetchWithTimeout() (*fetchedCycle, *TempFactory, error) {
	ctx, cancel := context.WithTimeout(r.ctx, r.rc.Timeout)
	defer cancel()

	tf := newCycleTempFactory(r.root)
	fetched, err := r.fetchRecovering(ctx, tf)
	if err != nil {
		tf.Cleanup()
		if r.ctx.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, fmt.Errorf("fetch exceeded %s", r.rc.Timeout)
		}
		return nil, nil, err
	}
	return fetched, tf, nil
}

// fetchRecovering runs the fetch cycle and turns a panic of the fetch step
// into an error. It is the safety net for panics not raised by FetchSecret
// itself (see recoverFetch), e.g. in provider-specific code.
func (r *refresher) fetchRecovering(ctx context.Context, tf *TempFactory) (fetched *fetchedCycle, err error) {
	defer func() {
		if v := recover(); v != nil {
			fetched, err = nil, recoveredPanic("fetch", v)
		}
	}()
	return fetchCycle(ctx, r.pollConfig, r.cycleConfig(false), tf)
}

// writeFiles writes every configured file whose rendered content changed since
// the last write and records the new checksums. One failing file does not stop
// the others. startup selects whether --ignore applies (see cycleConfig).
func (r *refresher) writeFiles(results []prov.Result, startup bool) (written []string, errs []fileWriteError) {
	for _, fileConfig := range r.config.Files {
		if err := fileConfig.Validate(); err != nil {
			errs = append(errs, fileWriteError{fileConfig.Path, err})
			continue
		}
		file := pushtofile.SecretFile{FileConfig: fileConfig, Daemon: true}
		if startup {
			file.Ignores, file.IgnoreAll = r.sc.Ignores, r.sc.IgnoreAll
		} else {
			file.Ignores = r.ignored
		}
		prev := r.fileSums[fileConfig.Path]
		path, sum, wrote, err := file.WriteIfChanged(results, prev)
		if err != nil {
			errs = append(errs, fileWriteError{fileConfig.Path, err})
			continue
		}
		if wrote {
			if prev == "" {
				r.root.AddFile(path) // removed when Summon exits
			}
			written = append(written, path)
		}
		r.fileSums[fileConfig.Path] = sum
	}
	return written, errs
}

// ignoredAtStartup returns, sorted, the keys --ignore/--ignore-all allowed to
// be missing from the startup fetch, i.e. those without a successful result.
func ignoredAtStartup(config *secretsyml.ParsedConfig, sc *SubprocessConfig, fetched *fetchedCycle) []string {
	if !sc.IgnoreAll && len(sc.Ignores) == 0 {
		return nil
	}
	ok := map[string]bool{}
	for _, res := range slices.Concat(fetched.envResults, fetched.fileResults) {
		if res.Error == nil {
			ok[res.Key] = true
		}
	}
	var keys []string
	add := func(specs secretsyml.SecretsMap) {
		for key, spec := range specs {
			if spec.IsVar() && !ok[key] && (sc.IgnoreAll || slices.Contains(sc.Ignores, key)) {
				keys = append(keys, key)
			}
		}
	}
	add(config.EnvSecrets)
	add(config.FileSecrets())
	slices.Sort(keys)
	return slices.Compact(keys)
}

// withoutSecrets returns a copy of config minus the given secret keys. Files
// keep their specs; the dropped aliases are tolerated as missing when written.
func withoutSecrets(config *secretsyml.ParsedConfig, drop []string) *secretsyml.ParsedConfig {
	if len(drop) == 0 {
		return config
	}
	filter := func(specs secretsyml.SecretsMap) secretsyml.SecretsMap {
		kept := make(secretsyml.SecretsMap, len(specs))
		for key, spec := range specs {
			if !slices.Contains(drop, key) {
				kept[key] = spec
			}
		}
		return kept
	}
	out := &secretsyml.ParsedConfig{EnvSecrets: filter(config.EnvSecrets)}
	for _, fc := range config.Files {
		fc.Secrets = filter(fc.Secrets.(secretsyml.SecretsMap))
		out.Files = append(out.Files, fc)
	}
	return out
}

// logChildExitBeforeRestart reports a child that exited on its own before a
// restart stopped it. Only the exit status is logged, never the command line
// or environment.
func logChildExitBeforeRestart(err error) {
	if err == nil {
		slog.Warn("Subprocess exited before restart", "exit_status", 0)
		return
	}
	if code, serr := returnStatusOfError(err); serr == nil {
		slog.Warn("Subprocess exited before restart", "exit_status", code)
		return
	}
	// Death by signal (or another wait error): the error text names only the
	// signal.
	slog.Warn("Subprocess exited before restart", "error", err.Error())
}

// stoppedChildStatus is the exit status to report for a child Summon stopped
// itself. Death by our stop signal is a clean shutdown (0); an exit code is
// passed through stopExitCode, which on Windows folds the codes a stop causes
// into 0.
func stoppedChildStatus(err error) (int, error) {
	code, serr := returnStatusOfError(err)
	if serr != nil {
		return 0, nil
	}
	return stopExitCode(code), nil
}
