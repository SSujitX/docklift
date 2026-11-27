package backup

import (
	"archive/zip"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"docklift/internal/pathsec"
)

// archiveWriter adds entries to a backup zip the way archiver 8 did.
type archiveWriter struct {
	zw *zip.Writer
	// warn receives "[WARN] ..." lines for skipped entries.
	warn func(string)
}

func newArchiveWriter(w io.Writer, warn func(string)) *archiveWriter {
	zw := zip.NewWriter(w)
	// archiver was created with zlib level 9.
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestCompression)
	})
	return &archiveWriter{zw: zw, warn: warn}
}

func (a *archiveWriter) Close() error { return a.zw.Close() }

// entryMode is the mode archiver stored: the permission and setuid/setgid/
// sticky bits on Unix, the permission bits on Windows (directories 0755).
func entryMode(fi fs.FileInfo) fs.FileMode {
	m := fi.Mode()
	if runtime.GOOS == "windows" {
		if m.IsDir() {
			return 0o755
		}
		return m.Perm()
	}
	return m & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
}

// addFile stores the regular file at src as name (deflated).
func (a *archiveWriter) addFile(ctx context.Context, src, name string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	return a.writeFile(ctx, f, fi, name)
}

func (a *archiveWriter) writeFile(ctx context.Context, r io.Reader, fi fs.FileInfo, name string) error {
	h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: fi.ModTime()}
	h.SetMode(entryMode(fi))
	w, err := a.zw.CreateHeader(h)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, ctxReader{ctx, r}); err != nil {
		return fmt.Errorf("archive %s: %w", name, err)
	}
	return nil
}

// addDir stores the tree below root under prefix: every directory as a
// "name/" entry, every file deflated, every symlink as a link entry holding
// the target relative to the link's directory (never followed), the order a
// pre-order walk visits them. Entries that vanish while the walk runs are
// skipped silently (archiver ignored ENOENT warnings); sockets, FIFOs and
// devices are skipped with a warning.
func (a *archiveWriter) addDir(ctx context.Context, root, prefix string) error {
	// readdir-glob read the root through links; resolve it the same way.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return a.walk(ctx, root, root, prefix)
}

func (a *archiveWriter) walk(ctx context.Context, root, dir, prefix string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && dir != root {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		full := filepath.Join(dir, e.Name())
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		name := prefix + "/" + filepath.ToSlash(rel)
		fi, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		switch mode := fi.Mode(); {
		case mode&fs.ModeSymlink != 0:
			if err := a.addSymlink(full, name, fi); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return err
			}
		case mode.IsDir():
			h := &zip.FileHeader{Name: name + "/", Method: zip.Store, Modified: fi.ModTime()}
			h.SetMode(fs.ModeDir | entryMode(fi))
			if _, err := a.zw.CreateHeader(h); err != nil {
				return err
			}
			if err := a.walk(ctx, root, full, prefix); err != nil {
				return err
			}
		case mode.IsRegular():
			f, err := os.Open(full)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			err = a.writeFile(ctx, f, fi, name)
			_ = f.Close()
			if err != nil {
				return err
			}
		default:
			a.warn("[WARN] entry not supported: " + name + "\n")
		}
	}
	return nil
}

// addSymlink stores a link entry. The link text is archiver's
// relative(dirname(link), resolve(dirname(link), readlink(link))), with
// forward slashes.
func (a *archiveWriter) addSymlink(full, name string, fi fs.FileInfo) error {
	target, err := os.Readlink(full)
	if err != nil {
		return err
	}
	dir := filepath.Dir(full)
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(dir, target)
	}
	linkname, err := filepath.Rel(dir, resolved)
	if err != nil {
		linkname = target
	}
	h := &zip.FileHeader{Name: name, Method: zip.Store, Modified: fi.ModTime()}
	h.SetMode(fs.ModeSymlink | (fi.Mode() & fs.ModePerm))
	w, err := a.zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, filepath.ToSlash(linkname))
	return err
}

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// linkEntry is a symlink entry of an archive (name relative to the archive
// root, target as stored).
type linkEntry struct {
	name   string
	target string
}

