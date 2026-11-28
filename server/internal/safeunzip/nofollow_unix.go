//go:build unix

package safeunzip

import "syscall"

// openNoFollow refuses to write through a symlink that already sits at an
// entry's destination.
const openNoFollow = syscall.O_NOFOLLOW
