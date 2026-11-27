// Package files serves /api/files: the project file tree and the file
// editor's read and write.
//
// Containment is checked twice. First lexically: the requested path,
// resolved like a standard path resolve (absolute, rooted and drive-qualified
// paths stand on their own), must stay inside the project and must not touch
// a `.git` segment in any letter case. Then on the opened handle: the
// kernel's final path of the file (every symlink, junction and mount point
// followed) must stay inside the real project path and outside `.git`. Reads
// come from that same handle; writes reopen the resolved path without
// following a final symlink and must hit the same file, so a link swapped in
// between check and use cannot redirect them.
package files

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/httpx"
	"docklift/internal/pathsec"
)

// MaxEditableBytes is the largest file the editor opens: the request body
// limit, since a bigger file could never be saved back.
const MaxEditableBytes = httpx.MaxBodyBytes

// maxDepth is listFilesRecursive's depth limit.
const maxDepth = 10

// validProjectID only allows UUIDs and alphanumeric-dash strings.
var validProjectID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// skipDirs are never listed (files with these names neither).
var skipDirs = map[string]bool{"node_modules": true, ".git": true, ".next": true, "dist": true, "__pycache__": true, ".venv": true, "venv": true}

// binaryExtensions mark files as not editable in the tree.
var binaryExtensions = map[string]bool{}

func init() {
	for _, ext := range strings.Fields(`png jpg jpeg gif bmp ico svg webp zip tar gz 7z rar exe dll so dylib bin
		pdf doc docx xls xlsx ppt pptx mp3 mp4 wav avi mov webm sqlite db`) {
		binaryExtensions[ext] = true
	}
}

const (
	msgAccessDenied  = "Access denied"
	msgSymlinkEscape = "Access denied - symlink outside project"
	msgGitRead       = "Access denied - git metadata is not accessible"
	msgGitWrite      = "Access denied - git metadata is not editable"
	msgNotFound      = "File not found"
	msgReadFailed    = "Failed to read file"
	msgWriteFailed   = "Failed to update file"
)

type handlers struct {
	d *app.Deps
}

// Mount registers the /api/files routes on r, which is already scoped to
// /api/files and wrapped in the group's authentication middleware.
func Mount(r chi.Router, d *app.Deps) {
	h := &handlers{d: d}
	r.Get("/{projectId}", h.tree)
	r.Get("/{projectId}/content", h.read)
	r.Put("/{projectId}/content", h.write)
}

// projectPath validates the projectId parameter (decoded once)
// and returns deployments/<id>.
func (h *handlers) projectPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "projectId")
	if r.URL.RawPath != "" {
		decoded, err := url.PathUnescape(id)
		if err != nil {
			decoded = id
		}
		id = decoded
	}
	if !validProjectID.MatchString(id) {
		httpx.Error(w, http.StatusBadRequest, "Invalid project ID")
		return "", false
	}
	return filepath.Join(h.d.Config.DeploymentsPath, id), true
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error(msg, slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, msg)
}

// node is a tree entry: folders carry children, files size and editable.
type node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Type     string  `json:"type"`
	Size     *int64  `json:"size,omitempty"`
	Editable *bool   `json:"editable,omitempty"`
	Children *[]node `json:"children,omitempty"`
}

// GET /:projectId — the project's file tree.
func (h *handlers) tree(w http.ResponseWriter, r *http.Request) {
	projectPath, ok := h.projectPath(w, r)
	if !ok {
		return
	}
	if _, err := os.Stat(projectPath); err != nil {
		httpx.Error(w, http.StatusNotFound, "Project files not found")
		return
	}
	files, err := listFiles(projectPath, projectPath, 0)
	if err != nil {
		h.fail(w, "Failed to list files", err)
		return
	}
	httpx.JSON(w, http.StatusOK, files)
}

// listFiles is listFilesRecursive: symlinks (and on Windows junctions and
// other reparse points) are skipped, folders come first, then names in
// localeCompare order. Paths are project-relative with forward slashes on
// every OS.
func listFiles(base, dir string, depth int) ([]node, error) {
	out := []node{}
	if depth > maxDepth {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if skipDirs[name] {
			continue
		}
		full := filepath.Join(dir, name)
		fi, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			continue
		}
		rel, err := filepath.Rel(base, full)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if fi.IsDir() {
			children, err := listFiles(base, full, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, node{Name: name, Path: rel, Type: "folder", Children: &children})
			continue
		}
		ext := strings.ToLower(name[strings.LastIndexByte(name, '.')+1:])
		size, editable := fi.Size(), !binaryExtensions[ext]
		out = append(out, node{Name: name, Path: rel, Type: "file", Size: &size, Editable: &editable})
	}
	slices.SortStableFunc(out, func(a, b node) int {
		if a.Type != b.Type {
			if a.Type == "folder" {
				return -1
			}
			return 1
		}
		return localeCompare(a.Name, b.Name)
	})
	return out, nil
}

// target is a requested file that passed every containment check, open
// read-only.
type target struct {
	file *os.File
	info fs.FileInfo
	// real is the kernel's final path of file.
	real string
	// name is path.basename of the requested (unresolved) path.
	name string
}

