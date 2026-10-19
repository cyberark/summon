//go:build !windows

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProvider creates an executable script in a fresh 0755 directory.
func writeProvider(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o755))
	path := filepath.Join(dir, "prov")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return path
}

func TestResolveDaemon_RefusesWritableBinaryOrDir(t *testing.T) {
	t.Run("group-writable binary", func(t *testing.T) {
		path := writeProvider(t, "echo x")
		require.NoError(t, os.Chmod(path, 0o775))

		_, err := ResolveDaemon(path, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})

	t.Run("world-writable directory", func(t *testing.T) {
		path := writeProvider(t, "echo x")
		require.NoError(t, os.Chmod(filepath.Dir(path), 0o777))

		_, err := ResolveDaemon(path, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})

	t.Run("one-shot Resolve is unchanged", func(t *testing.T) {
		path := writeProvider(t, "echo x")
		require.NoError(t, os.Chmod(path, 0o777))

		p, err := Resolve(path)

		require.NoError(t, err)
		assert.Equal(t, path, p.Path())
	})
}

func TestExternalProvider_ReverifiesBeforeEachExec(t *testing.T) {
	path := writeProvider(t, "printf secret")
	p, err := ResolveDaemon(path, 0)
	require.NoError(t, err)

	v, err := p.FetchSecret(context.Background(), "any")
	require.NoError(t, err)
	assert.Equal(t, "secret", string(v))

	t.Run("replaced binary (new inode) is refused", func(t *testing.T) {
		replacement := filepath.Join(filepath.Dir(path), "new")
		require.NoError(t, os.WriteFile(replacement, []byte("#!/bin/sh\nprintf evil\n"), 0o755))
		require.NoError(t, os.Rename(replacement, path))

		_, err := p.FetchSecret(context.Background(), "any")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "changed")
	})
}

func TestExternalProvider_RefusesChmodAfterResolve(t *testing.T) {
	path := writeProvider(t, "printf secret")
	p, err := ResolveDaemon(path, 0)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(path, 0o757))

	_, err = p.FetchSecret(context.Background(), "any")
	require.Error(t, err)
}

func TestExternalProvider_RefusesModifiedBinary(t *testing.T) {
	path := writeProvider(t, "printf secret")
	p, err := ResolveDaemon(path, 0)
	require.NoError(t, err)

	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))

	v, ok := p.(interface{ Verify() error })
	require.True(t, ok)
	assert.Error(t, v.Verify())
}

func TestCall_ErrorSeparatesStderr(t *testing.T) {
	path := writeProvider(t, "echo leaked-stderr-token >&2; exit 3")

	_, err := Call(context.Background(), path, "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "leaked-stderr-token", "full message is unchanged")
	var ce *CallError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "leaked-stderr-token", ce.Stderr)
	assert.NotContains(t, ce.Redacted(), "leaked-stderr-token")
	assert.Contains(t, ce.Redacted(), "exit status 3")
}

func TestResolveDaemon_RefusesUnsafeAncestor(t *testing.T) {
	t.Run("group-writable ancestor", func(t *testing.T) {
		parent := t.TempDir()
		sub := filepath.Join(parent, "bin")
		require.NoError(t, os.Mkdir(sub, 0o755))
		path := filepath.Join(sub, "prov")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho x\n"), 0o755))
		require.NoError(t, os.Chmod(parent, 0o775))

		_, err := ResolveDaemon(path, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable")
	})

}

func TestResolveDaemon_ExecsResolvedPath(t *testing.T) {
	real := writeProvider(t, "printf secret")
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	want, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)

	p, err := ResolveDaemon(link, 0)

	require.NoError(t, err)
	assert.Equal(t, want, p.Path())
}
