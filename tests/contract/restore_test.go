package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCriticalRestoreGate boots on a data dir that carries a .restore-critical
// seal (left by a failed DB rollback) and checks the maintenance gate.
func TestCriticalRestoreGate(t *testing.T) {
	seal := map[string]any{"at": "2026-01-02T03:04:05.678Z", "detail": "contract: simulated rollback failure", "liveDbPath": "/app/data/docklift.db", "preRestorePath": "/app/data/docklift.db.pre-restore"}
	inst := dedicatedInstance(t, BootOptions{Name: "critical", Prepare: func(d InstanceDirs) error {
		if err := prepareSeeded(d); err != nil {
			return err
		}
		b, _ := json.MarshalIndent(seal, "", "  ")
		return os.WriteFile(filepath.Join(d.Data, ".restore-critical"), b, 0o600)
	}})
	marker := filepath.Join(inst.DataDir, ".restore-critical")
	gate := Obj{"error": "CRITICAL restore recovery required — do not retry restore. See " + marker, "maintenance": true, "critical": true, "sealPath": marker}
	now := time.Now().Unix()
	token := signJWT(knownSecrets.JWT, map[string]any{"userId": seedAdminID, "email": seedAdminEmail, "role": "admin", "pwdv": seedAdminPwdv, "iat": now, "exp": now + 3600})
	c := inst.As(token)
	anon := inst.Anon()

	anon.Get(t, "/api/health").Expect(t, 200, Obj{"status": "ok", "timestamp": ISOms, "version": NonEmpty, "instanceId": UUID})
	for _, r := range []*Resp{
		anon.Get(t, "/api/auth/status"),
		anon.Post(t, "/api/auth/login", map[string]any{"email": seedAdminEmail, "password": seedAdminPassword}),
		c.Get(t, "/api/projects"),
		c.Get(t, "/api/system/stats"),
		anon.Get(t, "/api/nope"),
		anon.Post(t, "/api/github/webhook", map[string]any{}, "X-GitHub-Event", "ping"),
		c.Post(t, "/api/backup/restore-upload", backupUpload(restoreZip(t, true), "r.zip", "password", seedAdminPassword)),
		c.Post(t, "/api/backup/restore/x.zip", map[string]any{"password": seedAdminPassword}),
		c.Get(t, "/api/backup"),
	} {
		r.Expect(t, 503, gate)
	}
	// Recovery endpoints stay reachable (still authenticated).
	anon.Get(t, "/api/backup/critical-status").ExpectError(t, 401, "Authentication required")
	c.Get(t, "/api/backup/critical-status").Expect(t, 200, Obj{"critical": true, "seal": seal})
	if targetIsGo() {
		// The internal-secret bypass is removed on Go (deliberate fix).
		anon.With("X-Internal-Secret", knownSecrets.Internal).Get(t, "/api/backup/critical-status").ExpectError(t, 401, "Authentication required")
	} else {
		anon.With("X-Internal-Secret", knownSecrets.Internal).Get(t, "/api/backup/critical-status").Expect(t, 200, Obj{"critical": true, "seal": seal})
	}
	c.Post(t, "/api/backup/clear-critical-restore", nil).Expect(t, 403, Obj{"error": "Password confirmation required for this operation", "requirePassword": true})
	c.Post(t, "/api/backup/clear-critical-restore", map[string]any{"password": "wrong"}).Expect(t, 403, Obj{"error": "Invalid password", "requirePassword": true})
	internalMsg := "Unauthorized"
	if targetIsGo() {
		internalMsg = "Authentication required"
	}
	anon.With("X-Internal-Secret", knownSecrets.Internal).Post(t, "/api/backup/clear-critical-restore", map[string]any{"password": seedAdminPassword}).ExpectError(t, 401, internalMsg)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("seal must survive rejected clears: %v", err)
	}

	c.Post(t, "/api/backup/clear-critical-restore", map[string]any{"password": seedAdminPassword}).
		Expect(t, 200, Obj{"success": true, "message": "Critical restore seal cleared", "previous": seal})
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("seal file must be deleted (stat err=%v)", err)
	}
	anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": true, "userCount": 1, "bootstrapRequired": false})
	c.Get(t, "/api/backup/critical-status").Expect(t, 200, Obj{"critical": false, "seal": nil})
	c.Post(t, "/api/backup/clear-critical-restore", map[string]any{"password": seedAdminPassword}).ExpectError(t, 400, "No critical restore seal is active")
	// The restore lock is released with the seal.
	c.Post(t, "/api/backup/restore/missing.zip", map[string]any{"password": seedAdminPassword}).ExpectError(t, 404, "Backup file not found")
}