// open runs the checks of the content routes and opens the file. On
// failure it has written the response and returns nil.
func (h *handlers) open(w http.ResponseWriter, r *http.Request, gitMsg, failMsg string) *target {
	projectPath, ok := h.projectPath(w, r)
	if !ok {
		return nil
	}
	values := r.URL.Query()["path"]
	if len(values) > 1 {
		// req.query.path is an array; path.resolve threw a TypeError.
		h.fail(w, failMsg, errors.New("path query parameter repeated"))
		return nil
	}
	rel := ""
	if len(values) == 1 {
		rel = values[0]
	}
	filePath := pathsec.Resolve(projectPath, rel)
	if !pathsec.IsInside(projectPath, filePath) {
		httpx.Error(w, http.StatusForbidden, msgAccessDenied)
		return nil
	}
	if pathsec.TouchesGitDir(projectPath, filePath) {
		httpx.Error(w, http.StatusForbidden, gitMsg)
		return nil
	}
	if strings.ContainsRune(filePath, 0) {
		httpx.Error(w, http.StatusNotFound, msgNotFound)
		return nil
	}
	if _, err := os.Stat(filePath); err != nil {
		httpx.Error(w, http.StatusNotFound, msgNotFound)
		return nil
	}
	realProject, err := pathsec.RealPath(projectPath)
	if err != nil {
		h.fail(w, failMsg, err)
		return nil
	}
	f, err := pathsec.OpenRead(filePath)
	if err != nil {
		h.fail(w, failMsg, err)
		return nil
	}
	t, msg, status, err := inspect(f, realProject, gitMsg)
	if err != nil || msg != "" {
		f.Close()
		if err != nil {
			h.fail(w, failMsg, err)
		} else {
			httpx.Error(w, status, msg)
		}
		return nil
	}
	t.name = filepath.Base(filePath)
	return t
}

// inspect checks the final path of an open file against the real project
// path. It returns a client error (msg, status) or an internal error.
func inspect(f *os.File, realProject, gitMsg string) (*target, string, int, error) {
	real, err := pathsec.FinalPath(f)
	if err != nil {
		return nil, "", 0, err
	}
	if !pathsec.IsInside(realProject, real) {
		return nil, msgSymlinkEscape, http.StatusForbidden, nil
	}
	if pathsec.TouchesGitDir(realProject, real) {
		return nil, gitMsg, http.StatusForbidden, nil
	}
	info, err := f.Stat()
	if err != nil {
		return nil, "", 0, err
	}
	return &target{file: f, info: info, real: real}, "", 0, nil
}

type contentBody struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// GET /:projectId/content?path=... — read a file as UTF-8 text.
func (h *handlers) read(w http.ResponseWriter, r *http.Request) {
	t := h.open(w, r, msgGitRead, msgReadFailed)
	if t == nil {
		return
	}
	defer t.file.Close()
	switch {
	case t.info.IsDir():
		httpx.Error(w, http.StatusBadRequest, "Path is a directory")
		return
	case !t.info.Mode().IsRegular():
		httpx.Error(w, http.StatusBadRequest, "Path is not a regular file")
		return
	case t.info.Size() > MaxEditableBytes:
		httpx.Error(w, http.StatusRequestEntityTooLarge, "File is too large to open in the editor")
		return
	}
	b, err := io.ReadAll(io.LimitReader(t.file, MaxEditableBytes+1))
	if err != nil {
		h.fail(w, msgReadFailed, err)
		return
	}
	if len(b) > MaxEditableBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "File is too large to open in the editor")
		return
	}
	// Invalid UTF-8 becomes U+FFFD, as standard UTF-8 decoding does.
	httpx.JSON(w, http.StatusOK, contentBody{Name: t.name, Content: strings.ToValidUTF8(string(b), "�")})
}

// PUT /:projectId/content?path=... — overwrite an existing file with
// body.content. The editor never creates files.
func (h *handlers) write(w http.ResponseWriter, r *http.Request) {
	t := h.open(w, r, msgGitWrite, msgWriteFailed)
	if t == nil {
		return
	}
	defer t.file.Close()
	content, ok := httpx.BodyFrom(r).Get("content").String()
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "File content must be a string")
		return
	}
	if !t.info.Mode().IsRegular() {
		h.fail(w, msgWriteFailed, fmt.Errorf("%s is not a regular file", t.real))
		return
	}
	out, err := pathsec.OpenOverwrite(t.real)
	if err != nil {
		h.fail(w, msgWriteFailed, err)
		return
	}
	defer out.Close()
	if oi, err := out.Stat(); err != nil || !os.SameFile(oi, t.info) {
		h.fail(w, msgWriteFailed, fmt.Errorf("%s changed while it was being opened", t.real))
		return
	}
	if err := out.Truncate(0); err != nil {
		h.fail(w, msgWriteFailed, err)
		return
	}
	if _, err := io.WriteString(out, content); err != nil {
		h.fail(w, msgWriteFailed, err)
		return
	}
	if err := out.Close(); err != nil {
		h.fail(w, msgWriteFailed, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
