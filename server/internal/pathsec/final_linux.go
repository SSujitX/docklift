//go:build linux

package pathsec

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	openNonBlock = syscall.O_NONBLOCK
	openNoFollow = syscall.O_NOFOLLOW
)

// finalPath reads /proc/self/fd/N, which names the file the descriptor is
// bound to. Without /proc it falls back to resolving the name.
func finalPath(f *os.File) (string, error) {
	p, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd())))
	if err == nil && filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.EvalSymlinks(f.Name())
}
