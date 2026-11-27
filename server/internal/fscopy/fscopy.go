// Package fscopy is the cross-platform recursive directory copy and the
// content-level replace used by restores (the port of
// backend/src/lib/fsCopy.ts). It never shells out to `cp`.
//
// Restore targets (/deployments, /nginx-conf, /etc/letsencrypt) are Docker
// bind mounts. Renaming those directories (or renaming staging onto the
// mount path) fails with EBUSY/EXDEV, so ReplaceDirContents replaces the
// contents inside the mount only.
package fscopy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docklift/internal/pathsec"
)

// Name prefixes of the work directories ReplaceDirContents keeps inside the
// target.
const (
	StagingPrefix = ".docklift-restore-staging-"
	PrevPrefix    = ".docklift-restore-prev-"
)

// rename is os.Rename; tests replace it to simulate busy mounts and failures.
var rename = os.Rename

func isRestoreMetaName(name string) bool {
	return strings.HasPrefix(name, StagingPrefix) || strings.HasPrefix(name, PrevPrefix)
}

// copyTree is fs.cp(src, dest, {recursive: true, force: true}).
func copyTree(src, dest string) error {
	srcAbs, destAbs := filepath.Clean(src), filepath.Clean(dest)
	if a, err := filepath.Abs(srcAbs); err == nil {
		srcAbs = a
	}
	if a, err := filepath.Abs(destAbs); err == nil {
		destAbs = a
	}
	if srcAbs != destAbs && pathsec.IsInside(srcAbs, destAbs) {
		return fmt.Errorf("cannot copy %s to a subdirectory of self %s", src, dest)
	}
	fi, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return copyEntry(src, dest, fi)
}

func copyEntry(src, dest string, fi fs.FileInfo) error {
	mode := fi.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		return copySymlink(src, dest)
	case mode.IsDir():
		return copyDir(src, dest, mode.Perm())
	case mode.IsRegular():
		return copyFile(src, dest, mode.Perm())
	case mode&fs.ModeSocket != 0:
		return fmt.Errorf("cannot copy a socket file: %s", src)
	case mode&fs.ModeNamedPipe != 0:
		return fmt.Errorf("cannot copy a FIFO pipe: %s", src)
	default:
		return fmt.Errorf("cannot copy an unknown file type: %s", src)
	}
}

func copyDir(src, dest string, perm fs.FileMode) error {
	if di, err := os.Lstat(dest); err == nil {
		if !di.IsDir() {
			return fmt.Errorf("cannot overwrite non-directory %s with directory %s", dest, src)
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(dest, perm|0o700); err != nil {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		if err := os.Chmod(dest, perm); err != nil {
			return fmt.Errorf("chmod %s: %w", dest, err)
		}
	} else {
		return fmt.Errorf("stat %s: %w", dest, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", filepath.Join(src, e.Name()), err)
		}
		if err := copyEntry(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name()), fi); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string, perm fs.FileMode) (err error) {
	if di, err := os.Lstat(dest); err == nil {
		if di.IsDir() {
			return fmt.Errorf("cannot overwrite directory %s with non-directory %s", dest, src)
		}
		if di.Mode()&fs.ModeSymlink != 0 {
			if err := os.Remove(dest); err != nil {
				return fmt.Errorf("replace %s: %w", dest, err)
			}
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm|0o200)
	if err != nil {
		return fmt.Errorf("copy to %s: %w", dest, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", dest, cerr)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	if err := out.Chmod(perm); err != nil {
		return fmt.Errorf("chmod %s: %w", dest, err)
	}
	return nil
}

func copySymlink(src, dest string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("read link %s: %w", src, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(src), target)
		if a, err := filepath.Abs(target); err == nil {
			target = a
		}
	}
	if _, err := os.Lstat(dest); err == nil {
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("replace %s: %w", dest, err)
		}
	}
	if err := os.Symlink(target, dest); err != nil {
		return fmt.Errorf("copy link %s: %w", src, err)
	}
	return nil
}

// moveEntry prefers rename and falls back to copy+remove when rename fails
// with EXDEV / EPERM / EBUSY / EACCES (cross-device, Windows locks, busy
// files).
func moveEntry(from, to string) error {
	err := rename(from, to)
	if err == nil {
		return nil
	}
	if !isMoveFallback(err) {
		return err
	}
	fi, lerr := os.Lstat(from)
	if lerr != nil {
		return lerr
	}
	if fi.IsDir() {
		if err := copyTree(from, to); err != nil {
			return err
		}
		return os.RemoveAll(from)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := copyEntry(from, to, fi); err != nil {
		return err
	}
	return os.RemoveAll(from)
}

func listContentNames(dir string) ([]string, error) {
	names, err := readNames(dir)
	if err != nil {
		return nil, err
	}
	out := names[:0]
	for _, n := range names {
		if !isRestoreMetaName(n) {
			out = append(out, n)
		}
	}
	return out, nil
}

func readNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func clearContentEntries(dir string) error {
	names, err := listContentNames(dir)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := os.RemoveAll(filepath.Join(dir, n)); err != nil {
			return err
		}
	}
	return nil
}

// restoreAfterPromote runs after a full move-aside and a partial promote:
// it wipes the new content and puts prev back.
func restoreAfterPromote(prev, targetDir string) error {
	if !exists(prev) {
		return nil
	}
	if err := clearContentEntries(targetDir); err != nil {
		return err
	}
	names, err := readNames(prev)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := moveEntry(filepath.Join(prev, n), filepath.Join(targetDir, n)); err != nil {
			return err
		}
	}
	return nil
}

// restoreAfterPartialAside runs after a partial move-aside: it puts the moved
// children back without deleting entries that never left the target.
func restoreAfterPartialAside(prev, targetDir string) error {
	if !exists(prev) {
		return nil
	}
	names, err := readNames(prev)
	if err != nil {
		return err
	}
	for _, n := range names {
		dest := filepath.Join(targetDir, n)
		if exists(dest) {
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
		}
		if err := moveEntry(filepath.Join(prev, n), dest); err != nil {
			return err
		}
	}
	return nil
}

func prevHasEntries(prev string) bool {
	names, err := readNames(prev)
	return err == nil && len(names) > 0
}

func stamp() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b))
}

