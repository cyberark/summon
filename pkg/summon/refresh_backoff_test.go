//go:build !solaris

package summon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffDelay(t *testing.T) {
	base, ceiling := time.Second, 20*time.Second
	tests := []struct {
		failures uint
		want     time.Duration
	}{
		{0, base},
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, ceiling},
		{500, ceiling}, // no overflow for long outages
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, backoffDelay(tt.failures, base, ceiling), "failures=%d", tt.failures)
	}

	t.Run("ceiling below base wins", func(t *testing.T) {
		assert.Equal(t, 500*time.Millisecond, backoffDelay(3, base, 500*time.Millisecond))
	})
}

func TestJitter(t *testing.T) {
	d := 10 * time.Second
	for i := 0; i < 1000; i++ {
		got := jitter(d)
		assert.GreaterOrEqual(t, got, d-d/10)
		assert.LessOrEqual(t, got, d+d/10)
	}
	assert.Equal(t, time.Duration(0), jitter(0))
}
