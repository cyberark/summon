//go:build !windows

package pushtofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDaemonDestination(t *testing.T) {
	t.Run("private directory is accepted", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0o700))

		require.NoError(t, checkDaemonDestination(filepath.Join(dir, "s.yml")))
	})

	t.Run("group-writable ancestor is refused", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "secrets")
		require.NoError(t, os.Mkdir(dir, 0o700))
		require.NoError(t, os.Chmod(parent, 0o775))

		err := checkDaemonDestination(filepath.Join(dir, "s.yml"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})

	t.Run("symlinked directory to a writable target is refused", func(t *testing.T) {
		target := t.TempDir()
		require.NoError(t, os.Chmod(target, 0o777))
		link := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(target, link))

		err := checkDaemonDestination(filepath.Join(link, "s.yml"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})

	t.Run("symlinked final component is refused", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0o700))
		path := filepath.Join(dir, "s.yml")
		require.NoError(t, os.Symlink(filepath.Join(dir, "other"), path))

		err := checkDaemonDestination(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink")
	})
}
