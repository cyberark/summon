//go:build !windows

package pushtofile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cyberark/summon/internal/fsperm"
)

// checkDaemonDestination refuses a destination another local user could
// redirect or tamper with between rotations: a symlinked final component, or a
// directory (or any ancestor, after resolving symlinks) that is not owned by
// the current user or root, or is group/world-writable. The atomic writer
// renames by path, so these checks are repeated before every rewrite.
func checkDaemonDestination(path string) error {
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() && st.Uid != 0 {
		return fmt.Errorf("refusing to write secret file: directory %q is not owned by the current user", dir)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("refusing to write secret file: directory %q is group or world writable (%v)", dir, fi.Mode().Perm())
	}
	if err := fsperm.CheckAncestors(filepath.Dir(dir), os.Geteuid()); err != nil {
		return fmt.Errorf("refusing to write secret file: %w", err)
	}

	if li, err := os.Lstat(path); err == nil && li.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write secret file: %q is a symlink", path)
	}
	return nil
}
