//go:build !windows

package provider

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cyberark/summon/internal/fsperm"
)

// statIdentity is the part of a file's metadata that must not change while a
// daemon keeps exec'ing it.
type statIdentity struct {
	path  string
	dev   uint64
	ino   uint64
	uid   uint32
	mode  fs.FileMode
	mtime time.Time
}

// fileIdentity pins the resolved provider binary and its directory.
type fileIdentity struct {
	bin, dir statIdentity
}

func readIdentity(path string) (statIdentity, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return statIdentity{}, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return statIdentity{}, fmt.Errorf("cannot stat %s", path)
	}
	return statIdentity{
		path:  path,
		dev:   uint64(st.Dev), //nolint:unconvert // width differs per OS
		ino:   uint64(st.Ino), //nolint:unconvert
		uid:   st.Uid,
		mode:  fi.Mode(),
		mtime: fi.ModTime(),
	}, nil
}

// readIdentityChecked reads the identity of path (symlinks resolved, as exec
// would) and of its parent directory, refusing group/world-writable ones.
func readIdentityChecked(path string) (*fileIdentity, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	id := &fileIdentity{}
	if id.bin, err = readIdentity(real); err != nil {
		return nil, err
	}
	if id.dir, err = readIdentity(filepath.Dir(real)); err != nil {
		return nil, err
	}
	// A directory's mtime changes whenever anything is created in it; only
	// the binary's mtime is meaningful.
	id.dir.mtime = time.Time{}
	euid := os.Geteuid()
	for _, e := range []statIdentity{id.bin, id.dir} {
		if e.mode.Perm()&0o022 != 0 {
			return nil, fmt.Errorf("refusing provider: %s is group or world writable (%v)", e.path, e.mode.Perm())
		}
		if int(e.uid) != euid && e.uid != 0 {
			return nil, fmt.Errorf("refusing provider: %s is not owned by the current user or root", e.path)
		}
	}
	if err := fsperm.CheckAncestors(filepath.Dir(id.dir.path), euid); err != nil {
		return nil, fmt.Errorf("refusing provider: %w", err)
	}
	return id, nil
}

// captureIdentity records the provider's identity at Resolve time.
func captureIdentity(path string) (*fileIdentity, error) {
	return readIdentityChecked(path)
}

// verifyIdentity re-checks the provider before an exec. The check-to-exec gap
// is not closed (exec by path), but a swapped, replaced or re-permissioned
// binary or directory is caught on the next poll.
func verifyIdentity(path string, want *fileIdentity) error {
	got, err := readIdentityChecked(path)
	if err != nil {
		return err
	}
	if !sameIdentity(got.bin, want.bin) || !sameIdentity(got.dir, want.dir) {
		return fmt.Errorf("refusing provider: %s changed since startup", path)
	}
	return nil
}

func sameIdentity(a, b statIdentity) bool {
	return a.path == b.path && a.dev == b.dev && a.ino == b.ino && a.uid == b.uid &&
		a.mode == b.mode && a.mtime.Equal(b.mtime)
}

// execPath is the symlink-resolved path that was verified.
func (f *fileIdentity) execPath(_ string) string { return f.bin.path }
