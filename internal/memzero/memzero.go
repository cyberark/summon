// Package memzero zeroes sensitive byte slices in a way the compiler cannot
// optimize away as a dead store.
package memzero

import "runtime"

// Clear zeroes b and keeps it live until the zeroing is done.
func Clear(b []byte) {
	clear(b)
	runtime.KeepAlive(b)
}
