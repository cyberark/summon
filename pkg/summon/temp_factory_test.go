package summon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempFactory_PushAndCleanup(t *testing.T) {
	customDir := t.TempDir()

	tests := []struct {
		name   string
		pushes []string
		path   string
	}{
		{"no pushes", nil, ""},
		{"single push", []string{"secret1"}, ""},
		{"multiple pushes", []string{"secret1", "secret2", "secret3"}, ""},
		{"custom path", []string{"content"}, customDir},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tf := NewTempFactory(tc.path)

			var files []string
			for _, content := range tc.pushes {
				path, err := tf.Push(content)
				require.NoError(t, err)

				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, content, string(data))

				if tc.path != "" {
					assert.Contains(t, path, tc.path)
				}

				files = append(files, path)
			}

			tf.Cleanup()

			for _, file := range files {
				_, err := os.Stat(file)
				assert.True(t, os.IsNotExist(err), "Temp file was not removed by Cleanup")
			}

			if tf.path != devSHM {
				_, err := os.Stat(tf.path)
				assert.True(t, os.IsNotExist(err), "Temp directory was not removed by Cleanup")
			}
		})
	}
}

func TestTempFactory_AddFile(t *testing.T) {
	// Create a real temp file to track
	f, err := os.CreateTemp("", "addfile-test")
	require.NoError(t, err)
	f.Close()

	tf := NewTempFactory("")
	tf.AddFile(f.Name())

	// File should still exist before cleanup
	_, err = os.Stat(f.Name())
	assert.NoError(t, err)

	tf.Cleanup()

	// File should be removed after cleanup
	_, err = os.Stat(f.Name())
	assert.True(t, os.IsNotExist(err), "AddFile'd file was not removed by Cleanup")
}

func TestTempFactory_PushError(t *testing.T) {
	tf := NewTempFactory("/nonexistent/dir")

	_, err := tf.Push("value")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "/nonexistent/dir")
}

func TestDefaultTempPath_HomeFallback(t *testing.T) {
	// Override devSHM to a nonexistent path so defaultTempPath falls through
	// to the home-directory fallback, even on Linux where /dev/shm exists.
	original := devSHM
	devSHM = "/nonexistent/shm"
	t.Cleanup(func() { devSHM = original })

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	path := defaultTempPath()
	defer os.Remove(path)

	assert.DirExists(t, path, "defaultTempPath should create a directory")
	assert.Contains(t, path, home, "fallback path should be under the user's home directory")
}
