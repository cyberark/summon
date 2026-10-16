//go:build windows

package provider

// fileIdentity is unused on Windows: file ownership and mode bits are not
// meaningful there, so daemon-mode provider pinning is not implemented.
type fileIdentity struct{}

func captureIdentity(_ string) (*fileIdentity, error) {
	return nil, nil
}

func verifyIdentity(_ string, _ *fileIdentity) error { return nil }

func (f *fileIdentity) execPath(path string) string { return path }
