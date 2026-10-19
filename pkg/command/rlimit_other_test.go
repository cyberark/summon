//go:build !linux && !windows

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultApplyCoreDumpLimit_ZeroesCoreLimit(t *testing.T) {
	assert.NoError(t, defaultApplyCoreDumpLimit())
}
