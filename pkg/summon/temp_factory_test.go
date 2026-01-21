package summon

import (
    "os"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestTempFactoryPushAndCleanup(t *testing.T) {
    tf := NewTempFactory("")

    content := "test secret"
    tempFilePath, err := tf.Push(content)
	require.NoError(t, err)

    data, err := os.ReadFile(tempFilePath)
	require.NoError(t, err)
    assert.Equal(t, content, string(data))

    tf.Cleanup()

    _, err = os.Stat(tempFilePath)
    assert.True(t, os.IsNotExist(err), "Temp file was not removed by Cleanup")

    if tf.path != DEVSHM {
        _, err := os.Stat(tf.path)
        assert.True(t, os.IsNotExist(err), "Temp directory was not removed by Cleanup")
    }
}

func TestTempFactoryMultiplePushesAndCleanup(t *testing.T) {
    tf := NewTempFactory("")
    
	file1, err := tf.Push("secret1")
	require.NoError(t, err)
	file2, err := tf.Push("secret2")
	require.NoError(t, err)
	file3, err := tf.Push("secret3")
	require.NoError(t, err)
	
	files := []string{file1, file2, file3}

    tf.Cleanup()

    for _, file := range files {
        _, err := os.Stat(file)
        assert.True(t, os.IsNotExist(err), "Temp file was not removed by Cleanup")
    }

    if tf.path != DEVSHM {
        _, err := os.Stat(tf.path)
        assert.True(t, os.IsNotExist(err), "Temp directory was not removed by Cleanup")
    }
}

func TestTempFactoryCleanupWithNoFiles(t *testing.T) {
    tf := NewTempFactory("")

    tf.Cleanup()

    if tf.path != DEVSHM {
        _, err := os.Stat(tf.path)
        assert.True(t, os.IsNotExist(err), "Temp directory was not removed by Cleanup when no files were created")
    }
}