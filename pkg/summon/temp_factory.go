package summon

import (
	"os"
	"strings"
)

// DEVSHM is the default *nix shared-memory directory path
const DEVSHM = "/dev/shm"

// TempFactory handels transient files that require cleaning up
// after the child process exits.
type TempFactory struct {
	path  string
	files []string
}

// NewTempFactory creates a new temporary file factory.
// defer Cleanup() if you want the files removed.
func NewTempFactory(path string) TempFactory {
	if path == "" {
		path = DefaultTempPath()
	}
	return TempFactory{path: path}
}

// DefaultTempPath returns the best possible temp folder path for temp files
func DefaultTempPath() string {
	fi, err := os.Stat(DEVSHM)
	if err == nil && fi.Mode().IsDir() {
		return DEVSHM
	}
	home, err := os.UserHomeDir()
	if err == nil {
		dir, _ := os.MkdirTemp(home, ".tmp")
		return dir
	}
	return os.TempDir()
}

// Push creates a temp file with given value. Returns the path.
func (tf *TempFactory) Push(value string) string {
	f, _ := os.CreateTemp(tf.path, ".summon")
	defer f.Close()

	f.Write([]byte(value))
	name := f.Name()
	tf.files = append(tf.files, name)
	return name
}

// Cleanup removes the temporary files created with this factory.
func (tf *TempFactory) Cleanup() {
	for _, file := range tf.files {
		os.Remove(file)
	}
	// Also remove the tempdir if it's not DEVSHM
	if !strings.Contains(tf.path, DEVSHM) {
		os.Remove(tf.path)
	}
	tf = nil
}
