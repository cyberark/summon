package summon

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// helperEnv marks a re-executed test binary as a helper process. Tests set it
// with t.Setenv so that the subprocess Summon launches runs TestHelperProcess
// instead of the test suite. This replaces `bash -c ...` and `false`, which do
// not exist on every platform the tests run on.
const helperEnv = "SUMMON_TEST_HELPER_PROCESS"

// helperArgs returns the argv that re-executes the test binary as a helper in
// the given mode (see TestHelperProcess).
func helperArgs(mode string, args ...string) []string {
	return append([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}, args...)
}

// helperCommand returns an exec.Cmd running a helper in the given mode.
func helperCommand(mode string, args ...string) *exec.Cmd {
	argv := helperArgs(mode, args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	return cmd
}

// TestHelperProcess is not a real test: it is the body of the helper process
// started through helperArgs. Modes:
//
//	writevar  FILE NAME        write the value of env var NAME to FILE
//	writevars FILE NAME...     write "NAME=value|NAME=value" to FILE
//	exit      CODE             exit with CODE
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return // a normal test run
	}

	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}

	switch args[0] {
	case "writevar":
		_ = os.WriteFile(args[1], []byte(os.Getenv(args[2])), 0o600)
	case "writevars":
		pairs := make([]string, 0, len(args)-2)
		for _, name := range args[2:] {
			pairs = append(pairs, name+"="+os.Getenv(name))
		}
		_ = os.WriteFile(args[1], []byte(strings.Join(pairs, "|")), 0o600)
	case "exit":
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)
	}
	os.Exit(0)
}
