//go:build windows

package summon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var ctrlCExit = uint32(0xC000013A)

func TestStopExitCodeMapsWindowsStopCodes(t *testing.T) {
	assert.Equal(t, 0, stopExitCode(1), "TerminateJobObject")
	assert.Equal(t, 0, stopExitCode(int(ctrlCExit)), "STATUS_CONTROL_C_EXIT")
	assert.Equal(t, 0, stopExitCode(0))
	assert.Equal(t, 3, stopExitCode(3), "a deliberate exit code is kept")
}
