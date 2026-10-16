//go:build !windows

package pushtofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func daemonTestFile(path string) SecretFile {
	f := changedTestFile(path, true)
	f.Daemon = true
	return f
}

func TestWriteIfChanged_DaemonDestinationChecks(t *testing.T) {
	results := createResults(map[string]string{"alias1": "v1", "alias2": "v2"})

	t.Run("writes into an owned private directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secrets.yml")
		file := daemonTestFile(path)

		_, _, written, err := file.WriteIfChanged(results, "")

		require.NoError(t, err)
		assert.True(t, written)
	})

	for name, mode := range map[string]os.FileMode{"group-writable": 0o770, "world-writable": 0o777} {
		t.Run("refuses a "+name+" directory", func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Chmod(dir, mode))
			path := filepath.Join(dir, "secrets.yml")
			file := daemonTestFile(path)

			_, _, written, err := file.WriteIfChanged(results, "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "writable")
			assert.False(t, written)
			_, statErr := os.Lstat(path)
			assert.True(t, os.IsNotExist(statErr))
		})
	}

	t.Run("refuses a symlinked destination", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.WriteFile(target, []byte("orig"), 0o600))
		path := filepath.Join(dir, "secrets.yml")
		require.NoError(t, os.Symlink(target, path))
		file := daemonTestFile(path)

		_, _, _, err := file.WriteIfChanged(results, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink")
		content, _ := os.ReadFile(target)
		assert.Equal(t, "orig", string(content))
	})

	t.Run("one-shot mode is unchanged", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0o777))
		file := changedTestFile(filepath.Join(dir, "secrets.yml"), true)

		_, _, written, err := file.WriteIfChanged(results, "")

		require.NoError(t, err)
		assert.True(t, written)
	})
}
