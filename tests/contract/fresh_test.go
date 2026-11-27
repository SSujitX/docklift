package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// restoreZip is a backup archive in the 2.1.6 layout (database/docklift.db + deployments/).
func restoreZip(t *testing.T, withDB bool) []byte {
	t.Helper()
	var entries []zipEntry
	if withDB {
		raw, err := seedDBBytes()
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, zipEntry{Name: "database/docklift.db", Body: string(raw)})
	}
	entries = append(entries, zipEntry{Name: "nginx-conf/README.txt", Body: "restored by contract tests\n"})
	return buildZip(t, entries...)
}

func backupUpload(data []byte, filename string, fields ...string) *Multipart {
	mp := &Multipart{Files: []FilePart{{Field: "backup", Filename: filename, ContentType: "application/zip", Data: data}}}
	for i := 0; i+1 < len(fields); i += 2 {
		mp.Fields = append(mp.Fields, [2]string{fields[i], fields[i+1]})
	}
	return mp
}

func expectContains(t *testing.T, label, body string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Fatalf("%s: missing %q in:\n%s", label, p, truncate(body, 4000))
		}
	}
}

// TestFreshInstallFlow walks a brand-new install: bootstrap secret, setup
// token, setup-token restore (aborted without Docker), first registration and
// the account endpoints that would disturb the shared admin.
func TestFreshInstallFlow(t *testing.T) {
	inst := dedicatedInstance(t, BootOptions{Name: "fresh"})
	anon := inst.Anon()
	bootstrapPath := filepath.Join(inst.DataDir, ".bootstrap-secret")
	var secret, setupToken string
	var sess *Session

	steps := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{"status before setup", func(t *testing.T) {
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": false, "userCount": 0, "bootstrapRequired": true})
			b, err := os.ReadFile(bootstrapPath)
			if err != nil {
				t.Fatalf("bootstrap secret file: %v", err)
			}
			secret = strings.TrimSpace(string(b))
			match(t, "bootstrap secret", Hex(64), secret)
			// The secret is never exposed by a public API.
			for _, p := range []string{"/api/auth/status", "/api/health"} {
				if strings.Contains(string(anon.Get(t, p).Body), secret) {
					t.Fatalf("%s leaks the bootstrap secret", p)
				}
			}
		}},
		{"setup token requires bootstrap secret", func(t *testing.T) {
			c := inst.Anon()
			c.Get(t, "/api/auth/setup-token").ExpectError(t, 403, bootstrapRequiredMsg)
			c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", "wrong").ExpectError(t, 403, bootstrapRequiredMsg)
			c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", secret+"0").ExpectError(t, 403, bootstrapRequiredMsg)
			tok := c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", "  "+secret+"  ").
				Expect(t, 200, Obj{"setupToken": Hex(64)}).(map[string]any)["setupToken"].(string)
			// Stable until consumed.
			c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", secret).Expect(t, 200, Obj{"setupToken": tok})
			disk := strings.TrimSpace(string(readFileBytes(t, filepath.Join(inst.DataDir, ".setup-token"))))
			if disk != tok {
				t.Fatalf(".setup-token on disk = %q, API = %q", disk, tok)
			}
			setupToken = tok
		}},
		{"restore-upload auth rules before setup", func(t *testing.T) {
			c := inst.Anon()
			zipData := restoreZip(t, false)
			c.Post(t, "/api/backup/restore-upload", backupUpload(zipData, "b.zip")).ExpectError(t, 401, "Authentication required")
			c.Post(t, "/api/backup/restore-upload", backupUpload(zipData, "b.zip"), "X-Setup-Token", setupToken+"0").ExpectError(t, 401, "Authentication required")
			c.Post(t, "/api/backup/restore-upload", backupUpload(zipData, "b.zip"), "X-Setup-Token", "").ExpectError(t, 401, "Authentication required")
			// Other backup routes never accept the setup token.
			c.Get(t, "/api/backup", "X-Setup-Token", setupToken).ExpectError(t, 401, "Authentication required")
			c.Post(t, "/api/backup/upload", backupUpload(zipData, "b.zip"), "X-Setup-Token", setupToken).ExpectError(t, 401, "Authentication required")
			// Valid setup token: request validation.
			c.Post(t, "/api/backup/restore-upload", &Multipart{Fields: [][2]string{{"x", "y"}}}, "X-Setup-Token", setupToken).
				ExpectError(t, 400, "No backup file uploaded")
			c.Post(t, "/api/backup/restore-upload", &Multipart{Files: []FilePart{{Field: "backup", Filename: "notes.txt", ContentType: "text/plain", Data: []byte("x")}}}, "X-Setup-Token", setupToken).
				ExpectError(t, 400, "Only ZIP files are allowed")
			c.Post(t, "/api/backup/restore-upload", &Multipart{Files: []FilePart{{Field: "other", Filename: "b.zip", Data: zipData}}}, "X-Setup-Token", setupToken).
				ExpectError(t, 400, "Unexpected file field")
		}},
		{"setup-token restore without an admin in the archive aborts", func(t *testing.T) {
			requireNoDocker(t)
			r := inst.Anon().Post(t, "/api/backup/restore-upload", backupUpload(restoreZip(t, false), "fresh.zip"), "X-Setup-Token", setupToken)
			r.ExpectStatus(t, 200)
			if ct := r.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			expectContains(t, "restore log", string(r.Body),
				"RESTORING FROM UPLOADED BACKUP",
				"! No database in backup",
				"[ROLLBACK] Restore not committed: Restored database has no administrator (role=admin)",
				"[security] Setup credentials retained for retry",
				"[ERROR] RESTORE ABORTED")
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": false, "userCount": 0, "bootstrapRequired": true})
		}},
		{"setup-token restore rolls the database back when reconcile fails", func(t *testing.T) {
			requireNoDocker(t)
			r := inst.Anon().Post(t, "/api/backup/restore-upload", backupUpload(restoreZip(t, true), "fresh.zip"), "X-Setup-Token", setupToken)
			r.ExpectStatus(t, 200)
			expectContains(t, "restore log", string(r.Body),
				"+ Database restored",
				"[ROLLBACK] Restore not committed: Reconciliation incomplete",
				"+ Rolled database back from .pre-restore",
				"[security] Setup credentials retained for retry",
				"[ERROR] RESTORE ABORTED")
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": false, "userCount": 0, "bootstrapRequired": true})
			// Setup token and bootstrap secret survive for a retry.
			inst.Anon().Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", secret).Expect(t, 200, Obj{"setupToken": setupToken})
			// The maintenance gate is released after the aborted restore.
			anon.Get(t, "/api/projects").ExpectError(t, 401, "Authentication required")
		}},
		{"register validation", func(t *testing.T) {
			c := inst.Anon()
			body := map[string]any{"name": "Fresh Admin", "email": "Fresh.Admin@Contract.Test", "password": "FreshPassw0rd!"}
			c.Post(t, "/api/auth/register", map[string]any{"name": "a", "email": "b@c.d"}).ExpectError(t, 400, "Name, email, and password are required")
			c.Post(t, "/api/auth/register", map[string]any{"name": "a", "email": "b@c.d", "password": "1234567"}).ExpectError(t, 400, "Password must be at least 8 characters")
			c.Post(t, "/api/auth/register", body).ExpectError(t, 403, bootstrapRequiredMsg)
			c.Post(t, "/api/auth/register", body, "X-Bootstrap-Secret", "nope").ExpectError(t, 403, bootstrapRequiredMsg)
			withBody := map[string]any{"name": "Fresh Admin", "email": "x@y.z", "password": "FreshPassw0rd!", "bootstrapSecret": secret}
			// Header wins over body when both are present.
			c.Post(t, "/api/auth/register", withBody, "X-Bootstrap-Secret", "nope").ExpectError(t, 403, bootstrapRequiredMsg)
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": false, "userCount": 0, "bootstrapRequired": true})
		}},
		{"register first admin", func(t *testing.T) {
			c := inst.Anon()
			r := c.Post(t, "/api/auth/register", map[string]any{"name": "Fresh Admin", "email": "Fresh.Admin@Contract.Test", "password": "FreshPassw0rd!"},
				"X-Bootstrap-Secret", secret)
			got := r.Expect(t, 201, Obj{"message": "Registration successful", "token": JWT, "user": Obj{
				"id": UUID, "name": "Fresh Admin", "email": "fresh.admin@contract.test", "role": "admin",
			}}).(map[string]any)
			user := got["user"].(map[string]any)
			sess = &Session{Inst: inst, UserID: user["id"].(string), Name: "Fresh Admin", Email: "fresh.admin@contract.test", Password: "FreshPassw0rd!", Token: got["token"].(string)}
			sessionClaims(t, sess.Token)
			if _, err := os.Stat(bootstrapPath); !os.IsNotExist(err) {
				t.Fatalf("bootstrap secret must be consumed after registration (stat err=%v)", err)
			}
			matches, _ := filepath.Glob(filepath.Join(inst.DataDir, ".claimed-*"))
			if len(matches) != 0 {
				t.Fatalf("leftover claim files: %v", matches)
			}
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": true, "userCount": 1, "bootstrapRequired": false})
			sess.Client().Get(t, "/api/auth/me").Expect(t, 200, Obj{"user": Obj{
				"id": sess.UserID, "name": "Fresh Admin", "email": sess.Email, "role": "admin", "created_at": ISOms,
			}})
		}},
		{"setup surfaces close after registration", func(t *testing.T) {
			c := inst.Anon()
			c.Post(t, "/api/auth/register", map[string]any{"name": "B", "email": "b@contract.test", "password": "longenough"}, "X-Bootstrap-Secret", secret).
				ExpectError(t, 403, "Setup already complete. Use login instead.")
			c.Get(t, "/api/auth/setup-token", "X-Bootstrap-Secret", secret).
				ExpectError(t, 403, "Setup already complete. Setup tokens are only available before first user registration.")
			if targetIsGo() {
				// Go removes the setup token when the first admin registers
				// (deliberate fix), so it no longer authorises anything.
				if _, err := os.Stat(filepath.Join(inst.DataDir, ".setup-token")); !os.IsNotExist(err) {
					t.Fatalf(".setup-token must be removed after registration (stat err=%v)", err)
				}
				c.Post(t, "/api/backup/restore-upload", backupUpload(restoreZip(t, false), "late.zip"), "X-Setup-Token", setupToken).
					ExpectError(t, 401, "Authentication required")
			} else {
				// The setup token file outlives registration, but restore by setup token is refused once an admin exists.
				c.Post(t, "/api/backup/restore-upload", backupUpload(restoreZip(t, false), "late.zip"), "X-Setup-Token", setupToken).
					ExpectError(t, 403, "Setup-token restore is only allowed before the first admin exists")
			}
			// Uploaded files from the aborted restores were kept and marked as restored.
			ups := sess.Client().Get(t, "/api/backup/uploads").Arr(t)
			date := time.Now().UTC().Format("2006-01-02")
			restored := 0
			for _, u := range ups {
				match(t, "upload entry", Obj{"filename": Re(`^restore-\d+-[0-9a-f]{16}(\.restored-\d{4}-\d{2}-\d{2})?\.zip$`), "size": Int, "created_at": ISOms}, u)
				if strings.HasSuffix(u.(map[string]any)["filename"].(string), ".restored-"+date+".zip") {
					restored++
				}
			}
			if !cfg.docker && restored < 2 {
				t.Fatalf("want the two aborted uploads marked .restored-%s.zip, got %v", date, ups)
			}
		}},
		{"profile update", func(t *testing.T) {
			c := sess.Client()
			c.Patch(t, "/api/auth/profile", map[string]any{"name": "Only Name"}).ExpectError(t, 400, "Name and email are required")
			c.Patch(t, "/api/auth/profile", map[string]any{"email": "a@b.c"}).ExpectError(t, 400, "Name and email are required")
			c.Patch(t, "/api/auth/profile", map[string]any{"name": "Renamed Admin", "email": "Renamed@Contract.Test"}).
				Expect(t, 200, Obj{"message": "Profile updated", "user": Obj{"id": sess.UserID, "name": "Renamed Admin", "email": "renamed@contract.test", "role": "admin"}})
			sess.Email, sess.Name = "renamed@contract.test", "Renamed Admin"
			// The session token keeps working (email in the JWT is informational).
			c.Get(t, "/api/auth/me").Expect(t, 200, Obj{"user": Obj{"id": sess.UserID, "name": "Renamed Admin", "email": "renamed@contract.test", "role": "admin", "created_at": ISOms}})
			mustLogin(t, inst, "RENAMED@contract.test", sess.Password)
			inst.Anon().Post(t, "/api/auth/login", map[string]any{"email": "fresh.admin@contract.test", "password": sess.Password}).ExpectError(t, 401, "Invalid email or password")
			inst.Anon().Patch(t, "/api/auth/profile", map[string]any{"name": "x", "email": "y@z"}).ExpectError(t, 401, "Authentication required")
		}},
		{"change password invalidates older tokens", func(t *testing.T) {
			c := sess.Client()
			oldToken := sess.Token
			oldSSE := c.Post(t, "/api/auth/sse-token", nil).Obj(t)["token"].(string)
			oldTerminal := c.Post(t, "/api/auth/terminal-token", nil).Obj(t)["token"].(string)
			c.Post(t, "/api/auth/change-password", map[string]any{}).ExpectError(t, 400, "Current and new passwords are required")
			c.Post(t, "/api/auth/change-password", map[string]any{"currentPassword": sess.Password}).ExpectError(t, 400, "Current and new passwords are required")
			c.Post(t, "/api/auth/change-password", map[string]any{"currentPassword": sess.Password, "newPassword": "short"}).ExpectError(t, 400, "New password must be at least 8 characters")
			c.Post(t, "/api/auth/change-password", map[string]any{"currentPassword": "wrong-password", "newPassword": "BrandNewPassw0rd"}).ExpectError(t, 400, "Invalid current password")
			before := sessionClaims(t, oldToken)["pwdv"].(float64)
			time.Sleep(5 * time.Millisecond)
			got := c.Post(t, "/api/auth/change-password", map[string]any{"currentPassword": sess.Password, "newPassword": "BrandNewPassw0rd"}).
				Expect(t, 200, Obj{"message": "Password changed successfully", "token": JWT}).(map[string]any)
			newToken := got["token"].(string)
			if after := sessionClaims(t, newToken)["pwdv"].(float64); after <= before {
				t.Fatalf("pwdv must advance: before %v after %v", before, after)
			}
			old := inst.As(oldToken)
			old.Get(t, "/api/auth/me").ExpectError(t, 401, "Session expired. Please log in again.")
			old.Get(t, "/api/projects").ExpectError(t, 401, "Session expired. Please log in again.")
			inst.Anon().Get(t, "/api/system/logs/backend?token="+oldSSE).ExpectError(t, 401, "Session expired. Please log in again.")
			wsExpectStatus(t, inst, oldTerminal, inst.BaseURL, 401)
			inst.As(newToken).Get(t, "/api/auth/me").ExpectStatus(t, 200)
			inst.Anon().Post(t, "/api/auth/login", map[string]any{"email": sess.Email, "password": sess.Password}).ExpectError(t, 401, "Invalid email or password")
			sess.Password, sess.Token = "BrandNewPassw0rd", newToken
			relog := mustLogin(t, inst, sess.Email, sess.Password)
			if sessionClaims(t, relog.Token)["pwdv"] != sessionClaims(t, newToken)["pwdv"] {
				t.Fatalf("login after change must carry the new pwdv")
			}
		}},
		{"step-up uses the current password", func(t *testing.T) {
			c := sess.Client()
			c.Post(t, "/api/system/reboot", map[string]any{"password": "FreshPassw0rd!"}).
				Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
			c.Post(t, "/api/backup/restore/none.zip", map[string]any{"password": sess.Password}).ExpectError(t, 404, "Backup file not found")
		}},
	}
	for _, s := range steps {
		if !t.Run(s.name, s.fn) {
			return
		}
	}
}
