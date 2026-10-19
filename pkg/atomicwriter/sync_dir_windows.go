//go:build windows

package atomicwriter

// syncDir is a no-op on Windows: directories cannot be opened and fsynced
// the way they can on Unix.
func syncDir(string) error { return nil }
