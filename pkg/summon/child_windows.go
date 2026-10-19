//go:build windows

package summon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsTree is a Job Object holding the supervised child and its
// descendants, plus the child's console process group for CTRL_BREAK.
type windowsTree struct {
	job         windows.Handle
	groupID     uint32 // the child's PID: CREATE_NEW_PROCESS_GROUP makes it the group ID
	haveConsole bool
}

// jobBasicAccounting mirrors JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, which
// golang.org/x/sys/windows does not define.
type jobBasicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// consoleAttached reports whether Summon has a console. GenerateConsoleCtrlEvent
// only works for processes sharing Summon's console.
func consoleAttached() bool {
	var pids [1]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), 1)
	return n != 0
}

// prepareChild starts the child in a new process group so CTRL_BREAK can be
// sent to it and its descendants without reaching Summon. The child ignores
// Ctrl+C as a side effect; Summon forwards its own interrupts as CTRL_BREAK.
func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// attachTree assigns the started child to a new Job Object. Descendants
// inherit the membership. KILL_ON_JOB_CLOSE makes the tree die with Summon.
func attachTree(cmd *exec.Cmd) (processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure job object: %w", err)
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("open subprocess: %w", err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("assign subprocess to job object: %w", err)
	}

	return &windowsTree{
		job:         job,
		groupID:     uint32(cmd.Process.Pid),
		haveConsole: consoleAttached(),
	}, nil
}

// gracefulStop sends CTRL_BREAK to the child's process group. Without a
// console, or if delivery fails, it kills the tree immediately.
func (t *windowsTree) gracefulStop() error {
	if !t.haveConsole {
		return t.forceKill()
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, t.groupID); err != nil {
		slog.Warn("Could not deliver CTRL_BREAK to subprocess; force-killing", "error", err)
		return t.forceKill()
	}
	return nil
}

func (t *windowsTree) forceKill() error { return windows.TerminateJobObject(t.job, 1) }

func (t *windowsTree) alive() bool {
	var info jobBasicAccounting
	err := windows.QueryInformationJobObject(
		t.job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	)
	return err == nil && info.ActiveProcesses > 0
}

// signal maps interrupt/terminate requests to a graceful stop; other signals
// have no Windows equivalent for a child process and are ignored.
func (t *windowsTree) signal(sig os.Signal) {
	if sig == os.Interrupt || sig == syscall.SIGTERM {
		_ = t.gracefulStop()
	}
}

// close releases the job handle. KILL_ON_JOB_CLOSE kills any process still in
// the job, which stop has already force-killed by this point.
func (t *windowsTree) close() { _ = windows.CloseHandle(t.job) }

// warnIfGracefulStopUnavailable tells the operator when restarts will
// force-kill because Summon has no console to deliver CTRL_BREAK through
// (e.g. it runs as a Windows service).
func warnIfGracefulStopUnavailable() {
	if !consoleAttached() {
		slog.Warn("No console attached: restarts cannot stop the subprocess gracefully and will force-kill it")
	}
}

// ignoredSignal reports no signals on Windows; everything delivered is relayed.
func ignoredSignal(os.Signal) bool { return false }

// stopExitCode maps the exit code of a child that Summon stopped. WaitStatus
// always reports a normal exit on Windows, so the codes our stop produces are
// folded into a clean shutdown: 1 from TerminateJobObject and
// STATUS_CONTROL_C_EXIT (0xC000013A) from an unhandled CTRL_BREAK. Other codes
// are the child's own and kept.
func stopExitCode(code int) int {
	if code == 1 || uint32(code) == 0xC000013A {
		return 0
	}
	return code
}
