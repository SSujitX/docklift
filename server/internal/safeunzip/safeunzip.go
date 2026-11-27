// Package safeunzip extracts ZIP archives with zip-slip rejection and
// file-count / actual-bytes-written quotas (the port of
// backend/src/lib/safeUnzip.ts).
package safeunzip

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"docklift/internal/pathsec"
)

// Defaults of safeExtractZip.
const (
	DefaultMaxFiles             = 50_000
	DefaultMaxUncompressedBytes = 2 * 1024 * 1024 * 1024 // 2 GiB
)

// Options are the quotas; zero values take the defaults.
type Options struct {
	MaxFiles             int
	MaxUncompressedBytes int64
}

// Sentinel errors, matched with errors.Is. The error text is the TS message
// (callers show it to users), e.g. "Archive contains too many files (max 50000)".
var (
	ErrTooManyFiles = errors.New("archive contains too many files")
	ErrTooLarge     = errors.New("archive uncompressed size exceeds limit")
	ErrUnsafePath   = errors.New("unsafe path in archive")
)

type limitError struct {
	kind error
	msg  string
}

func (e *limitError) Error() string        { return e.msg }
func (e *limitError) Is(target error) bool { return target == e.kind }

func tooLarge(maxBytes int64) error {
	return &limitError{ErrTooLarge, fmt.Sprintf("Archive uncompressed size exceeds limit (%d bytes)", maxBytes)}
}

func unsafePath(format, name string) error {
	return &limitError{ErrUnsafePath, fmt.Sprintf(format, name)}
}

// Extract extracts every file entry of the archive at zipPath into destDir
// (created if missing). Directory entries are skipped (directories are
// created for the files inside them), as unzipper did. Symlink entries are
// written as regular files holding the link text; no link is ever created.
//
// It fails on the first entry that is over the quotas, has an empty,
// absolute, NUL-containing or `..` path (backslashes count as separators),
// on Windows a drive letter, stream suffix or a segment the filesystem would
// reduce to `..`, or would land outside destDir — also through a link that
// already exists in destDir. Files written before the failure are left in
// place.
func Extract(ctx context.Context, zipPath, destDir string, opts Options) error {
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	maxBytes := opts.MaxUncompressedBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxUncompressedBytes
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create extract directory: %w", err)
	}
	realDest, err := pathsec.RealPath(destDir)
	if err != nil {
		return err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	fileCount := 0
	var total int64
	for _, entry := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isDirectoryEntry(entry) {
			continue
		}
		fileCount++
		if fileCount > maxFiles {
			return &limitError{ErrTooManyFiles, fmt.Sprintf("Archive contains too many files (max %d)", maxFiles)}
		}
		declared := entry.UncompressedSize64
		if declared > uint64(maxBytes) || uint64(total)+declared > uint64(maxBytes) {
			return tooLarge(maxBytes)
		}
		segments, err := entrySegments(entry.Name, runtime.GOOS == "windows")
		if err != nil {
			return err
		}
		destPath := filepath.Join(append([]string{destDir}, segments...)...)
		if !pathsec.IsInside(destDir, destPath) {
			return unsafePath("Zip-slip blocked: %s", entry.Name)
		}
		parent := filepath.Dir(destPath)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", parent, err)
		}
		if realParent, err := pathsec.RealPath(parent); err != nil || !pathsec.IsInside(realDest, realParent) {
			return unsafePath("Zip-slip blocked: %s", entry.Name)
		}
		if err := writeEntry(ctx, entry, destPath, &total, maxBytes); err != nil {
			return err
		}
	}
	return nil
}

// isDirectoryEntry is unzipper's rule: an empty entry whose path ends in a
// slash or backslash.
func isDirectoryEntry(f *zip.File) bool {
	return f.UncompressedSize64 == 0 && (strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, `\`))
}

// entrySegments validates an entry path and splits it into segments.
func entrySegments(name string, windows bool) ([]string, error) {
	raw := strings.ReplaceAll(name, `\`, "/")
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "\x00") {
		return nil, unsafePath("Unsafe path in archive: %s", name)
	}
	var segments []string
	for _, s := range strings.Split(raw, "/") {
		if s == "" {
			continue
		}
		if s == ".." {
			return nil, unsafePath("Path traversal in archive: %s", name)
		}
		if windows {
			// Drive letters (C:), alternate data streams and device paths.
			if strings.Contains(s, ":") {
				return nil, unsafePath("Unsafe path in archive: %s", name)
			}
			// Win32 drops trailing dots and spaces: "... " and ".. " mean "..".
			if s != "." && strings.TrimRight(s, ". ") == "" {
				return nil, unsafePath("Path traversal in archive: %s", name)
			}
		}
		segments = append(segments, s)
	}
	if len(segments) == 0 {
		return nil, unsafePath("Unsafe path in archive: %s", name)
	}
	return segments, nil
}

func writeEntry(ctx context.Context, entry *zip.File, destPath string, total *int64, maxBytes int64) (err error) {
	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("read %s: %w", entry.Name, err)
	}
	defer src.Close()
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|openNoFollow, 0o666)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", destPath, cerr)
		}
	}()
	buf := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			*total += int64(n)
			if *total > maxBytes {
				return tooLarge(maxBytes)
			}
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write %s: %w", destPath, werr)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("read %s: %w", entry.Name, rerr)
		}
	}
}
