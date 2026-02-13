package atomicwriter

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// osFuncCounts struct is used to track the number of calls that were
// made to each OS function during an individual atomic writer test case.
type osFuncCounts struct {
	chmodCount    int
	renameCount   int
	removeCount   int
	syncCount     int
	tempFileCount int
	truncateCount int
	writeCount    int
}

// injectErrors define errors which should be returned (i.e. simulated) for
// OS functions for a given atomic writer test case.
type injectErrors struct {
	chmodErr    error
	renameErr   error
	removeErr   error
	syncErr     error
	tempFileErr error
	truncateErr error
	writeErr    error
}

// newTestOSFuncs creates an osFuncs table of OS functions, each of which
// wraps the equivalent standard OS function, and adds the following
// functionality for testing:
//
//   - Track the number of times that the function was called
//   - Optionally return an error for testing purposes
func newTestOSFuncs(injectErrs injectErrors) (osFuncs, *osFuncCounts) {
	counts := &osFuncCounts{}
	funcs := osFuncs{
		chmod: func(filename string, mode os.FileMode) error {
			counts.chmodCount++
			if injectErrs.chmodErr != nil {
				return injectErrs.chmodErr
			}
			return stdOSFuncs.chmod(filename, mode)
		},
		rename: func(oldName, newName string) error {
			counts.renameCount++
			if injectErrs.renameErr != nil {
				return injectErrs.renameErr
			}
			return stdOSFuncs.rename(oldName, newName)
		},
		remove: func(filename string) error {
			counts.removeCount++
			if injectErrs.removeErr != nil {
				return injectErrs.removeErr
			}
			return stdOSFuncs.remove(filename)
		},
		sync: func(file *os.File) error {
			counts.syncCount++
			if injectErrs.syncErr != nil {
				return injectErrs.syncErr
			}
			return stdOSFuncs.sync(file)
		},
		tempFile: func(dir, file string) (*os.File, error) {
			counts.tempFileCount++
			if injectErrs.tempFileErr != nil {
				return nil, injectErrs.tempFileErr
			}
			return stdOSFuncs.tempFile(dir, file)
		},
		truncate: func(filename string, size int64) error {
			counts.truncateCount++
			if injectErrs.truncateErr != nil {
				return injectErrs.truncateErr
			}
			return stdOSFuncs.truncate(filename, size)
		},
		write: func(file *os.File, content []byte) (int, error) {
			counts.writeCount++
			if injectErrs.writeErr != nil {
				return 0, injectErrs.writeErr
			}
			return stdOSFuncs.write(file, content)
		},
	}
	return funcs, counts
}

type assertFunc func(t *testing.T, path string, tempFilePath string,
	counts *osFuncCounts, err error)
type errorAssertFunc func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
	tempFileName string, err error)

func TestWriteAndClose(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		permissions os.FileMode
		content     string
		skipWrite   bool
		assert      assertFunc
	}{
		{
			name:        "happy path",
			path:        "test_file.txt",
			permissions: 0644,
			content:     "test content",
			assert: func(t *testing.T, path string, tempFilePath string, counts *osFuncCounts, err error) {
				assert.NoError(t, err)
				// Check that the file exists
				assert.FileExists(t, path)
				// Check the contents of the file
				contents, err := os.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, "test content", string(contents))
				// Check the file permissions
				mode, err := os.Stat(path)
				assert.NoError(t, err)
				assert.Equal(t, os.FileMode(0644), mode.Mode())
				// Check that the temp file was deleted
				assert.NoFileExists(t, tempFilePath)
				// Check that OS functions were called as expected
				assert.Equal(t, counts.tempFileCount, 1)
				assert.Equal(t, counts.writeCount, 1)
				assert.Equal(t, counts.syncCount, 1)
				assert.Equal(t, counts.chmodCount, 1)
				assert.Equal(t, counts.renameCount, 1)
				assert.Equal(t, counts.removeCount, 0)
				assert.Equal(t, counts.truncateCount, 0)
			},
		},
		{
			name:        "close without a write",
			path:        "test_file.txt",
			permissions: 0644,
			content:     "test content",
			skipWrite:   true,
			assert: func(t *testing.T, path string, tempFilePath string, counts *osFuncCounts, err error) {
				assert.NoError(t, err)
				// Check that the file does not exist
				assert.NoFileExists(t, path)
				// Check that no OS functions were called
				assert.Equal(t, counts.tempFileCount, 0)
				assert.Equal(t, counts.writeCount, 0)
				assert.Equal(t, counts.syncCount, 0)
				assert.Equal(t, counts.chmodCount, 0)
				assert.Equal(t, counts.renameCount, 0)
				assert.Equal(t, counts.removeCount, 0)
				assert.Equal(t, counts.truncateCount, 0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var tempFilePath string

			// Create a temp file for writing secret files
			tmpDir := t.TempDir()

			// Create a test atomic writer
			path := filepath.Join(tmpDir, tc.path)
			writer, funcCounts, _ := newTestWriter(path, tc.permissions, injectErrors{})

			// Write the temp file and record its path
			if tc.skipWrite != true {
				_, err := writer.Write([]byte(tc.content))
				assert.NoError(t, err)
				tempFilePath = writer.(*atomicWriter).tempFile.Name()
			}

			// Rename/close the file
			err := writer.Close()
			assert.NoError(t, err)

			tc.assert(t, path, tempFilePath, funcCounts, err)
		})
	}
}

func TestWriterAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_file.txt")
	initialContent := "initial content"

	// First create the file with some test content
	os.WriteFile(path, []byte(initialContent), 0644)

	// Create 2 writers for the same path
	writer1 := NewAtomicWriter(path, 0600)
	writer2 := NewAtomicWriter(path, 0644)

	// Write different content to each writer
	writer1.Write([]byte("writer 1 line 1\n"))
	writer2.Write([]byte("writer 2 line 1\n"))
	writer1.Write([]byte("writer 1 line 2\n"))
	writer2.Write([]byte("writer 2 line 2\n"))

	// Ensure the destination file hasn't changed
	contents, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, initialContent, string(contents))

	// Close the first writer and ensure only the contents of the first writer are written
	err = writer1.Close()

	assert.NoError(t, err)
	// Check that the file exists
	assert.FileExists(t, path)
	// Check the contents of the file match the first writer (which was closed)
	contents, err = os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "writer 1 line 1\nwriter 1 line 2\n", string(contents))
	// Check the file permissions match the first writer
	mode, err := os.Stat(path)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), mode.Mode())
}

func TestLogsErrors(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		injectErrs   injectErrors
		errorOnWrite bool
		assert       errorAssertFunc
	}{
		{
			name:         "unable to create temporary file",
			path:         "test_file.txt",
			injectErrs:   injectErrors{tempFileErr: os.ErrPermission},
			errorOnWrite: true,
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)
				assert.Contains(t, buf.String(), "Could not create temporary file")
			},
		},
		{
			name:         "unable to write temporary file",
			path:         "test_file.txt",
			injectErrs:   injectErrors{writeErr: os.ErrInvalid},
			errorOnWrite: true,
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)
				assert.Contains(t, buf.String(), "Could not write content to temporary file")
			},
		},
		{
			name: "unable to remove temporary file",
			path: "test_file.txt",
			injectErrs: injectErrors{
				renameErr: os.ErrPermission,
				removeErr: os.ErrPermission,
			},
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)

				// The file should be truncated instead of being deleted
				assert.Contains(t, buf.String(), "Could not delete temporary file")
				assert.Contains(t, buf.String(), "Truncated file")

				// Check that the temp file was truncated
				assert.FileExists(t, tempFileName)
				content, err := os.ReadFile(tempFileName)
				assert.NoError(t, err)
				assert.Equal(t, "", string(content))
			},
		},
		{
			name: "unable to remove or truncate temporary file",
			path: "test_file.txt",
			injectErrs: injectErrors{
				renameErr:   os.ErrPermission,
				removeErr:   os.ErrPermission,
				truncateErr: syscall.EINVAL,
			},
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)

				assert.Contains(t, buf.String(), "Could not delete temporary file")
				assert.Contains(t, buf.String(), "File may be left on disk")

				assert.FileExists(t, tempFileName)
			},
		},
		{
			name: "unable to remove temp file, truncate returns ErrNotExist",
			path: "test_file.txt",
			injectErrs: injectErrors{
				renameErr:   os.ErrPermission,
				removeErr:   os.ErrPermission,
				truncateErr: os.ErrNotExist,
			},
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)

				// Check that the writer's temp file pointer has been cleared
				writer, ok := wc.(*atomicWriter)
				assert.True(t, ok)
				assert.Nil(t, writer.tempFile)
			},
		},
		{
			name:       "unable to chmod",
			path:       "test_file.txt",
			injectErrs: injectErrors{chmodErr: os.ErrPermission},
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.NoError(t, err)
				assert.Contains(t, buf.String(), "Could not set permissions on temporary file")

				// Check that the file was still renamed
				writer, ok := wc.(*atomicWriter)
				assert.True(t, ok)
				assert.FileExists(t, writer.path)
				assert.NoFileExists(t, tempFileName)
			},
		},
		{
			name:       "unable to sync",
			path:       "test_file.txt",
			injectErrs: injectErrors{syncErr: os.ErrInvalid},
			assert: func(t *testing.T, buf *bytes.Buffer, wc io.WriteCloser,
				tempFileName string, err error) {
				assert.Error(t, err)
				assert.Contains(t, buf.String(), "Could not flush temporary file")
				assert.NoFileExists(t, tempFileName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, tc.path)

			// Create a test atomic writer
			writer, _, logBuf := newTestWriter(path, 0644, tc.injectErrs)

			// Try to write the file
			_, err := writer.Write([]byte("test content"))

			if tc.errorOnWrite {
				assert.Error(t, err)
				tc.assert(t, logBuf, writer, "", err)
				err = writer.Close()
				assert.NoError(t, err)
				return
			}

			assert.NoError(t, err)
			atomicWriter, ok := writer.(*atomicWriter)
			assert.True(t, ok)
			tempFileName := atomicWriter.tempFile.Name()
			err = writer.Close()
			tc.assert(t, logBuf, writer, tempFileName, err)
		})
	}
}

func TestDefaultDirectory(t *testing.T) {
	writer := NewAtomicWriter("test_file.txt", 0644)

	_, err := writer.Write([]byte("test content"))
	assert.NoError(t, err)

	err = writer.Close()
	defer os.Remove("test_file.txt")
	assert.NoError(t, err)
	assert.FileExists(t, "./test_file.txt")
}

func newTestWriter(path string, permissions os.FileMode,
	injectErrs injectErrors) (io.WriteCloser, *osFuncCounts, *bytes.Buffer) {

	writer := NewAtomicWriter(path, permissions)
	funcs, counts := newTestOSFuncs(injectErrs)
	logBuf := bytes.Buffer{}
	writer.(*atomicWriter).os = funcs
	writer.(*atomicWriter).logger = slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return writer, counts, &logBuf
}