// maxLinkTarget bounds the link text read from an archive entry.
const maxLinkTarget = 4096

// archiveLinks lists the symlink entries of the archive at zipPath.
func archiveLinks(zipPath string) ([]linkEntry, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, err
	}
	defer zr.Close()
	var out []linkEntry
	for _, f := range zr.File {
		if f.Mode()&fs.ModeSymlink == 0 || f.UncompressedSize64 > maxLinkTarget {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxLinkTarget+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, linkEntry{name: f.Name, target: string(b)})
	}
	return out, nil
}

// archiveSize is the total declared uncompressed size of the file entries.
func archiveSize(zipPath string) (uint64, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return 0, err
	}
	defer zr.Close()
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
	}
	return total, nil
}

// safeRelPath reports whether an archive path (forward or back slashes) is
// relative and stays below the extraction root: no "..", drive letter,
// stream suffix or segment Windows would reduce to "..".
func safeRelPath(name string) bool {
	raw := strings.ReplaceAll(name, `\`, "/")
	if raw == "" || strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, "\x00:") {
		return false
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." || (seg != "" && seg != "." && strings.TrimRight(seg, ". ") == "") {
			return false
		}
	}
	return true
}

// restoreDirs creates the directory entries of the archive below destDir.
// Extraction (safeunzip, like unzipper in 2.1.6) skips them, so empty
// directories of a backup were lost on restore. Unsafe names are skipped;
// nothing is created outside destDir.
func restoreDirs(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return err
	}
	defer zr.Close()
	realDest, err := pathsec.RealPath(destDir)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, "/") || f.UncompressedSize64 != 0 {
			continue
		}
		rel := strings.TrimSuffix(f.Name, "/")
		if !safeRelPath(rel) {
			continue
		}
		p := filepath.Join(destDir, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		if !pathsec.IsInside(destDir, p) {
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
		if real, err := pathsec.RealPath(p); err != nil || !pathsec.IsInside(realDest, real) {
			return fmt.Errorf("Zip-slip blocked: %s", f.Name)
		}
	}
	return nil
}

// safeLinkTarget reports whether a link entry named name (inside area) with
// the given target may be recreated as a symlink: the target is relative and
// stays inside the area once resolved against the link's directory.
func safeLinkTarget(area, name, target string) bool {
	if target == "" || path.IsAbs(target) || filepath.IsAbs(target) || filepath.VolumeName(target) != "" {
		return false
	}
	for _, c := range target {
		if c == 0 || c == '\\' || c == ':' {
			return false
		}
	}
	rel, ok := cutArea(area, name)
	if !ok {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(rel), target))
	return resolved != ".." && !startsWithDotDot(resolved)
}

func startsWithDotDot(p string) bool {
	return len(p) >= 3 && p[:3] == "../"
}

// cutArea returns name relative to the area prefix ("letsencrypt/live/x" ->
// "live/x").
func cutArea(area, name string) (string, bool) {
	prefix := area + "/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return "", false
	}
	return name[len(prefix):], true
}

// relinkArea turns the regular files that extraction wrote for the area's
// symlink entries (safeunzip writes link text as file content) back into
// symlinks inside the restored target directory, so certbot's
// live/<domain>/*.pem links work after a restore. Unsafe targets keep the
// file, as 2.1.6 did. Each link replaces its file atomically (a temporary
// link renamed over it). It returns how many links were recreated and the
// first error.
func relinkArea(links []linkEntry, area, targetDir string) (int, error) {
	n := 0
	for _, l := range links {
		if !safeLinkTarget(area, l.name, l.target) {
			continue
		}
		rel, _ := cutArea(area, l.name)
		dst := filepath.Join(targetDir, filepath.FromSlash(rel))
		fi, err := os.Lstat(dst)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		tmp := filepath.Join(filepath.Dir(dst), fmt.Sprintf(".%s.link-%d", filepath.Base(dst), time.Now().UnixNano()))
		if err := os.Symlink(filepath.FromSlash(l.target), tmp); err != nil {
			return n, err
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			return n, err
		}
		n++
	}
	return n, nil
}
