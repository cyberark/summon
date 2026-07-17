//go:build !linux

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultApplyCoreDumpLimit_NoOpOutsideLinux(t *testing.T) {
	assert.NoError(t, defaultApplyCoreDumpLimit())
}
