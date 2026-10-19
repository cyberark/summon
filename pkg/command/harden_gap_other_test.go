//go:build !linux && !windows

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformHardeningGap_NotNonDumpable(t *testing.T) {
	gap := platformHardeningGap()
	assert.Contains(t, gap, "not marked non-dumpable")
	assert.NotContains(t, gap, "provider")
}

func TestDefaultApplyNotDumpable_Silent(t *testing.T) {
	buf := captureWarnings(t)
	assert.NoError(t, defaultApplyNotDumpable())
	assert.Empty(t, buf.String())
}
