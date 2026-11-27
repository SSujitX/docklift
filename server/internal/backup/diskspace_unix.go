//go:build unix

package backup

import "golang.org/x/sys/unix"

// freeSpace returns the bytes available to this process on the filesystem
// holding dir; ok is false when the system cannot tell.
func freeSpace(dir string) (free uint64, ok bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}
