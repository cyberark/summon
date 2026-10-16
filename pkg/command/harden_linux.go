//go:build linux

package command

import "golang.org/x/sys/unix"

// defaultApplyNotDumpable sets PR_SET_DUMPABLE to 0.
func defaultApplyNotDumpable() error {
	return unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}

// platformHardeningGap: Linux has every protection, so there is nothing to say.
func platformHardeningGap() string { return "" }
