//go:build windows

package pathsec

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Windows has no O_NONBLOCK or O_NOFOLLOW; FinalPath already resolves every
// reparse point, so callers open fully resolved paths.
const (
	openNonBlock = 0
	openNoFollow = 0
)

// Flags of GetFinalPathNameByHandleW (not exported by x/sys/windows).
const (
	fileNameNormalized = 0x0
	volumeNameDOS      = 0x0
)

// finalPath asks GetFinalPathNameByHandle for the normalized DOS path of the
// handle, which follows symlinks, junctions and mount points (unlike
// filepath.EvalSymlinks since Go 1.23).
func finalPath(f *os.File) (string, error) {
	h := windows.Handle(f.Fd())
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), fileNameNormalized|volumeNameDOS)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return stripExtendedPrefix(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n+1)
	}
}

// stripExtendedPrefix turns `\\?\C:\x` into `C:\x` and `\\?\UNC\srv\share`
// into `\\srv\share`.
func stripExtendedPrefix(p string) string {
	if rest, ok := strings.CutPrefix(p, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	return strings.TrimPrefix(p, `\\?\`)
}
