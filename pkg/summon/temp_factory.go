package summon

import (
	"os"
	"strings"
)

// devSHM is the default *nix shared-memory directory path
var devSHM = "/dev/shm"

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
		path = defaultTempPath()
	}
	return TempFactory{path: path}
}

// defaultTempPath returns the best possible temp folder path for temp files
func defaultTempPath() string {
	fi, err := os.Stat(devSHM)
	if err == nil && fi.Mode().IsDir() {
		return devSHM
	}
	home, err := os.UserHomeDir()
	if err == nil {
		dir, err := os.MkdirTemp(home, ".tmp")
		if err == nil {
			return dir
		}
	}
	return os.TempDir()
}

// AddFile adds an existing file to the factory's list of files to clean up.
func (tf *TempFactory) AddFile(path string) {
	tf.files = append(tf.files, path)
}

// Push creates a temp file with given value. Returns the path.
func (tf *TempFactory) Push(value string) (string, error) {
	f, err := os.CreateTemp(tf.path, ".summon")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write([]byte(value)); err != nil {
		return "", err
	}
	name := f.Name()
	tf.files = append(tf.files, name)
	return name, nil
}

// Cleanup removes the temporary files created with this factory.
func (tf *TempFactory) Cleanup() {
	for _, file := range tf.files {
		_ = os.Remove(file) // Best-effort cleanup
	}
	// Also remove the tempdir if it's not devSHM
	if !strings.Contains(tf.path, devSHM) {
		_ = os.Remove(tf.path) // Best-effort cleanup
	}
	tf.files = []string{}
	tf.path = ""
}