// TestAuthenticatedRestore runs a real restore-from-upload on a dedicated
// instance: maintenance gate and restore lock while it streams, process exit
// afterwards, and the restored data after a restart.
func TestAuthenticatedRestore(t *testing.T) {
	requireNoDocker(t)
	inst := dedicatedInstance(t, BootOptions{Name: "restore"})
	adm, err := ensureAdmin(inst)
	if err != nil {
		t.Fatal(err)
	}
	c := adm.Client()
	before := createProject(t, c, uniq("pre-restore"), dockerfileApp())
	healthBefore := inst.Anon().Get(t, "/api/health").Obj(t)

	archive := buildZip(t,
		zipEntry{Name: "database/docklift.db", Body: string(mustSeed(t))},
		zipEntry{Name: "deployments/" + seedApp + "/Dockerfile", Body: "FROM alpine:3.20\n"},
		zipEntry{Name: "deployments/" + seedApp + "/restored.txt", Body: "from backup\n"},
		zipEntry{Name: "nginx-conf/restored.contract-test.dev.conf", Body: "# restored by contract tests\n"},
	)
	up := c.Post(t, "/api/backup/upload", backupUpload(archive, "restore.zip")).Obj(t)
	name := up["filename"].(string)

	s := c.Open(t, "POST", "/api/backup/restore-from-upload/"+name, map[string]any{"password": adm.Password})
	if s.Status != 200 {
		t.Fatalf("restore status %d: %s", s.Status, s.ReadAll())
	}
	if _, ok := s.ReadUntil("RESTORING FROM UPLOADED FILE", 30*time.Second); !ok {
		t.Fatal("restore stream did not start")
	}
	// While the restore runs: API gated, restore routes reach the lock. In TS
	// the 409 error is the lock owner label (restoreLockReason returns the "why"
	// string); Go returns a sentence (deliberate fix 7).
	c.Get(t, "/api/projects").Expect(t, 503, Obj{"error": "Restoring from uploaded backup", "maintenance": true})
	inst.Anon().Get(t, "/api/auth/status").Expect(t, 503, Obj{"error": "Restoring from uploaded backup", "maintenance": true})
	inst.Anon().Get(t, "/api/health").ExpectStatus(t, 200)
	locked := Obj{"error": "restore-from-upload", "locked": true}
	if targetIsGo() {
		locked = Obj{"error": "A restore is already in progress. Wait for it to finish before starting another one.", "locked": true}
	}
	c.Post(t, "/api/backup/restore-upload", backupUpload(archive, "again.zip", "password", adm.Password)).Expect(t, 409, locked)
	c.Post(t, "/api/backup/restore/x.zip", map[string]any{"password": adm.Password}).Expect(t, 409, locked)

	log := s.ReadAll()
	expectContains(t, "restore log", log,
		"  File: "+name+"\n",
		"[3/7] Restoring database...\n", "      + Created backup of current database\n", "      + Database restored\n",
		"      + Deployments restored\n", "      + Nginx configs restored\n",
		"      + Marked as restored: "+strings.TrimSuffix(name, ".zip")+".restored-",
		"  RECONCILING SYSTEM STATE\n",
		"[ERROR] RESTORE INCOMPLETE — data restored but some projects failed to start\n",
		"  [!] Restarting backend service to apply changes...\n")
	if strings.Contains(log, "RESTORE ABORTED") || strings.Contains(log, "Setup token + bootstrap secret consumed") {
		t.Fatalf("authenticated restore must commit without consuming setup secrets:\n%s", log)
	}
	if !inst.WaitExit(20 * time.Second) {
		t.Fatal("server must exit after a committed restore")
	}
	if err := inst.Restart(); err != nil {
		t.Fatal(err)
	}

	health := inst.Anon().Get(t, "/api/health").Obj(t)
	if health["instanceId"] == healthBefore["instanceId"] {
		t.Fatal("instanceId must change across restarts")
	}
	inst.As(adm.Token).Get(t, "/api/auth/me").ExpectError(t, 401, "User not found")
	inst.Anon().Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": true, "userCount": 1, "bootstrapRequired": false})
	seed := mustLogin(t, inst, seedAdminEmail, seedAdminPassword)
	sc := seed.Client()
	sc.Get(t, "/api/projects/"+before["id"].(string)).ExpectError(t, 404, "Project not found")
	sc.Get(t, "/api/files/"+seedApp+"/content?path=restored.txt").Expect(t, 200, Obj{"name": "restored.txt", "content": "from backup\n"})
	sc.Get(t, "/api/deployments/"+seedApp).Expect(t, 200, ArrayOf(AnyObj, 4))
	if _, err := os.Stat(filepath.Join(inst.Dirs.Nginx, "restored.contract-test.dev.conf")); err != nil {
		t.Fatalf("nginx conf not restored: %v", err)
	}
	if _, err := os.Stat(inst.Dirs.DBFile + ".pre-restore"); err != nil {
		t.Fatalf("pre-restore DB copy must be kept: %v", err)
	}
	restored := false
	for _, u := range sc.Get(t, "/api/backup/uploads").Arr(t) {
		if strings.HasPrefix(u.(map[string]any)["filename"].(string), strings.TrimSuffix(name, ".zip")+".restored-") {
			restored = true
		}
	}
	if !restored {
		t.Fatal("uploaded archive must be kept as .restored-<date>.zip")
	}
}

func mustSeed(t *testing.T) []byte {
	t.Helper()
	b, err := seedDBBytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
