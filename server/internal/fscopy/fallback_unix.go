//go:build unix

package fscopy

import (
	"errors"
	"syscall"
)

// isMoveFallback reports the rename errors moveEntry recovers from by
// copying: EXDEV, EPERM, EBUSY and EACCES.
func isMoveFallback(err error) bool {
	return errors.Is(err, syscall.EXDEV) || errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EACCES)
}
