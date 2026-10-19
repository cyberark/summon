//go:build !windows

// Package fsperm holds filesystem ownership/permission checks shared by the
// daemon-mode hardening code.
package fsperm

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// CheckAncestors walks dir and every parent up to / (dir must already be
// symlink-resolved), refusing any not owned by euid or root, or writable by
// group/other unless sticky and root-owned (like /tmp). Errors read
// "ancestor <path> is ..." so callers can prefix their own context.
func CheckAncestors(dir string, euid int) error {
	for p := dir; ; p = filepath.Dir(p) {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			if int(st.Uid) != euid && st.Uid != 0 {
				return fmt.Errorf("ancestor %q is not owned by the current user or root", p)
			}
			if fi.Mode().Perm()&0o022 != 0 && !(fi.Mode()&fs.ModeSticky != 0 && st.Uid == 0) {
				return fmt.Errorf("ancestor %q is group or world writable (%v)", p, fi.Mode().Perm())
			}
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}
