package pushtofile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func changedTestFile(path string, overwrite bool) SecretFile {
	return SecretFile{
		FileConfig: secretsyml.FileConfig{
			Path:        path,
			Format:      "yaml",
			Permissions: 0o600,
			Secrets:     goodSecretSpecs(),
			Overwrite:   overwrite,
		},
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestWriteIfChanged(t *testing.T) {
	t.Run("first write creates file and returns checksum of its content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secrets.yml")
		file := changedTestFile(path, false)

		gotPath, sum, written, err := file.WriteIfChanged(
			createResults(map[string]string{"alias1": "v1", "alias2": "v2"}), "")

		require.NoError(t, err)
		assert.True(t, written)
		assert.Equal(t, path, gotPath)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, sha256Hex(content), sum)
		assert.Len(t, sum, 64)
	})

	t.Run("unchanged content is not rewritten", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secrets.yml")
		file := changedTestFile(path, false)
		results := createResults(map[string]string{"alias1": "v1", "alias2": "v2"})
		_, sum, _, err := file.WriteIfChanged(results, "")
		require.NoError(t, err)

		// Tamper with the file: a rewrite would restore the original content.
		require.NoError(t, os.WriteFile(path, []byte("tampered"), 0o600))

		_, sum2, written, err := file.WriteIfChanged(results, sum)

		require.NoError(t, err)
		assert.False(t, written)
		assert.Equal(t, sum, sum2)
		content, _ := os.ReadFile(path)
		assert.Equal(t, "tampered", string(content))
	})

	t.Run("changed content is rewritten even when overwrite is false", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secrets.yml")
		file := changedTestFile(path, false)
		_, sum, _, err := file.WriteIfChanged(
			createResults(map[string]string{"alias1": "v1", "alias2": "v2"}), "")
		require.NoError(t, err)

		_, sum2, written, err := file.WriteIfChanged(
			createResults(map[string]string{"alias1": "rotated", "alias2": "v2"}), sum)

		require.NoError(t, err)
		assert.True(t, written)
		assert.NotEqual(t, sum, sum2)
		content, _ := os.ReadFile(path)
		assert.Contains(t, string(content), "rotated")
		assert.Equal(t, sha256Hex(content), sum2)
	})

	t.Run("first write respects overwrite=false on an existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secrets.yml")
		require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))
		file := changedTestFile(path, false)

		_, _, written, err := file.WriteIfChanged(
			createResults(map[string]string{"alias1": "v1", "alias2": "v2"}), "")

		require.Error(t, err)
		assert.False(t, written)
		assert.Contains(t, err.Error(), "overwrite is not enabled")
	})
}

type failingCloser struct{ closeErr error }

func (f failingCloser) Write(p []byte) (int, error) { return len(p), nil }
func (f failingCloser) Close() error                { return f.closeErr }

func TestWriteIfChangedCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yml")
	file := changedTestFile(path, false)
	open := func(string, os.FileMode, bool) (io.WriteCloser, error) {
		return failingCloser{closeErr: errors.New("rename failed")}, nil
	}

	_, sum, written, err := file.writeIfChanged(
		createResults(map[string]string{"alias1": "v1", "alias2": "v2"}), "", open)

	require.Error(t, err)
	assert.Empty(t, sum)
	assert.False(t, written)
	assert.NotContains(t, err.Error(), "v1")
}

type failingWriter struct{ writeErr error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.writeErr }
func (f failingWriter) Close() error              { return nil }

func TestWriteIfChangedKeepsWriterErrorCause(t *testing.T) {
	file := changedTestFile(filepath.Join(t.TempDir(), "secrets.yml"), false)
	results := createResults(map[string]string{"alias1": "v1", "alias2": "v2"})

	t.Run("write error", func(t *testing.T) {
		open := func(string, os.FileMode, bool) (io.WriteCloser, error) {
			return failingWriter{writeErr: errors.New("no space left on device")}, nil
		}
		_, _, _, err := file.writeIfChanged(results, "", open)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no space left on device")
		assert.NotContains(t, err.Error(), "v1")
	})

	t.Run("close error", func(t *testing.T) {
		open := func(string, os.FileMode, bool) (io.WriteCloser, error) {
			return failingCloser{closeErr: errors.New("rename failed")}, nil
		}
		_, _, _, err := file.writeIfChanged(results, "", open)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rename failed")
	})
}

func TestPushPlanKeepsCloseErrorCause(t *testing.T) {
	file := changedTestFile(filepath.Join(t.TempDir(), "secrets.yml"), false)
	plan, err := file.plan(createResults(map[string]string{"alias1": "v1", "alias2": "v2"}))
	require.NoError(t, err)
	open := func(string, os.FileMode, bool) (io.WriteCloser, error) {
		return failingCloser{closeErr: errors.New("rename failed")}, nil
	}

	err = file.pushPlan(plan, open, pushToWriter, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename failed")
	assert.NotContains(t, err.Error(), "%!w")
}
