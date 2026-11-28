// Package pathsec holds the path-containment checks shared by the file
// editor, uploads and restores,
// plus the handle-based real-path resolution the file editor uses so that
// symlinks, NTFS junctions and swaps between check and use cannot lead
// outside a project.
package pathsec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrOutside is returned when a path resolves outside its root.
var ErrOutside = errors.New("path is outside the allowed directory")

// abs is path.resolve(p): absolute and clean.
func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return a
}

// rel is path.relative(path.resolve(base), path.resolve(candidate)); ok is
// false when no relative path exists (different Windows volumes).
func rel(base, candidate string) (string, bool) {
	r, err := filepath.Rel(abs(base), abs(candidate))
	if err != nil {
		return "", false
	}
	if r == "." {
		r = ""
	}
	return r, true
}

// IsInside reports whether candidate is exactly base or a path inside it,
// after resolving both to absolute clean paths. It rejects `..` escapes,
// absolute escapes and sibling prefixes (`/a/b-evil` is not inside `/a/b`).
// On Windows the comparison ignores letter case.
func IsInside(base, candidate string) bool {
	r, ok := rel(base, candidate)
	if !ok {
		return false
	}
	if r == "" {
		return true
	}
	return r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r) && filepath.VolumeName(r) == ""
}

// TouchesGitDir reports whether candidate is, or sits under, a `.git` entry
// below base, in any letter case. Git metadata (config, hooks) is executed
// by git itself on the next pull, so the file editor must never read or
// write it. On Windows, segment spellings that the filesystem maps onto
// `.git` (trailing dots or spaces, `::$DATA`-style stream suffixes, the 8.3
// short name `GIT~1`) count as `.git` too.
func TouchesGitDir(base, candidate string) bool {
	r, ok := rel(base, candidate)
	if !ok {
		// path.relative yields the absolute target when no relative path exists.
		r = abs(candidate)
	}
	for _, seg := range strings.FieldsFunc(r, func(c rune) bool { return c == '/' || c == '\\' }) {
		if isGitSegment(seg, runtime.GOOS == "windows") {
			return true
		}
	}
	return false
}

func isGitSegment(seg string, windows bool) bool {
	seg = strings.ToLower(seg)
	if windows {
		if i := strings.IndexByte(seg, ':'); i >= 0 {
			seg = seg[:i]
		}
		seg = strings.TrimRight(seg, ". ")
		if strings.HasPrefix(seg, "git~") {
			return true
		}
		return seg == ".git"
	}
	return seg == ".git"
}

// Resolve resolves p against base: an absolute p (or, on Windows, a
// rooted or drive-qualified one such as `\x` or `C:x`) is resolved on its
// own; anything else is joined to base. The result is absolute and clean.
func Resolve(base, p string) string {
	if filepath.IsAbs(p) || filepath.VolumeName(p) != "" || (p != "" && os.IsPathSeparator(p[0])) {
		return abs(p)
	}
	return abs(filepath.Join(abs(base), p))
}

// RealPath returns the fully resolved path of an existing file or directory:
// every symlink, and on Windows every junction and mount point, is followed.
// It opens the path without blocking (FIFOs and devices included) and asks
// the operating system for the final path of the handle.
func RealPath(p string) (string, error) {
	f, err := OpenRead(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return FinalPath(f)
}

// FinalPath returns the absolute path the open file f actually refers to
// (the target of every link on the way), as reported by the kernel for the
// handle rather than re-resolved from its name.
func FinalPath(f *os.File) (string, error) {
	p, err := finalPath(f)
	if err != nil {
		return "", fmt.Errorf("resolve final path of %s: %w", f.Name(), err)
	}
	return filepath.Clean(p), nil
}

// OpenRead opens p read-only. On Unix it does not block on FIFOs (O_NONBLOCK
// has no effect on regular files and directories).
func OpenRead(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_RDONLY|openNonBlock, 0)
}

// OpenOverwrite opens an existing p for writing without creating or
// truncating it. On Unix the final component is never followed when it is a
// symlink (O_NOFOLLOW) and FIFOs do not block, so callers pass a path that
// was already resolved with RealPath.
func OpenOverwrite(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_WRONLY|openNoFollow|openNonBlock, 0)
}
