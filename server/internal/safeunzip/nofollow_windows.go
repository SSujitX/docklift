//go:build windows

package safeunzip

// openNoFollow is a no-op on Windows; the parent's real path is checked
// before every write instead.
const openNoFollow = 0
