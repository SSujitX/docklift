package projects

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"docklift/internal/deploy"
	"docklift/internal/gitops"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/safeunzip"
	"docklift/internal/store"
)

// Upload limits of the create route.
const (
	// MaxUploadZipBytes is the largest ZIP accepted (compressed).
	MaxUploadZipBytes = 100 * 1024 * 1024
	// MaxExtractedBytes is the most an upload may expand to.
	MaxExtractedBytes = 512 * 1024 * 1024
	// MaxExtractedFiles is the most files an upload may contain.
	MaxExtractedFiles = 20_000

	// uploadField is the only form field that may carry a file.
	uploadField = "files"
	// maxFieldBytes is busboy's default per-field value limit (1 MiB).
	maxFieldBytes = 1 << 20
	// maxParts and maxFieldsBytes bound the non-file parts of one form.
	maxParts       = 1000
	maxFieldsBytes = httpx.MaxBodyBytes
	// maxDrainBytes bounds how much of a rejected upload is read and discarded.
	maxDrainBytes = 64 << 20
	// uploadReadTimeout bounds reading the whole multipart body.
	uploadReadTimeout = 30 * time.Minute
)

// maxUploadBytes is MaxUploadZipBytes (lowered by tests).
var maxUploadBytes int64 = MaxUploadZipBytes

const (
	msgNameRequired   = "Project name is required (max 120 characters)"
	msgSourceRequired = "A valid source is required: GitHub repository URL or ZIP upload"
	msgGithubURL      = "A valid GitHub repository URL is required"
	msgZipRequired    = "ZIP upload is required for upload projects"
	msgInvalidRef     = "Invalid branch or tag name"
	msgUploadFailed   = "Upload failed"
	msgUnsafeZip      = "Invalid or unsafe ZIP upload"
	msgCreateFailed   = "Failed to create project"
)

var errUploadTooLarge = errors.New("upload too large")

// readUpload parses a multipart body the way multer.single('files') with
// {fileSize: 100 MiB, files: 1} did: text parts become fields (a repeated
// name becomes an array), the one file part named "files" is streamed to a
// temporary file under DATA_PATH/uploads. Any other file part, a second
// file, an oversized part or a malformed body is an error. On error no
// temporary file is left behind.
func (h *handlers) readUpload(r *http.Request, boundary string) (fields, string, error) {
	mr := multipart.NewReader(r.Body, boundary)
	out := fields{}
	tmp := ""
	cleanup := func() {
		if tmp != "" {
			_ = os.Remove(tmp)
		}
	}
	parts, fieldBytes := 0, 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return out, tmp, nil
		}
		if err != nil {
			cleanup()
			return nil, "", err
		}
		parts++
		if parts > maxParts+1 {
			cleanup()
			return nil, "", errors.New("too many parts")
		}
		name := part.FormName()
		if part.FileName() != "" {
			if name != uploadField || tmp != "" {
				cleanup()
				return nil, "", fmt.Errorf("unexpected file part %q", name)
			}
			tmp, err = h.saveUpload(part)
			if err != nil {
				return nil, "", err
			}
			continue
		}
		if name == "" {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
		if err == nil && len(b) > maxFieldBytes {
			err = errors.New("field value too long")
		}
		if fieldBytes += len(b); err == nil && fieldBytes > maxFieldsBytes {
			err = errors.New("form fields too large")
		}
		if err != nil {
			cleanup()
			return nil, "", err
		}
		switch prev := out[name].(type) {
		case nil:
			out[name] = string(b)
		case string:
			out[name] = []any{prev, string(b)}
		case []any:
			out[name] = append(prev, string(b))
		}
	}
}

