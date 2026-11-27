//go:build windows

package fscopy

import (
	"errors"
	"syscall"
)

// Win32 errors libuv maps to EXDEV, EPERM, EBUSY and EACCES.
const (
	errorAccessDenied     syscall.Errno = 5
	errorNotSameDevice    syscall.Errno = 17
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
	errorBusy             syscall.Errno = 170
)

// isMoveFallback reports the rename errors moveEntry recovers from by
// copying: cross-device moves, access denied and busy or locked files.
func isMoveFallback(err error) bool {
	for _, code := range []syscall.Errno{errorAccessDenied, errorNotSameDevice, errorSharingViolation, errorLockViolation, errorBusy} {
		if errors.Is(err, code) {
			return true
		}
	}
	return errors.Is(err, syscall.EXDEV) || errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EACCES)
}
