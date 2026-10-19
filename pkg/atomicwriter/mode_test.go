package atomicwriter

import (
	"os"
	"runtime"
)

// expectedMode returns the mode os.Stat reports for a file created with perm.
// Windows has no POSIX permission bits: files are reported as 0666, or 0444
// when they are read-only.
func expectedMode(perm os.FileMode) os.FileMode {
	if runtime.GOOS != "windows" {
		return perm
	}
	if perm&0o200 != 0 {
		return 0o666
	}
	return 0o444
}
