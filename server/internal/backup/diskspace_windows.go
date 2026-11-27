//go:build windows

package backup

import "golang.org/x/sys/windows"

// freeSpace returns the bytes available to this process on the volume
// holding dir; ok is false when the system cannot tell.
func freeSpace(dir string) (free uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, false
	}
	var avail, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &totalFree); err != nil {
		return 0, false
	}
	return avail, true
}
