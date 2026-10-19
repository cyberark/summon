package atomicwriter

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// OS Function table
type osFuncs struct {
	chmod    func(string, os.FileMode) error
	rename   func(string, string) error
	remove   func(string) error
	sync     func(*os.File) error
	syncDir  func(string) error
	tempFile func(string, string) (*os.File, error)
	truncate func(string, int64) error
	write    func(*os.File, []byte) (int, error)
}

// Instantiation of OS Function table using std OS
var stdOSFuncs = osFuncs{
	chmod:  os.Chmod,
	rename: os.Rename,
	remove: os.Remove,
	sync: func(file *os.File) error {
		return file.Sync()
	},
	syncDir:  syncDir,
	tempFile: os.CreateTemp,
	truncate: os.Truncate,
	write: func(file *os.File, content []byte) (int, error) {
		return file.Write(content)
	},
}

type atomicWriter struct {
	path        string
	permissions os.FileMode
	// syncDir makes Close fsync the parent directory after the rename so the
	// new directory entry survives a crash.
	syncDir  bool
	tempFile *os.File
	os       osFuncs
	logger   *slog.Logger
}

// NewAtomicWriter provides a simple atomic file writer which implements the
// io.WriteCloser interface. This allows us to use the atomic writer the way we
// would use any other Writer, such as a Buffer. Additonally, this struct
// takes the file path during construction, so the code which calls
// `Write()` doesn't need to be concerned with the destination, just like
// any other writer.
func NewAtomicWriter(path string, permissions os.FileMode) io.WriteCloser {
	return newAtomicWriter(path, permissions, stdOSFuncs)
}

// NewDurableAtomicWriter is NewAtomicWriter that additionally fsyncs the
// destination directory after the rename. Long-running callers that rewrite
// the same file repeatedly use it so a crash cannot lose the update.
func NewDurableAtomicWriter(path string, permissions os.FileMode) io.WriteCloser {
	w := newAtomicWriter(path, permissions, stdOSFuncs)
	w.(*atomicWriter).syncDir = true
	return w
}

func newAtomicWriter(path string, permissions os.FileMode, osFuncs osFuncs) io.WriteCloser {
	return &atomicWriter{
		path:        path,
		tempFile:    nil,
		permissions: permissions,
		os:          osFuncs,
		logger:      slog.Default(),
	}
}

func (w *atomicWriter) Write(content []byte) (n int, err error) {
	// Create a temporary file if not created already
	if w.tempFile == nil {
		dir, file := filepath.Split(w.path)

		f, err := w.os.tempFile(dir, file)
		if err != nil {
			w.logger.Error("Could not create temporary file", "path", w.path, "error", err)
			return 0, err
		}
		w.logger.Debug("Writing temporary secrets file", "tempFile", f.Name())
		w.tempFile = f
	}

	// Write to the temporary file
	n, err = w.os.write(w.tempFile, content)
	if err != nil {
		w.logger.Error("Could not write content to temporary file", "path", w.path)
		w.Cleanup()
	}
	return n, err
}

func (w *atomicWriter) Close() error {
	if w.tempFile == nil {
		return nil
	}
	defer w.Cleanup()

	// Flush and close the temporary file
	err := w.os.sync(w.tempFile)
	if err != nil {
		w.logger.Error("Could not flush temporary file", "tempFile", w.tempFile.Name())
		return err
	}
	w.tempFile.Close()

	// Set the file permissions
	err = w.os.chmod(w.tempFile.Name(), w.permissions)
	if err != nil {
		w.logger.Error("Could not set permissions on temporary file", "tempFile", w.tempFile.Name())
		// Try to rename the file anyway
	}

	// Rename the temporary file to the destination
	w.logger.Debug("Moving temporary file", "dest", w.path)
	err = w.os.rename(w.tempFile.Name(), w.path)
	if err != nil {
		w.logger.Error("Could not move temporary file", "dest", w.path, "error", err)
		return err
	}
	w.tempFile = nil

	if w.syncDir {
		if err = w.os.syncDir(filepath.Dir(w.path)); err != nil {
			w.logger.Error("Could not flush destination directory", "dest", w.path, "error", err)
			return err
		}
	}

	return nil
}

// Cleanup attempts to remove the temporary file. This function is called by
// the `Close()` method, but can also be called manually in cases where `Close()`
// is not called.
func (w *atomicWriter) Cleanup() {
	if w.tempFile == nil {
		return
	}

	// Windows cannot remove a file that is still open. Closing an already
	// closed file is harmless (its error is ignored).
	_ = w.tempFile.Close()

	err := w.os.remove(w.tempFile.Name())
	if err == nil || os.IsNotExist(err) {
		w.tempFile = nil
		return
	}

	// If we can't remove the temporary directory, truncate the file to remove all secret content
	truncErr := w.os.truncate(w.tempFile.Name(), 0)
	switch {
	case os.IsNotExist(truncErr):
		// This shouldn't happen, but just to be safe.
		w.tempFile = nil
	case truncErr != nil:
		// Truncate failed as well
		w.logger.Error("Could not delete temporary file. File may be left on disk.", "tempFile", w.tempFile.Name(), "error", truncErr)
	default:
		w.logger.Error("Could not delete temporary file. Truncated file.", "tempFile", w.tempFile.Name(), "error", err)
	}
}
