//go:build unix && !linux

package pathsec

import (
	"os"
	"path/filepath"
	"syscall"
)

const (
	openNonBlock = syscall.O_NONBLOCK
	openNoFollow = syscall.O_NOFOLLOW
)

// finalPath resolves the name of f; Unix systems without /proc offer no
// portable descriptor-to-path call.
func finalPath(f *os.File) (string, error) {
	return filepath.EvalSymlinks(f.Name())
}