// saveUpload streams one file part into DATA_PATH/uploads, refusing more
// than maxUploadBytes; on error the file is removed.
func (h *handlers) saveUpload(part io.Reader) (string, error) {
	dir := filepath.Join(h.d.Config.DataPath, "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "upload-*.zip")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(part, maxUploadBytes+1))
	if err == nil && n > maxUploadBytes {
		err = errUploadTooLarge
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// createBody returns req.body (and the uploaded file, if any) of POST /,
// answering the upload errors itself. ok is false when a response was
// written.
func (h *handlers) createBody(w http.ResponseWriter, r *http.Request) (fields, string, bool) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return jsonFields(httpx.BodyFrom(r)), "", true
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadReadTimeout))
	var f fields
	tmp := ""
	if boundary := params["boundary"]; boundary == "" {
		err = errors.New("multipart boundary not found")
	} else {
		f, tmp, err = h.readUpload(r, boundary)
	}
	if err == nil {
		_ = rc.SetReadDeadline(time.Time{})
		return f, tmp, true
	}
	h.d.Log.Debug("project upload rejected", slog.Any("error", err))
	// Drain what is left so a client still sending reads the answer.
	if _, derr := io.CopyN(io.Discard, r.Body, maxDrainBytes); errors.Is(derr, io.EOF) {
		_ = rc.SetReadDeadline(time.Time{})
	} else {
		w.Header().Set("Connection", "close")
	}
	if errors.Is(err, errUploadTooLarge) {
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Upload too large (max %d bytes)", maxUploadBytes))
	} else {
		httpx.Error(w, http.StatusBadRequest, msgUploadFailed)
	}
	return nil, "", false
}

// validGithubRepoURL is isValidGithubRepoUrl: an http(s) URL on github.com
// or www.github.com with at least owner and repository path segments. URLs
// with embedded credentials are refused, so a token can never be stored in
// github_url or the checkout's origin.
func validGithubRepoURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return false
	}
	if host := strings.ToLower(u.Hostname()); host != "github.com" && host != "www.github.com" {
		return false
	}
	p := strings.TrimSuffix(path.Clean("/"+u.Path), ".git")
	n := 0
	for _, seg := range strings.Split(p, "/") {
		if seg != "" {
			n++
		}
	}
	return n >= 2
}

