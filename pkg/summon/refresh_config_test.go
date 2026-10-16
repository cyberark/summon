package summon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestRefreshConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		rc, err := RefreshConfigFromEnv(envFrom(nil))

		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute, rc.Interval)
		assert.Equal(t, 60*time.Second, rc.Timeout)
		assert.Equal(t, 30*time.Second, rc.TermGracePeriod)
		assert.Equal(t, time.Duration(0), rc.MaxOutage)
		assert.True(t, rc.RestartOnRotation)
	})

	t.Run("explicit values", func(t *testing.T) {
		rc, err := RefreshConfigFromEnv(envFrom(map[string]string{
			"SUMMON_REFRESH_INTERVAL":    "10s",
			"SUMMON_REFRESH_TIMEOUT":     "2s",
			"SUMMON_TERM_GRACE_PERIOD":   "5s",
			"SUMMON_MAX_REFRESH_OUTAGE":  "15m",
			"SUMMON_RESTART_ON_ROTATION": "false",
		}))

		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, rc.Interval)
		assert.Equal(t, 2*time.Second, rc.Timeout)
		assert.Equal(t, 5*time.Second, rc.TermGracePeriod)
		assert.Equal(t, 15*time.Minute, rc.MaxOutage)
		assert.False(t, rc.RestartOnRotation)
	})

	for name, tc := range map[string]struct {
		env     map[string]string
		errText string
	}{
		"interval below minimum":    {map[string]string{"SUMMON_REFRESH_INTERVAL": "4s"}, "SUMMON_REFRESH_INTERVAL"},
		"interval not a duration":   {map[string]string{"SUMMON_REFRESH_INTERVAL": "soon"}, "SUMMON_REFRESH_INTERVAL"},
		"timeout zero":              {map[string]string{"SUMMON_REFRESH_TIMEOUT": "0s"}, "SUMMON_REFRESH_TIMEOUT"},
		"grace negative":            {map[string]string{"SUMMON_TERM_GRACE_PERIOD": "-1s"}, "SUMMON_TERM_GRACE_PERIOD"},
		"max outage negative":       {map[string]string{"SUMMON_MAX_REFRESH_OUTAGE": "-1m"}, "SUMMON_MAX_REFRESH_OUTAGE"},
		"max outage malformed":      {map[string]string{"SUMMON_MAX_REFRESH_OUTAGE": "long"}, "SUMMON_MAX_REFRESH_OUTAGE"},
		"max outage bare number":    {map[string]string{"SUMMON_MAX_REFRESH_OUTAGE": "3"}, "SUMMON_MAX_REFRESH_OUTAGE"},
		"max outage below interval": {map[string]string{"SUMMON_REFRESH_INTERVAL": "30s", "SUMMON_MAX_REFRESH_OUTAGE": "10s"}, "SUMMON_MAX_REFRESH_OUTAGE"},
		"restart not a bool":        {map[string]string{"SUMMON_RESTART_ON_ROTATION": "maybe"}, "SUMMON_RESTART_ON_ROTATION"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RefreshConfigFromEnv(envFrom(tc.env))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errText)
		})
	}
}

func TestRefreshConfigMaxOutage(t *testing.T) {
	t.Run("equal to the interval is valid", func(t *testing.T) {
		rc, err := RefreshConfigFromEnv(envFrom(map[string]string{
			"SUMMON_REFRESH_INTERVAL": "30s", "SUMMON_MAX_REFRESH_OUTAGE": "30s",
		}))
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, rc.MaxOutage)
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		rc, err := RefreshConfigFromEnv(envFrom(map[string]string{"SUMMON_MAX_REFRESH_OUTAGE": "0s"}))
		require.NoError(t, err)
		assert.Zero(t, rc.MaxOutage)
	})

	t.Run("removed failure-count variable is ignored", func(t *testing.T) {
		rc, err := RefreshConfigFromEnv(envFrom(map[string]string{"SUMMON_MAX_REFRESH_FAILURES": "3"}))
		require.NoError(t, err)
		assert.Zero(t, rc.MaxOutage)
	})
}

func TestRefreshConfigMaxAuthFailures(t *testing.T) {
	get := func(env map[string]string) (*RefreshConfig, error) { return RefreshConfigFromEnv(envFrom(env)) }

	for name, tc := range map[string]struct {
		env  map[string]string
		want int
	}{
		"default is 5":          {nil, 5},
		"empty is default":      {map[string]string{"SUMMON_MAX_AUTH_FAILURES": ""}, 5},
		"explicit":              {map[string]string{"SUMMON_MAX_AUTH_FAILURES": "3"}, 3},
		"one":                   {map[string]string{"SUMMON_MAX_AUTH_FAILURES": "1"}, 1},
		"zero disables":         {map[string]string{"SUMMON_MAX_AUTH_FAILURES": "0"}, 0},
		"independent of outage": {map[string]string{"SUMMON_MAX_REFRESH_OUTAGE": "10m"}, 5},
	} {
		t.Run(name, func(t *testing.T) {
			rc, err := get(tc.env)
			require.NoError(t, err)
			assert.Equal(t, tc.want, rc.MaxAuthFailures)
		})
	}

	for name, val := range map[string]string{
		"negative": "-1", "malformed": "many", "duration": "5s", "float": "2.5", "padded": " 3",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := get(map[string]string{"SUMMON_MAX_AUTH_FAILURES": val})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "SUMMON_MAX_AUTH_FAILURES")
		})
	}
}
