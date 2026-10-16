package summon

import (
	"fmt"
	"strconv"
	"time"
)

const (
	defaultRefreshInterval = 5 * time.Minute
	minRefreshInterval     = 5 * time.Second
	defaultRefreshTimeout  = 60 * time.Second
	defaultTermGrace       = 30 * time.Second
	defaultMaxAuthFailures = 5
)

// RefreshConfig holds the daemon-mode settings. A nil *RefreshConfig in
// SubprocessConfig means one-shot behavior.
type RefreshConfig struct {
	// Interval between polls of the secrets provider.
	Interval time.Duration
	// Timeout for a single poll; exceeding it counts as a fetch failure.
	Timeout time.Duration
	// TermGracePeriod is how long a restart waits for the subprocess tree to
	// stop gracefully before force-killing it.
	TermGracePeriod time.Duration
	// MaxOutage is the wall-clock time the provider may stay unavailable
	// (measured from the first failed poll after the last fully successful
	// one) before the subprocess is terminated; 0 = unlimited.
	MaxOutage time.Duration
	// MaxAuthFailures is the number of consecutive polls answered with HTTP
	// 401 after which the subprocess is terminated; 0 disables the check.
	MaxAuthFailures int
	// RestartOnRotation restarts the subprocess when any secret changes.
	// When false only polling and push-to-file rewrites happen.
	RestartOnRotation bool
}

// RefreshConfigFromEnv reads the SUMMON_* refresh settings via getenv and
// validates them.
func RefreshConfigFromEnv(getenv func(string) string) (*RefreshConfig, error) {
	rc := &RefreshConfig{
		Interval:          defaultRefreshInterval,
		Timeout:           defaultRefreshTimeout,
		TermGracePeriod:   defaultTermGrace,
		RestartOnRotation: true,
	}

	var err error
	if rc.Interval, err = durationEnv(getenv, "SUMMON_REFRESH_INTERVAL", rc.Interval); err != nil {
		return nil, err
	}
	if rc.Interval < minRefreshInterval {
		return nil, fmt.Errorf("SUMMON_REFRESH_INTERVAL must be at least %s", minRefreshInterval)
	}
	if rc.Timeout, err = durationEnv(getenv, "SUMMON_REFRESH_TIMEOUT", rc.Timeout); err != nil {
		return nil, err
	}
	if rc.Timeout <= 0 {
		return nil, fmt.Errorf("SUMMON_REFRESH_TIMEOUT must be greater than zero")
	}
	if rc.TermGracePeriod, err = durationEnv(getenv, "SUMMON_TERM_GRACE_PERIOD", rc.TermGracePeriod); err != nil {
		return nil, err
	}
	if rc.TermGracePeriod <= 0 {
		return nil, fmt.Errorf("SUMMON_TERM_GRACE_PERIOD must be greater than zero")
	}

	if rc.MaxOutage, err = durationEnv(getenv, "SUMMON_MAX_REFRESH_OUTAGE", 0); err != nil {
		return nil, err
	}
	if rc.MaxOutage < 0 {
		return nil, fmt.Errorf("SUMMON_MAX_REFRESH_OUTAGE must not be negative: %q", getenv("SUMMON_MAX_REFRESH_OUTAGE"))
	}
	if rc.MaxOutage > 0 && rc.MaxOutage < rc.Interval {
		return nil, fmt.Errorf("SUMMON_MAX_REFRESH_OUTAGE (%s) must be at least SUMMON_REFRESH_INTERVAL (%s), or 0 for unlimited", rc.MaxOutage, rc.Interval)
	}
	rc.MaxAuthFailures = defaultMaxAuthFailures
	if v := getenv("SUMMON_MAX_AUTH_FAILURES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("SUMMON_MAX_AUTH_FAILURES must be a non-negative integer (0 disables): %q", v)
		}
		rc.MaxAuthFailures = n
	}
	if v := getenv("SUMMON_RESTART_ON_ROTATION"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("SUMMON_RESTART_ON_ROTATION must be a boolean: %q", v)
		}
		rc.RestartOnRotation = b
	}
	return rc, nil
}

func durationEnv(getenv func(string) string, name string, def time.Duration) (time.Duration, error) {
	v := getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 30s or 5m: %q", name, v)
	}
	return d, nil
}
