//go:build linux

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformHardeningGap_LinuxHasNone(t *testing.T) {
	assert.Empty(t, platformHardeningGap())
}
