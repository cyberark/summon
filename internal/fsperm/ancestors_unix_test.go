//go:build !windows

package fsperm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckAncestors_RefusesForeignOwner(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory owned by root is always accepted")
	}

	err := CheckAncestors(t.TempDir(), os.Geteuid()+1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned")
}

func TestCheckAncestors_RefusesWritableAncestor(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.Chmod(parent, 0o775))

	err := CheckAncestors(sub, os.Geteuid())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable")
}
