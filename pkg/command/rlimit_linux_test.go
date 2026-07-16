//go:build linux

package command

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultApplyCoreDumpLimit_SetsRLIMIT_CORE(t *testing.T) {
	err := defaultApplyCoreDumpLimit()
	if err != nil {
		t.Skipf("Setrlimit not permitted in this environment: %v", err)
	}

	var lim syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_CORE, &lim))
	assert.Equal(t, uint64(0), lim.Cur)
	assert.Equal(t, uint64(0), lim.Max)
}
