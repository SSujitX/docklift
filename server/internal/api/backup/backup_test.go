package backup_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/api/backup"
	"docklift/internal/app"
	"docklift/internal/app/apptest"
	"docklift/internal/auth"
	svc "docklift/internal/backup"
	"docklift/internal/httpx"
)

type fakeDocker struct{}

func (*fakeDocker) RunningUserContainers(context.Context) ([]string, error) { return nil, nil }
func (*fakeDocker) StopContainers(context.Context, []string) error          { return nil }
func (*fakeDocker) VolumeCreate(context.Context, string) error              { return nil }
func (*fakeDocker) ComposeUp(context.Context, string, string, string) error { return nil }
func (*fakeDocker) ConnectProxy(context.Context, string) error              { return nil }
func (*fakeDocker) ReloadProxy(context.Context) error                       { return nil }
func (f *fakeReloader) Reload(context.Context) error                        { return f.err }

type fakeReloader struct{ err error }

// server is one installation with the backup routes behind the same
// middleware the router applies (body parsing, maintenance gate, session
// auth or the setup token for POST /restore-upload).
type server struct {
	t        *testing.T
	d        *app.Deps
	h        http.Handler
	token    string
	password string
	restarts chan struct{}
	nginx    *fakeReloader
}

func newServer(t *testing.T, withAdmin bool) *server {
	t.Helper()
	d := apptest.NewDeps(t)
	d.Config.GitHubPrivateKeyPath = filepath.Join(t.TempDir(), "github-app.pem")
	s := &server{t: t, d: d, restarts: make(chan struct{}, 4), nginx: &fakeReloader{}, password: "Passw0rd!x"}
	if withAdmin {
		_, s.token = apptest.CreateAdmin(t, d, "admin@example.test", s.password)
	}
	r := chi.NewRouter()
	r.Use(httpx.ParseBody, httpx.MaintenanceGate(d.Maintenance))
	r.Route("/api/backup", func(sub chi.Router) {
		sub.Use(func(next http.Handler) http.Handler {
			session := d.Auth.RequireSession(next)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/backup/restore-upload" &&
					d.Auth.Bootstrap().ValidateSetupToken(r.Header.Get(auth.SetupTokenHeader)) {
					next.ServeHTTP(w, r.WithContext(auth.WithSetupTokenAuth(r.Context())))
					return
				}
				session.ServeHTTP(w, r)
			})
		})
		backup.MountWith(sub, d, backup.Options{
			Docker: &fakeDocker{}, Nginx: s.nginx, RestartDelay: time.Millisecond,
			Restart: func() { s.restarts <- struct{}{} },
		})
	})
	s.h = r
	return s
}

type resp struct {
	status int
	header http.Header
	body   []byte
}

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("not JSON (%d): %s", r.status, r.body)
	}
	return m
}

func (r resp) expect(t *testing.T, status int, body string) {
	t.Helper()
	if r.status != status || strings.TrimSpace(string(r.body)) != body {
		t.Fatalf("got %d %s, want %d %s", r.status, r.body, status, body)
	}
}

