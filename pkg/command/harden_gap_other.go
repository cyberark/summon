//go:build !linux && !windows

package command

// platformHardeningGap: core dumps are suppressed via RLIMIT_CORE and provider
// and destination checks run, but the process cannot be marked non-dumpable.
func platformHardeningGap() string {
	return "--refresh: process is not marked non-dumpable on this platform, so same-user processes may read credentials from the environment"
}