// POST / — create a project from a GitHub repository (cloned now) or a ZIP
// upload (extracted now). The row is created first and rolled back, with
// its files, if the clone or extraction fails.
func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	b, upload, ok := h.createBody(w, r)
	if !ok {
		return
	}
	if upload != "" {
		defer os.Remove(upload)
	}

	nameVal, _ := b.get("name").str()
	name := js.Trim(nameVal)
	if name == "" || httpx.UTF16Len(name) > 120 {
		httpx.Error(w, http.StatusBadRequest, msgNameRequired)
		return
	}
	githubURLVal, githubURLIsStr := b.get("github_url").str()
	source := ""
	switch st, _ := b.get("source_type").str(); {
	case st == "github" || st == "upload":
		source = st
	case upload != "":
		source = "upload"
	case githubURLIsStr && js.Trim(githubURLVal) != "":
		source = "github"
	}
	if source == "" {
		httpx.Error(w, http.StatusBadRequest, msgSourceRequired)
		return
	}
	githubURL := js.Trim(githubURLVal)
	if source == "github" {
		if githubURL == "" || !validGithubRepoURL(githubURL) || h.d.Git.ValidateURL(githubURL) != nil {
			httpx.Error(w, http.StatusBadRequest, msgGithubURL)
			return
		}
	} else if upload == "" {
		httpx.Error(w, http.StatusBadRequest, msgZipRequired)
		return
	}
	domain := b.get("domain")
	if !checkDomain(w, domain) {
		return
	}
	description, ok := truthyString(w, b.get("description"), "description")
	if !ok {
		return
	}
	branch, ok := truthyString(w, b.get("github_branch"), "github_branch")
	if !ok {
		return
	}
	if source == "github" && branch != "" && gitops.ValidateRef(branch) != nil {
		httpx.Error(w, http.StatusBadRequest, msgInvalidRef)
		return
	}
	projectType, ok := truthyString(w, b.get("project_type"), "project_type")
	if !ok {
		return
	}
	if projectType == "" {
		projectType = "app"
	}
	domainStr, _ := domain.str()

	in := store.ProjectCreate{
		Name:           name,
		Description:    nonEmpty(description),
		SourceType:     &source,
		GithubBranch:   nonEmpty(branch),
		ProjectType:    &projectType,
		Domain:         nonEmpty(domainStr),
		Status:         store.Ptr(deploy.StatusPending),
		AutoDeploy:     store.Ptr(source == "github"),
		BuildType:      store.Ptr(deploy.NormalizeBuildType(b.get("build_type").v)),
		BaseDirectory:  store.Ptr(baseDirectory(b.get("base_directory"))),
		DockerfilePath: dockerfilePath(b.get("dockerfile_path")),
		InternalPort:   store.Ptr(clampPort(b.get("internal_port"))),
	}
	if source == "github" {
		secret, err := generateWebhookSecret()
		if err != nil {
			h.fail(w, msgCreateFailed, err)
			return
		}
		in.GithubURL, in.WebhookSecret = &githubURL, &secret
	}
	// Clone and extraction continue if the client disconnects, as in TS.
	ctx := h.d.Background
	project, err := h.d.Store.CreateProject(ctx, in)
	if err != nil {
		h.fail(w, msgCreateFailed, err)
		return
	}
	dir := h.d.Deploy.ProjectPath(project.ID)
	if source == "github" {
		status, msg, err := h.cloneProject(ctx, dir, githubURL, branch)
		if err != nil {
			h.d.Log.Error(msg+" — rolling back project", slog.String("project", project.ID), slog.Any("error", err))
			h.rollback(project.ID)
			httpx.Error(w, status, msg)
			return
		}
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			h.rollback(project.ID)
			h.fail(w, msgCreateFailed, err)
			return
		}
		err := safeunzip.Extract(ctx, upload, dir, safeunzip.Options{MaxFiles: MaxExtractedFiles, MaxUncompressedBytes: MaxExtractedBytes})
		if err != nil {
			h.rollback(project.ID)
			if errors.Is(err, safeunzip.ErrTooLarge) || errors.Is(err, safeunzip.ErrTooManyFiles) {
				httpx.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			h.d.Log.Warn("project upload extraction failed", slog.String("project", project.ID), slog.Any("error", err))
			httpx.Error(w, http.StatusBadRequest, msgUnsafeZip)
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, project)
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// cloneProject clones the repository (authenticated with the GitHub App
// installation when one covers it; the token is sent as a header and never
// stored), then scrubs and verifies the origin remote. On failure it
// returns the response status and message.
func (h *handlers) cloneProject(ctx context.Context, dir, repoURL, ref string) (int, string, error) {
	creds := h.cloneCredentials(ctx, repoURL)
	if err := h.d.Git.Clone(ctx, repoURL, dir, gitops.CloneOptions{Ref: ref, Credentials: creds}); err != nil {
		return http.StatusInternalServerError, "Failed to clone repository", err
	}
	err := h.d.Git.ScrubOrigin(ctx, dir, repoURL)
	if err == nil {
		err = h.d.Git.VerifyNoCredentials(ctx, dir)
	}
	if err != nil {
		return http.StatusInternalServerError, "Failed to secure repository credentials after clone", err
	}
	return 0, "", nil
}

// cloneCredentials looks up an installation token for a GitHub URL: the
// repository's installation, else the saved github_installation_id. Any
// failure falls back to an anonymous clone, as in TS.
func (h *handlers) cloneCredentials(ctx context.Context, repoURL string) *gitops.Credentials {
	if h.github == nil {
		return nil
	}
	owner, repo, ok := gitops.GitHubOwnerRepo(repoURL)
	if !ok {
		return nil
	}
	installID, err := h.github.InstallationIDForRepo(ctx, owner, repo)
	if err != nil {
		h.d.Log.Warn(fmt.Sprintf("Dynamic installation lookup failed for %s/%s, trying saved ID", owner, repo), slog.Any("error", err))
		if installID, _, err = h.d.Store.GetSetting(ctx, "github_installation_id"); err != nil {
			h.d.Log.Warn("Failed to inject GitHub token, trying public clone", slog.Any("error", err))
			return nil
		}
	}
	if installID == "" {
		return nil
	}
	token, err := h.github.InstallationToken(ctx, installID)
	if err != nil || token == "" {
		h.d.Log.Warn("Failed to inject GitHub token, trying public clone", slog.Any("error", err))
		return nil
	}
	return gitops.InstallationCredentials(token)
}

// rollback removes a project that could not be populated: its files, then
// its row (rollbackProject; failures are ignored).
func (h *handlers) rollback(projectID string) {
	_ = os.RemoveAll(h.d.Deploy.ProjectPath(projectID))
	if err := h.d.Store.DeleteProject(h.d.Background, projectID); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.d.Log.Warn("project rollback failed", slog.String("project", projectID), slog.Any("error", err))
	}
}