func (s *server) do(method, path string, body io.Reader, contentType string, headers ...string) resp {
	s.t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return resp{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

func (s *server) json(method, path string, v any) resp {
	s.t.Helper()
	if v == nil {
		return s.do(method, path, nil, "")
	}
	raw, _ := json.Marshal(v)
	return s.do(method, path, bytes.NewReader(raw), "application/json")
}

type filePart struct{ field, name, contentType, data string }

func multipartBody(t *testing.T, files []filePart, fields ...string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for i := 0; i+1 < len(fields); i += 2 {
		_ = mw.WriteField(fields[i], fields[i+1])
	}
	for _, f := range files {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name=%q; filename=%q`, f.field, f.name)}
		if f.contentType != "" {
			h["Content-Type"] = []string{f.contentType}
		}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, f.data)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func (s *server) createBackup() (string, string) {
	s.t.Helper()
	r := s.json(http.MethodPost, "/api/backup/create", map[string]any{"name": "t"})
	if r.status != 200 || r.header.Get("Content-Type") != "text/plain; charset=utf-8" {
		s.t.Fatalf("create %d %v", r.status, r.header)
	}
	m := regexp.MustCompile(`  File: (\S+\.zip)\n`).FindStringSubmatch(string(r.body))
	if m == nil {
		s.t.Fatalf("no File line:\n%s", r.body)
	}
	return m[1], string(r.body)
}

func TestBackupFileRoutes(t *testing.T) {
	s := newServer(t, true)
	name, log := s.createBackup()
	if !strings.Contains(log, "  BACKUP COMPLETE\n") {
		t.Fatalf("log:\n%s", log)
	}
	if err := os.MkdirAll(filepath.Join(s.d.Config.BackupPath, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	list := s.do(http.MethodGet, "/api/backup", nil, "")
	var entries []map[string]any
	if err := json.Unmarshal(list.body, &entries); err != nil || len(entries) != 1 || entries[0]["filename"] != name {
		t.Fatalf("list %s", list.body)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`).MatchString(entries[0]["created_at"].(string)) {
		t.Fatalf("created_at %v", entries[0]["created_at"])
	}

	dl := s.do(http.MethodGet, "/api/backup/download/"+name, nil, "")
	if dl.status != 200 || dl.header.Get("Content-Type") != "application/zip" ||
		dl.header.Get("Content-Disposition") != `attachment; filename="`+name+`"` ||
		dl.header.Get("Content-Length") != fmt.Sprint(len(dl.body)) || float64(len(dl.body)) != entries[0]["size"].(float64) {
		t.Fatalf("download %d %v (%d bytes)", dl.status, dl.header, len(dl.body))
	}
	if _, err := zip.NewReader(bytes.NewReader(dl.body), int64(len(dl.body))); err != nil {
		t.Fatal(err)
	}

	s.do(http.MethodDelete, "/api/backup/"+name, nil, "").expect(t, 200, `{"success":true,"message":"Backup deleted successfully"}`)
	s.do(http.MethodDelete, "/api/backup/"+name, nil, "").expect(t, 404, `{"error":"Backup not found"}`)
	s.do(http.MethodGet, "/api/backup/download/"+name, nil, "").expect(t, 404, `{"error":"Backup not found"}`)
	s.do(http.MethodGet, "/api/backup", nil, "").expect(t, 200, `[]`)
	s.do(http.MethodGet, "/api/backup/uploads", nil, "").expect(t, 200, `[]`)

	for _, bad := range []string{"notazip.txt", "a..zip", "sp%20ace.zip", "x.zip.exe", "uploads", "a%2Fb.zip"} {
		s.do(http.MethodDelete, "/api/backup/"+bad, nil, "").expect(t, 400, `{"error":"Invalid backup filename"}`)
		s.do(http.MethodGet, "/api/backup/download/"+bad, nil, "").expect(t, 400, `{"error":"Invalid backup filename"}`)
	}
	s.do(http.MethodDelete, "/api/backup/uploads/a..zip", nil, "").expect(t, 400, `{"error":"Invalid filename"}`)
	s.do(http.MethodDelete, "/api/backup/uploads/missing.zip", nil, "").expect(t, 404, `{"error":"File not found"}`)
	// A directory named like a backup cannot be deleted through the API.
	if err := os.Mkdir(filepath.Join(s.d.Config.BackupPath, "dir.zip"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.do(http.MethodDelete, "/api/backup/dir.zip", nil, "").expect(t, 500, `{"error":"Failed to delete backup"}`)
	s.do(http.MethodGet, "/api/backup/download/dir.zip", nil, "").expect(t, 500, `{"error":"Failed to download backup"}`)

	t.Run("links out of the backup directory are refused", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "secret.zip")
		if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(s.d.Config.BackupPath, "link.zip")); err != nil {
			t.Skip("symlinks not supported")
		}
		s.do(http.MethodGet, "/api/backup/download/link.zip", nil, "").expect(t, 400, `{"error":"Invalid backup filename"}`)
		s.do(http.MethodDelete, "/api/backup/link.zip", nil, "").expect(t, 400, `{"error":"Invalid backup filename"}`)
		s.json(http.MethodPost, "/api/backup/restore/link.zip", map[string]any{"password": s.password}).expect(t, 400, `{"error":"Invalid backup filename"}`)
		if _, err := os.Stat(outside); err != nil {
			t.Fatal("target must survive")
		}
	})
}

func TestCreateNameHandling(t *testing.T) {
	s := newServer(t, true)
	for _, tc := range []struct {
		body   any
		prefix string
	}{
		{nil, "docklift-backup-"},
		{map[string]any{"name": ""}, "docklift-backup-"},
		{map[string]any{"name": false}, "docklift-backup-"},
		{map[string]any{"name": "my backup!"}, "my-backup--backup-"},
	} {
		r := s.json(http.MethodPost, "/api/backup/create", tc.body)
		if !strings.Contains(string(r.body), "  File: "+tc.prefix) {
			t.Fatalf("%v: %s", tc.body, r.body)
		}
	}
	for _, bad := range []any{12, true, []any{"x"}, map[string]any{}} {
		r := s.json(http.MethodPost, "/api/backup/create", map[string]any{"name": bad})
		if r.status != 200 || !strings.Contains(string(r.body), "\n[ERROR] Backup failed: name must be a string\n") {
			t.Fatalf("%v: %d %s", bad, r.status, r.body)
		}
	}
}

// Deliberate fix 1: upload mistakes are 400 with a clear message.
func TestUploadErrors(t *testing.T) {
	s := newServer(t, true)
	data := zipBytes(t, map[string]string{"nginx-conf/x.conf": "x"})
	body, ct := multipartBody(t, []filePart{{"backup", "../../evil name.zip", "application/zip", data}})
	r := s.do(http.MethodPost, "/api/backup/upload", body, ct)
	m := r.json(t)
	if r.status != 200 || !regexp.MustCompile(`^restore-\d+-[0-9a-f]{16}\.zip$`).MatchString(m["filename"].(string)) ||
		m["size"].(float64) != float64(len(data)) || m["success"] != true || m["message"] != "Backup uploaded successfully" {
		t.Fatalf("upload %d %s", r.status, r.body)
	}
	if b, err := os.ReadFile(filepath.Join(s.d.Config.BackupPath, "uploads", m["filename"].(string))); err != nil || string(b) != data {
		t.Fatalf("stored upload: %v", err)
	}
	body, ct = multipartBody(t, []filePart{{"backup", "noext", "application/zip", data}})
	if r := s.do(http.MethodPost, "/api/backup/upload", body, ct); r.status != 200 {
		t.Fatalf("application/zip without .zip name: %d %s", r.status, r.body)
	}

	for _, tc := range []struct {
		name  string
		files []filePart
		want  string
	}{
		{"wrong type", []filePart{{"backup", "x.txt", "text/plain", "x"}}, `{"error":"Only ZIP files are allowed"}`},
		{"wrong field", []filePart{{"file", "x.zip", "application/zip", data}}, `{"error":"Unexpected file field"}`},
		{"two files", []filePart{{"backup", "a.zip", "", data}, {"backup", "b.zip", "", data}}, `{"error":"Unexpected file field"}`},
		{"no file", nil, `{"error":"No backup file uploaded"}`},
	} {
		body, ct := multipartBody(t, tc.files, "a", "b")
		s.do(http.MethodPost, "/api/backup/upload", body, ct).expect(t, 400, tc.want)
	}
	s.do(http.MethodPost, "/api/backup/upload", strings.NewReader("--x--"), "multipart/form-data").expect(t, 400, `{"error":"Multipart: Boundary not found"}`)
	s.do(http.MethodPost, "/api/backup/upload", strings.NewReader("--b\r\nContent-Disposition: form-data; name=\"backup\"; filename=\"a.zip\"\r\n\r\nabc"), "multipart/form-data; boundary=b").
		expect(t, 400, `{"error":"Unexpected end of form"}`)
	s.json(http.MethodPost, "/api/backup/upload", map[string]any{"x": 1}).expect(t, 400, `{"error":"No backup file uploaded"}`)

	entries, _ := os.ReadDir(filepath.Join(s.d.Config.BackupPath, "uploads"))
	if len(entries) != 2 {
		t.Fatalf("only the two accepted uploads may be stored, got %d", len(entries))
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".partial") {
			t.Fatalf("partial upload left: %s", e.Name())
		}
	}
}

