package backup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docklift/internal/httpx"
)

// Upload limits of the multer instance in backup.ts.
const (
	// MaxUploadBytes is the largest accepted backup (aligned with nginx
	// client_max_body_size).
	MaxUploadBytes = 500 * 1024 * 1024
	// uploadField is the only form field that may carry the file.
	uploadField = "backup"
	// maxFieldBytes is busboy's default per-field value limit (1 MiB).
	maxFieldBytes = 1 << 20
	// maxFieldNameBytes is busboy's default field name limit.
	maxFieldNameBytes = 100
	// maxParts and maxFieldsBytes bound the non-file parts of one form.
	maxParts       = 1000
	maxFieldsBytes = httpx.MaxBodyBytes
	// maxDrainBytes bounds how much of a rejected upload is read and discarded.
	maxDrainBytes = 64 << 20
	// uploadReadTimeout bounds reading the whole multipart body.
	uploadReadTimeout = 30 * time.Minute
)

// Client-facing multipart errors (multer / busboy messages).
const (
	msgUnexpectedFile   = "Unexpected file field"
	msgOnlyZip          = "Only ZIP files are allowed"
	msgFileTooLarge     = "File too large"
	msgFieldValueLong   = "Field value too long"
	msgFieldNameLong    = "Field name too long"
	msgFieldNameMissing = "Field name missing"
	msgTooManyParts     = "Too many parts"
	msgNoBoundary       = "Multipart: Boundary not found"
	msgMalformed        = "Unexpected end of form"
)

// uploadError is a multipart body the server refuses. Client mistakes are
// 400; failing to store the file is the server's problem (500).
type uploadError struct {
	status int
	msg    string
	cause  error
}

func (e *uploadError) Error() string {
	if e.cause != nil {
		return e.msg + ": " + e.cause.Error()
	}
	return e.msg
}

func badUpload(msg string) *uploadError { return &uploadError{status: http.StatusBadRequest, msg: msg} }

// uploadedFile is the stored backup part.
type uploadedFile struct {
	path string
	name string
	size int64
}

// readUpload parses a multipart body the way multer.single('backup') with
// {fileSize: 500 MiB} and the ZIP fileFilter did:
//
//   - a non-multipart request is not touched (no file, no fields);
//   - text parts become fields (a repeated name becomes an array);
//   - the one file part named "backup" must be application/zip or a *.zip
//     name; it is streamed to BACKUP_PATH/uploads/restore-<ms>-<hex>.zip
//     (server-chosen name — the client name never reaches the disk) through
//     a .partial file renamed into place once the body is complete;
//   - a file part with an empty filename is ignored; any other file part,
//     a second file, an oversized part or a malformed body is an error, and
//     no file is left behind.
func (h *handlers) readUpload(w http.ResponseWriter, r *http.Request) (*uploadedFile, map[string]any, *uploadError) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, nil, nil
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadReadTimeout))
	file, fields, uerr := h.parseMultipart(r, params["boundary"])
	if uerr == nil {
		_ = rc.SetReadDeadline(time.Time{})
		return file, fields, nil
	}
	h.d.Log.Debug("backup upload rejected", "error", uerr)
	drain(w, r, rc)
	return nil, nil, uerr
}

// drain reads what is left of a rejected body so a client that is still
// sending receives the answer; past maxDrainBytes the connection is closed.
func drain(w http.ResponseWriter, r *http.Request, rc *http.ResponseController) {
	if _, err := io.CopyN(io.Discard, r.Body, maxDrainBytes); errors.Is(err, io.EOF) {
		_ = rc.SetReadDeadline(time.Time{})
		return
	}
	w.Header().Set("Connection", "close")
}

