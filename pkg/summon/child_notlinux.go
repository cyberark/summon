//go:build !windows && !solaris && !linux

package summon

// groupAlive reports whether process group pgid has any member. Without
// /proc, zombies cannot be told apart from running processes here.
func groupAlive(pgid int) bool { return signalProbe(pgid) }