func zipBytes(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = io.WriteString(w, body)
	}
	_ = zw.Close()
	return buf.String()
}

// lockFree reports whether a new restore could take the lock (and gives it back).
func (s *server) lockFree() bool {
	if !s.d.Maintenance.TryAcquireRestoreLock("probe") {
		return false
	}
	s.d.Maintenance.ReleaseRestoreLock()
	return true
}

func TestRestoreValidationReleasesTheLock(t *testing.T) {
	s := newServer(t, true)
	needPw := `{"error":"Password confirmation required for this operation","requirePassword":true}`
	badPw := `{"error":"Invalid password","requirePassword":true}`
	for _, base := range []string{"/api/backup/restore/", "/api/backup/restore-from-upload/"} {
		s.json(http.MethodPost, base+"x.zip", nil).expect(t, 403, needPw)
		s.json(http.MethodPost, base+"x.txt", map[string]any{"password": "wrong"}).expect(t, 403, badPw)
		s.json(http.MethodPost, base+"x.txt", map[string]any{"password": s.password}).expect(t, 400, `{"error":"Invalid backup filename"}`)
	}
	s.json(http.MethodPost, "/api/backup/restore/missing.zip", map[string]any{"password": s.password}).expect(t, 404, `{"error":"Backup file not found"}`)
	s.json(http.MethodPost, "/api/backup/restore-from-upload/missing.zip", map[string]any{"password": s.password}).expect(t, 404, `{"error":"Uploaded file not found"}`)

	data := zipBytes(t, map[string]string{"nginx-conf/x.conf": "x"})
	body, ct := multipartBody(t, []filePart{{"backup", "r.zip", "application/zip", data}})
	s.do(http.MethodPost, "/api/backup/restore-upload", body, ct).expect(t, 403, needPw)
	body, ct = multipartBody(t, []filePart{{"backup", "r.zip", "application/zip", data}}, "password", "wrong")
	s.do(http.MethodPost, "/api/backup/restore-upload", body, ct).expect(t, 403, badPw)
	body, ct = multipartBody(t, nil, "password", s.password)
	s.do(http.MethodPost, "/api/backup/restore-upload", body, ct).expect(t, 400, `{"error":"No backup file uploaded"}`)
	body, ct = multipartBody(t, []filePart{{"backup", "r.txt", "text/plain", "x"}})
	s.do(http.MethodPost, "/api/backup/restore-upload", body, ct).expect(t, 400, `{"error":"Only ZIP files are allowed"}`)
	s.json(http.MethodPost, "/api/backup/restore-upload", map[string]any{"password": s.password}).expect(t, 400, `{"error":"No backup file uploaded"}`)

	if !s.lockFree() || s.d.Maintenance.Active() {
		t.Fatal("rejected restores must release the lock and never enter maintenance")
	}
	if entries, _ := os.ReadDir(filepath.Join(s.d.Config.BackupPath, "uploads")); len(entries) != 0 {
		t.Fatalf("rejected restore uploads must be deleted: %v", entries)
	}
}

