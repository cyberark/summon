//go:build windows

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformHardeningGap_Windows(t *testing.T) {
	gap := platformHardeningGap()
	assert.Contains(t, gap, "core-dump suppression")
	assert.Contains(t, gap, "provider binary verification")
	assert.Contains(t, gap, "directory checks")
}

func TestOneShotOnWindowsPrintsNothing(t *testing.T) {
	buf := captureWarnings(t)
	applyCoreDumpLimit = defaultApplyCoreDumpLimit
	suppressCoreDumps()
	assert.Empty(t, buf.String())
}

func TestDefaultApplyNotDumpable_Silent(t *testing.T) {
	buf := captureWarnings(t)
	assert.NoError(t, defaultApplyNotDumpable())
	assert.NoError(t, defaultApplyCoreDumpLimit())
	assert.Empty(t, buf.String())
}
