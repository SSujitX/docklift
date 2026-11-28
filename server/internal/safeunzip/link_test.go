package safeunzip

import (
	"os"
	"os/exec"
	"runtime"
)

// linkDir creates a directory link at link pointing to target: an NTFS
// junction on Windows (no privilege needed), a symlink elsewhere.
func linkDir(target, link string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
	}
	return os.Symlink(target, link)
}
