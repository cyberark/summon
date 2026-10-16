//go:build !windows

package atomicwriter

import "os"

// syncDir fsyncs a directory so a rename into it is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