// ReplaceDirContents replaces the contents of targetDir with those of
// sourceDir.
//
// Staging and prev directories live inside targetDir so Docker bind mounts
// work: targetDir itself is never renamed and no directory is renamed onto
// the mount path. Incomplete staging directories and orphaned prev
// directories of crashed runs are cleaned up. On failure after any child was
// moved aside, the previous contents are put back before the original error
// is returned.
func ReplaceDirContents(sourceDir, targetDir string) (err error) {
	if !exists(sourceDir) {
		return fmt.Errorf("replaceDirContents: source missing: %s", sourceDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	names, err := readNames(targetDir)
	if err != nil {
		return err
	}
	for _, n := range names {
		if strings.HasPrefix(n, StagingPrefix) {
			if err := os.RemoveAll(filepath.Join(targetDir, n)); err != nil {
				return err
			}
		}
	}

	s := stamp()
	staging := filepath.Join(targetDir, StagingPrefix+s)
	prev := filepath.Join(targetDir, PrevPrefix+s)
	promotedAny := false

	defer func() {
		if err == nil {
			return
		}
		// Best effort; the original error is returned.
		if prevHasEntries(prev) {
			if promotedAny {
				_ = restoreAfterPromote(prev, targetDir)
			} else {
				_ = restoreAfterPartialAside(prev, targetDir)
			}
		}
		_ = os.RemoveAll(staging)
		if names, rerr := readNames(prev); rerr == nil && len(names) == 0 {
			_ = os.RemoveAll(prev)
		}
	}()

	if err = os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	if err = copyTree(sourceDir, staging); err != nil {
		return err
	}
	if err = os.MkdirAll(prev, 0o755); err != nil {
		return err
	}

	// Fold orphaned prev dirs from crashed runs into this prev so success can reclaim them.
	if names, err = readNames(targetDir); err != nil {
		return err
	}
	for _, n := range names {
		if !strings.HasPrefix(n, PrevPrefix) || n == filepath.Base(prev) {
			continue
		}
		orphan := filepath.Join(targetDir, n)
		var children []string
		if children, err = readNames(orphan); err != nil {
			return err
		}
		for _, c := range children {
			dest := filepath.Join(prev, c)
			if exists(dest) {
				if err = os.RemoveAll(dest); err != nil {
					return err
				}
			}
			if err = moveEntry(filepath.Join(orphan, c), dest); err != nil {
				return err
			}
		}
		if err = os.RemoveAll(orphan); err != nil {
			return err
		}
	}

	var live []string
	if live, err = listContentNames(targetDir); err != nil {
		return err
	}
	for _, n := range live {
		dest := filepath.Join(prev, n)
		// Live wins over orphan-folded names (a crash mid-promote can leave both).
		if exists(dest) {
			if err = os.RemoveAll(dest); err != nil {
				return err
			}
		}
		if err = moveEntry(filepath.Join(targetDir, n), dest); err != nil {
			return err
		}
	}

	var staged []string
	if staged, err = readNames(staging); err != nil {
		return err
	}
	for _, n := range staged {
		if err = moveEntry(filepath.Join(staging, n), filepath.Join(targetDir, n)); err != nil {
			return err
		}
		promotedAny = true
	}

	if err = os.RemoveAll(staging); err != nil {
		return err
	}
	return os.RemoveAll(prev)
}