func (h *handlers) parseMultipart(r *http.Request, boundary string) (_ *uploadedFile, _ map[string]any, uerr *uploadError) {
	if boundary == "" {
		return nil, nil, badUpload(msgNoBoundary)
	}
	uploads := h.paths.UploadsDir()
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		return nil, nil, &uploadError{status: http.StatusInternalServerError, msg: "Failed to upload backup", cause: err}
	}
	mr := multipart.NewReader(r.Body, boundary)
	fields := map[string]any{}
	var file *uploadedFile
	partial := ""
	defer func() {
		if uerr != nil && partial != "" {
			_ = os.Remove(partial)
		}
	}()
	parts, fieldBytes := 0, 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, &uploadError{status: http.StatusBadRequest, msg: msgMalformed, cause: err}
		}
		parts++
		if parts > maxParts {
			return nil, nil, badUpload(msgTooManyParts)
		}
		disposition, dparams, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil || disposition != "form-data" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		name, hasName := dparams["name"]
		filename, isFile := dparams["filename"]
		if !hasName {
			return nil, nil, badUpload(msgFieldNameMissing)
		}
		if isFile {
			if filename == "" {
				_, _ = io.Copy(io.Discard, part)
				continue
			}
			if name != uploadField || file != nil {
				return nil, nil, badUpload(msgUnexpectedFile)
			}
			if !zipAllowed(part.Header.Get("Content-Type"), filename) {
				return nil, nil, badUpload(msgOnlyZip)
			}
			f, tmp, serr := h.storeUpload(part, uploads)
			partial = tmp
			if serr != nil {
				return nil, nil, serr
			}
			file = f
			continue
		}
		if len(name) > maxFieldNameBytes {
			return nil, nil, badUpload(msgFieldNameLong)
		}
		value, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
		if err != nil {
			return nil, nil, &uploadError{status: http.StatusBadRequest, msg: msgMalformed, cause: err}
		}
		if len(value) > maxFieldBytes {
			return nil, nil, badUpload(msgFieldValueLong)
		}
		if fieldBytes += len(name) + len(value); fieldBytes > maxFieldsBytes {
			return nil, nil, badUpload(httpx.MsgBodyTooLarge)
		}
		addField(fields, name, string(value))
	}
	if file != nil {
		if err := os.Rename(partial, file.path); err != nil {
			return nil, nil, &uploadError{status: http.StatusInternalServerError, msg: "Failed to upload backup", cause: err}
		}
		partial = ""
	}
	return file, fields, nil
}

// zipAllowed is the fileFilter: mimetype application/zip or a .zip name.
func zipAllowed(contentType, filename string) bool {
	mt := "application/octet-stream"
	if contentType != "" {
		if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
			mt = parsed
		}
	}
	return mt == "application/zip" || strings.HasSuffix(filename, ".zip")
}

// addField is append-field for a plain name: a repeated name becomes an array.
func addField(fields map[string]any, name, value string) {
	switch prev := fields[name].(type) {
	case nil:
		fields[name] = value
	case []any:
		fields[name] = append(prev, value)
	default:
		fields[name] = []any{prev, value}
	}
}

// storeUpload streams the file part to a .partial file under a server-chosen
// name and returns the final location (renamed by the caller once the whole
// body was accepted) and the partial path to remove on failure.
func (h *handlers) storeUpload(part io.Reader, dir string) (*uploadedFile, string, *uploadError) {
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	name := fmt.Sprintf("restore-%d-%s.zip", h.now().UnixMilli(), hex.EncodeToString(suffix))
	final := filepath.Join(dir, name)
	partial := final + ".partial"
	out, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, "", &uploadError{status: http.StatusInternalServerError, msg: "Failed to upload backup", cause: err}
	}
	n, copyErr := io.Copy(out, io.LimitReader(part, MaxUploadBytes+1))
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return nil, partial, &uploadError{status: http.StatusBadRequest, msg: msgMalformed, cause: copyErr}
	case n > MaxUploadBytes:
		return nil, partial, badUpload(msgFileTooLarge)
	case closeErr != nil:
		return nil, partial, &uploadError{status: http.StatusInternalServerError, msg: "Failed to upload backup", cause: closeErr}
	}
	return &uploadedFile{path: final, name: name, size: n}, partial, nil
}

// withFields puts the multipart text fields on the request as its parsed
// body (multer filled req.body), so the step-up check reads the password
// from the form. The fields are re-encoded as JSON and run through
// httpx.ParseBody, the only exported way to build an httpx.Body.
func withFields(r *http.Request, fields map[string]any) *http.Request {
	raw, err := json.Marshal(fields)
	if err != nil {
		return r
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/", bytes.NewReader(raw))
	if err != nil {
		return r
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(raw))
	parsed := httpx.Body{}
	httpx.ParseBody(http.HandlerFunc(func(_ http.ResponseWriter, pr *http.Request) {
		parsed = httpx.BodyFrom(pr)
	})).ServeHTTP(discardWriter{h: http.Header{}}, req)
	return r.WithContext(httpx.WithBody(r.Context(), parsed))
}

// discardWriter is the response writer of withFields (nothing is sent).
type discardWriter struct{ h http.Header }

func (d discardWriter) Header() http.Header       { return d.h }
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) WriteHeader(int)             {}
