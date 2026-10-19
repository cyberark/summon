package atomicwriter

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncDirAfterRename(t *testing.T) {
	t.Run("durable writer fsyncs the directory after rename", func(t *testing.T) {
		dir := t.TempDir()
		funcs, counts := newTestOSFuncs(injectErrors{})
		var synced []string
		funcs.syncDir = func(d string) error {
			assert.Equal(t, 1, counts.renameCount, "directory sync must follow the rename")
			synced = append(synced, d)
			return nil
		}
		w := newAtomicWriter(filepath.Join(dir, "f"), 0o600, funcs)
		w.(*atomicWriter).syncDir = true

		_, err := w.Write([]byte("x"))
		require.NoError(t, err)
		require.NoError(t, w.Close())

		assert.Equal(t, []string{dir}, synced)
	})

	t.Run("directory sync failure is reported", func(t *testing.T) {
		funcs, _ := newTestOSFuncs(injectErrors{})
		funcs.syncDir = func(string) error { return errors.New("eio") }
		w := newAtomicWriter(filepath.Join(t.TempDir(), "f"), 0o600, funcs)
		w.(*atomicWriter).syncDir = true

		_, err := w.Write([]byte("x"))
		require.NoError(t, err)

		assert.Error(t, w.Close())
	})

	t.Run("default writer does not sync the directory", func(t *testing.T) {
		funcs, _ := newTestOSFuncs(injectErrors{})
		funcs.syncDir = func(string) error { t.Error("unexpected directory sync"); return nil }
		w := newAtomicWriter(filepath.Join(t.TempDir(), "f"), 0o600, funcs)

		_, err := w.Write([]byte("x"))
		require.NoError(t, err)
		require.NoError(t, w.Close())
	})

	t.Run("real durable writer succeeds", func(t *testing.T) {
		w := NewDurableAtomicWriter(filepath.Join(t.TempDir(), "f"), 0o600)

		_, err := w.Write([]byte("x"))
		require.NoError(t, err)
		require.NoError(t, w.Close())
	})
}
