//go:build windows

package command

// platformHardeningGap lists the protections --refresh lacks on Windows.
func platformHardeningGap() string {
	return "--refresh on Windows runs without core-dump suppression, provider binary verification and secret-file directory checks"
}
