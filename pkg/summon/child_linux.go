//go:build linux

package summon

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
)

// groupAlive reports whether process group pgid has a member that is still
// running. A zombie (or dead) member keeps kill(-pgid, 0) succeeding until its
// parent reaps it, which never happens when the zombie was re-parented to a
// PID 1 that does not reap orphans (common in containers). Those must not
// count, or stop would wait out the whole grace period for a tree that is
// already gone.
func groupAlive(pgid int) bool {
	if !signalProbe(pgid) {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return true // no /proc: trust the signal probe
	}
	sawZombie := false
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue // the process vanished while scanning
		}
		state, pg, ok := parseProcStat(b)
		if !ok || pg != pgid {
			continue
		}
		if state != 'Z' && state != 'X' {
			return true
		}
		sawZombie = true
	}
	// The probe saw a member. Declare the group drained only if /proc showed
	// it to consist solely of zombies; if /proc showed nothing (hidepid, a
	// process vanishing mid-scan), trust the probe.
	return !sawZombie
}

// parseProcStat extracts the state letter and process group from the contents
// of /proc/<pid>/stat: "pid (comm) state ppid pgrp ...". comm is arbitrary
// (spaces, parentheses), so fields are counted from the last ')'.
func parseProcStat(b []byte) (state byte, pgrp int, ok bool) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, 0, false
	}
	f := bytes.Fields(b[i+1:])
	if len(f) < 3 || len(f[0]) != 1 {
		return 0, 0, false
	}
	pgrp, err := strconv.Atoi(string(f[2]))
	if err != nil {
		return 0, 0, false
	}
	return f[0][0], pgrp, true
}
