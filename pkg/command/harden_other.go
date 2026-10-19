//go:build !linux

package command

// defaultApplyNotDumpable cannot mark the process non-dumpable outside Linux
// (PR_SET_DUMPABLE is Linux-only). The gap is reported by platformHardeningGap.
func defaultApplyNotDumpable() error { return nil }