func TestRestoreFromUploadCommitsAndRestarts(t *testing.T) {
	s := newServer(t, true)
	name, _ := s.createBackup()
	archive := filepath.Join(s.d.Config.BackupPath, name)
	body, ct := multipartBody(t, []filePart{{"backup", "b.zip", "application/zip", readFile(t, archive)}})
	up := s.do(http.MethodPost, "/api/backup/upload", body, ct).json(t)
	uploaded := up["filename"].(string)

	r := s.json(http.MethodPost, "/api/backup/restore-from-upload/"+uploaded, map[string]any{"password": s.password})
	log := string(r.body)
	if r.status != 200 || !strings.Contains(log, "  RESTORING FROM UPLOADED FILE\n") || !strings.Contains(log, "  RESTORE COMPLETE\n") ||
		!strings.Contains(log, "      + Marked as restored: "+strings.TrimSuffix(uploaded, ".zip")+".restored-") {
		t.Fatalf("restore %d:\n%s", r.status, log)
	}
	select {
	case <-s.restarts:
	case <-time.After(5 * time.Second):
		t.Fatal("a committed restore must restart the process")
	}
	// Until the restart: maintenance on, lock held (409 with a sentence — fix 7).
	if !s.d.Maintenance.Active() || s.d.Maintenance.Reason() != "Restoring from uploaded backup" {
		t.Fatal("maintenance must stay on until the restart")
	}
	s.do(http.MethodGet, "/api/backup", nil, "").expect(t, 503, `{"error":"Restoring from uploaded backup","maintenance":true}`)
	s.json(http.MethodPost, "/api/backup/restore/x.zip", map[string]any{"password": s.password}).
		expect(t, 409, `{"error":"A restore is already in progress. Wait for it to finish before starting another one.","locked":true}`)
}

