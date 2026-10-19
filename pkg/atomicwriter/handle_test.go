package atomicwriter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Windows cannot delete a file that is still open, so Cleanup must release the
// temp file's handle before removing it.
func TestCleanupClosesTempFileHandle(t *testing.T) {
	w := newAtomicWriter(filepath.Join(t.TempDir(), "secret"), 0o600, stdOSFuncs).(*atomicWriter)
	_, err := w.Write([]byte("value"))
	require.NoError(t, err)
	f := w.tempFile
	require.NotNil(t, f)

	w.Cleanup()

	_, err = f.Write([]byte("more"))
	assert.ErrorIs(t, err, os.ErrClosed)
	assert.Nil(t, w.tempFile)
}

func TestCloseSyncFailureClosesTempFileHandle(t *testing.T) {
	funcs := stdOSFuncs
	funcs.sync = func(*os.File) error { return assert.AnError }
	w := newAtomicWriter(filepath.Join(t.TempDir(), "secret"), 0o600, funcs).(*atomicWriter)
	_, err := w.Write([]byte("value"))
	require.NoError(t, err)
	f := w.tempFile

	assert.Error(t, w.Close())

	_, err = f.Write([]byte("more"))
	assert.ErrorIs(t, err, os.ErrClosed)
}
