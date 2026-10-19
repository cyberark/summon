//go:build solaris

package summon

import (
	"context"
	"errors"

	"github.com/cyberark/summon/pkg/secretsyml"
)

// runRefreshing always fails on Solaris: without syscall.Getpgid a child's
// descendants cannot be tracked, so a restart could leave orphans. The CLI
// rejects --refresh before getting here; this guards library callers.
func runRefreshing(_ context.Context, _ *SubprocessConfig, _ *secretsyml.ParsedConfig, _ *TempFactory) (int, error) {
	return 0, errors.New("refresh mode is not supported on solaris")
}