func TestRestoreUploadWithSetupToken(t *testing.T) {
	// The archive comes from an installation with an admin.
	src := newServer(t, true)
	name, _ := src.createBackup()
	data := readFile(t, filepath.Join(src.d.Config.BackupPath, name))

	fresh := newServer(t, false)
	b := fresh.d.Auth.Bootstrap()
	token, err := b.EnsureSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureSecret(); err != nil {
		t.Fatal(err)
	}
	t.Run("failed reconcile keeps the setup secrets", func(t *testing.T) {
		fresh.nginx.err = fmt.Errorf("nginx down")
		defer func() { fresh.nginx.err = nil }()
		body, ct := multipartBody(t, []filePart{{"backup", "fresh.zip", "application/zip", data}})
		r := fresh.do(http.MethodPost, "/api/backup/restore-upload", body, ct, auth.SetupTokenHeader, token)
		mustContain(t, string(r.body), "[ROLLBACK] Restore not committed: Reconciliation incomplete", "[security] Setup credentials retained for retry", "[ERROR] RESTORE ABORTED")
		if !b.ValidateSetupToken(token) || fresh.d.Maintenance.Active() || !fresh.lockFree() {
			t.Fatal("aborted setup restore must keep the token and release maintenance and the lock")
		}
		if n, _ := fresh.d.Store.CountUsers(context.Background()); n != 0 {
			t.Fatalf("database must be rolled back, %d users", n)
		}
	})
	t.Run("success consumes them", func(t *testing.T) {
		body, ct := multipartBody(t, []filePart{{"backup", "fresh.zip", "application/zip", data}})
		r := fresh.do(http.MethodPost, "/api/backup/restore-upload", body, ct, auth.SetupTokenHeader, token)
		mustContain(t, string(r.body), "  RESTORING FROM UPLOADED BACKUP\n", "  [security] Setup token + bootstrap secret consumed\n", "  RESTORE COMPLETE\n")
		if b.ValidateSetupToken(token) || b.ReadSecret() != "" {
			t.Fatal("setup token and bootstrap secret must be consumed")
		}
	})
	t.Run("refused once an admin exists", func(t *testing.T) {
		s := newServer(t, true)
		s.token = ""
		tok, _ := s.d.Auth.Bootstrap().EnsureSetupToken()
		body, ct := multipartBody(t, []filePart{{"backup", "late.zip", "application/zip", data}})
		s.do(http.MethodPost, "/api/backup/restore-upload", body, ct, auth.SetupTokenHeader, tok).
			expect(t, 403, `{"error":"Setup-token restore is only allowed before the first admin exists"}`)
		if entries, _ := os.ReadDir(filepath.Join(s.d.Config.BackupPath, "uploads")); len(entries) != 0 || !s.lockFree() {
			t.Fatalf("refused upload must be deleted and the lock released: %v", entries)
		}
	})
}

func TestCriticalEndpoints(t *testing.T) {
	s := newServer(t, true)
	s.do(http.MethodGet, "/api/backup/critical-status", nil, "").expect(t, 200, `{"critical":false,"seal":null}`)
	s.json(http.MethodPost, "/api/backup/clear-critical-restore", map[string]any{"password": "x"}).expect(t, 400, `{"error":"No critical restore seal is active"}`)
	if err := s.d.Maintenance.EnterCritical("rollback failed", s.d.Config.DatabasePath); err != nil {
		t.Fatal(err)
	}
	st := s.do(http.MethodGet, "/api/backup/critical-status", nil, "").json(t)
	seal, _ := st["seal"].(map[string]any)
	if st["critical"] != true || seal["detail"] != "rollback failed" || seal["preRestorePath"] != s.d.Config.DatabasePath+".pre-restore" {
		t.Fatalf("status %v", st)
	}
	s.json(http.MethodGet, "/api/backup", nil).expectStatus(t, 503)
	s.json(http.MethodPost, "/api/backup/clear-critical-restore", nil).expect(t, 403, `{"error":"Password confirmation required for this operation","requirePassword":true}`)
	r := s.json(http.MethodPost, "/api/backup/clear-critical-restore", map[string]any{"password": s.password})
	m := r.json(t)
	if r.status != 200 || m["success"] != true || m["message"] != "Critical restore seal cleared" || m["previous"].(map[string]any)["detail"] != "rollback failed" {
		t.Fatalf("clear %d %s", r.status, r.body)
	}
	if s.d.Maintenance.Critical() || s.d.Maintenance.Active() || !s.lockFree() {
		t.Fatal("clearing must release maintenance and the lock")
	}
}

func (r resp) expectStatus(t *testing.T, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status %d (%s), want %d", r.status, r.body, status)
	}
}

// The boot hook seals an instance whose last restore was interrupted.
func TestBootHookSealsInterruptedRestore(t *testing.T) {
	s := newServer(t, true)
	marker := filepath.Join(s.d.Config.DataPath, svc.InProgressMarkerName)
	if err := os.WriteFile(marker, []byte(`{"at":"2026-01-02T03:04:05.000Z","stage":"nginx-conf","source":"b.zip"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(s.d.Config.BackupPath, "temp-restore-1")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.d.Lifecycle.Run(context.Background(), app.PhaseDockerNetwork); err != nil {
		t.Fatal(err)
	}
	if !s.d.Maintenance.Critical() || !strings.Contains(s.d.Maintenance.ReadCritical().Detail, "interrupted during nginx-conf") {
		t.Fatal("interrupted restore must seal at boot")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("marker must be replaced by the seal")
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("extraction leftovers must be removed at boot")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustContain(t *testing.T, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Fatalf("missing %q in:\n%s", p, s)
		}
	}
}
