//go:build windows

package pushtofile

// checkDaemonDestination is a no-op on Windows (ownership and mode bits are
// not meaningful there); --refresh reports the gap once at startup.
func checkDaemonDestination(string) error { return nil }
