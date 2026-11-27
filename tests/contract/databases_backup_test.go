package contract

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
)

func TestDatabaseEngines(t *testing.T) {
	c := admin.Client()
	version := Obj{"tag": Str, "label": Str, "recommended": Optional(true)}
	engine := func(id, label, repo string, port int, mount, envKey string) Obj {
		return Obj{"id": id, "label": label, "description": NonEmpty, "imageRepo": repo, "versions": ArrayOf(version, 1),
			"image": Prefix(repo + ":"), "port": port, "volumeMount": mount, "defaultEnvKey": envKey, "serviceName": "db"}
	}
	// Versions come from Docker Hub when reachable, else the static catalog.
	c.Get(t, "/api/databases/engines").Expect(t, 200, Arr{
		engine("postgres", "PostgreSQL", "postgres", 5432, "/var/lib/postgresql/data", "DATABASE_URL"),
		engine("mysql", "MySQL", "mysql", 3306, "/var/lib/mysql", "DATABASE_URL"),
		engine("mariadb", "MariaDB", "mariadb", 3306, "/var/lib/mysql", "DATABASE_URL"),
		engine("redis", "Redis", "redis", 6379, "/data", "REDIS_URL"),
		engine("mongodb", "MongoDB", "mongo", 27017, "/data/db", "MONGODB_URI"),
	})
	mainInst.Anon().Get(t, "/api/databases/engines").ExpectError(t, 401, "Authentication required")
}

func TestDatabaseValidationWithoutDocker(t *testing.T) {
	c := admin.Client()
	c.Post(t, "/api/databases", map[string]any{}).ExpectError(t, 400, "Name is required (max 120 characters)")
	c.Post(t, "/api/databases", map[string]any{"name": strings.Repeat("d", 121), "engine": "postgres"}).ExpectError(t, 400, "Name is required (max 120 characters)")
	for _, e := range []any{nil, "oracle", "Postgres", 1} {
		c.Post(t, "/api/databases", map[string]any{"name": "db", "engine": e}).ExpectError(t, 400, "Invalid engine. Use one of: postgres, mysql, mariadb, redis, mongodb")
	}
	c.Post(t, "/api/databases", map[string]any{"name": "db", "engine": "postgres", "version": "not a tag!"}).
		ExpectError(t, 400, Prefix("Invalid version for PostgreSQL. Try one of: "))
	c.Post(t, "/api/databases", map[string]any{"name": "db", "engine": "redis", "tag": "mysql:8"}).
		ExpectError(t, 400, Prefix("Invalid version for Redis. Try one of: "))

	c.Get(t, "/api/databases/links/by-app/"+missingProjectID).Expect(t, 200, Arr{})
	c.Get(t, "/api/databases/"+missingProjectID+"/connection").ExpectError(t, 404, "Managed database not found or credentials missing")
	c.Get(t, "/api/databases/"+missingProjectID+"/links").ExpectError(t, 404, "Database not found")
	c.Post(t, "/api/databases/"+missingProjectID+"/links", map[string]any{"app_project_id": "x"}).ExpectError(t, 404, "Database not found")
	c.Delete(t, "/api/databases/"+missingProjectID+"/links/x").ExpectError(t, 404, "Link not found")
	app := createProject(t, c, uniq("dbapp"), dockerfileApp())
	c.Get(t, "/api/databases/"+app["id"].(string)+"/links").ExpectError(t, 404, "Database not found")
	c.Get(t, "/api/databases/links/by-app/"+app["id"].(string)).Expect(t, 200, Arr{})

	t.Run("create fails and rolls back without docker", func(t *testing.T) {
		requireNoDocker(t)
		name := uniq("db")
		create := c.Post(t, "/api/databases", map[string]any{"name": name, "engine": "postgres"})
		if targetIsGo() {
			// Deliberate fix: an unreachable daemon is a 503, not a 500 carrying docker's stderr.
			create.ExpectError(t, 503, "Docker is not available")
		} else {
			create.Expect(t, 500, Obj{"error": NonEmpty})
		}
		for _, d := range c.Get(t, "/api/databases").Arr(t) {
			if d.(map[string]any)["name"] == name {
				t.Fatal("failed database create must be rolled back")
			}
		}
		if projectNames(t, c)[name] {
			t.Fatal("failed database create left a project row")
		}
	})
	t.Run("create with docker", func(t *testing.T) {
		requireDocker(t)
		name := uniq("db")
		got := c.Post(t, "/api/databases", map[string]any{"name": name, "engine": "redis"}).Expect(t, 201, projectRow(Obj{
			"name": name, "description": Prefix("Redis "), "source_type": "managed", "project_type": "database", "db_engine": "redis",
			"build_type": "dockerfile", "internal_port": 6379, "auto_deploy": false,
			"services":           ArrayOf(serviceRow(Obj{"name": "db", "internal_port": 6379, "port": nil, "status": "pending"}), 1),
			"persistent_volumes": ArrayOf(Obj{"id": UUID, "project_id": UUID, "service_name": "db", "name": Prefix("dl-"), "display_name": "data", "mount_path": "/data", "created_at": ISOms}, 1),
			"engine":             AnyObj, "image": Prefix("redis:"), "version": Str, "connection_url": Prefix("redis://:"),
			"credentials": Obj{"username": "docklift", "database": Str, "password": NonEmpty},
		})).(map[string]any)
		_ = got
	})
	c.Get(t, "/api/databases").Expect(t, 200, AnyArr)
	mainInst.Anon().Get(t, "/api/databases").ExpectError(t, 401, "Authentication required")
}

var backupFileRe = regexp.MustCompile(`  File: (\S+\.zip)\n`)

func createBackup(t *testing.T, c *Client, body any) (string, string) {
	t.Helper()
	r := c.Post(t, "/api/backup/create", body)
	r.ExpectStatus(t, 200)
	if ct := r.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("backup Content-Type = %q", ct)
	}
	log := string(r.Body)
	m := backupFileRe.FindStringSubmatch(log)
	if m == nil {
		t.Fatalf("backup log has no File line:\n%s", log)
	}
	return m[1], log
}

func TestBackupLifecycle(t *testing.T) {
	c := admin.Client()
	createProject(t, c, uniq("backup-src"), dockerfileApp())
	label := "contract test!" + randHex(2)
	name, log := createBackup(t, c, map[string]any{"name": label})
	expectContains(t, "backup log", log, "  CREATING BACKUP\n", "[1/4] Adding database...\n", "+ docklift.db (", "[consistent snapshot]",
		"[2/4] Adding deployments...\n", "[3/5] Adding nginx configs...\n", "[4/5] Adding TLS certificates...\n", "[5/5] Adding GitHub App key...\n",
		"      - No GitHub App key configured (skipped)\n", "  BACKUP COMPLETE\n")
	sanitized := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(label, "-")
	if !regexp.MustCompile(`^` + regexp.QuoteMeta(sanitized) + `-backup-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}\.zip$`).MatchString(name) {
		t.Fatalf("backup filename = %q (sanitized label %q)", name, sanitized)
	}

	list := c.Get(t, "/api/backup").Expect(t, 200, ArrayOf(Obj{"filename": Re(`\.zip$`), "size": Int, "created_at": ISOms}, 1)).([]any)
	entry := findBy(list, "filename", name)
	if entry == nil {
		t.Fatalf("new backup not listed: %v", short(list))
	}

	r := c.Get(t, "/api/backup/download/"+name)
	r.ExpectStatus(t, 200)
	for k, v := range map[string]string{"Content-Type": "application/zip", "Content-Disposition": `attachment; filename="` + name + `"`} {
		if got := r.Header.Get(k); got != v {
			t.Errorf("download %s = %q, want %q", k, got, v)
		}
	}
	if float64(len(r.Body)) != entry["size"].(float64) {
		t.Fatalf("download size %d != listed size %v", len(r.Body), entry["size"])
	}
	zr, err := zip.NewReader(bytes.NewReader(r.Body), int64(len(r.Body)))
	if err != nil {
		t.Fatalf("download is not a zip: %v", err)
	}
	seen := map[string]bool{}
	for _, f := range zr.File {
		top, _, _ := strings.Cut(f.Name, "/")
		seen[top] = true
		if f.Name == "database/docklift.db" {
			rc, _ := f.Open()
			head := make([]byte, 16)
			io.ReadFull(rc, head)
			rc.Close()
			if string(head) != "SQLite format 3\x00" {
				t.Fatalf("database/docklift.db is not SQLite: %q", head)
			}
			seen["db"] = true
		}
		if strings.HasPrefix(f.Name, "deployments/") && strings.HasSuffix(f.Name, "/Dockerfile") {
			seen["project-file"] = true
		}
	}
	for _, want := range []string{"db", "deployments", "project-file"} {
		if !seen[want] {
			t.Fatalf("backup zip lacks %s; top-level entries: %v", want, seen)
		}
	}

	c.Delete(t, "/api/backup/"+name).Expect(t, 200, Obj{"success": true, "message": "Backup deleted successfully"})
	c.Delete(t, "/api/backup/"+name).ExpectError(t, 404, "Backup not found")
	c.Get(t, "/api/backup/download/"+name).ExpectError(t, 404, "Backup not found")
	if findBy(c.Get(t, "/api/backup").Arr(t), "filename", name) != nil {
		t.Fatal("deleted backup still listed")
	}

	t.Run("default and long names", func(t *testing.T) {
		n, _ := createBackup(t, c, nil)
		if !strings.HasPrefix(n, "docklift-backup-") {
			t.Fatalf("default backup name = %q", n)
		}
		c.Delete(t, "/api/backup/"+n).ExpectStatus(t, 200)
		long := strings.Repeat("L", 60) + randHex(2)
		n, _ = createBackup(t, c, map[string]any{"name": long})
		if !strings.HasPrefix(n, strings.Repeat("L", 50)+"-backup-") {
			t.Fatalf("long name not truncated to 50: %q", n)
		}
		c.Delete(t, "/api/backup/"+n).ExpectStatus(t, 200)
	})
	t.Run("non-string name fails inside the stream", func(t *testing.T) {
		r := c.Post(t, "/api/backup/create", map[string]any{"name": 12})
		r.ExpectStatus(t, 200)
		expectContains(t, "backup log", string(r.Body), "\n[ERROR] Backup failed: ")
	})
}

func TestBackupFilenameValidation(t *testing.T) {
	c := admin.Client()
	for _, bad := range []string{"notazip.txt", "a..zip", "sp%20ace.zip", "x.zip.exe", "uploads", ".zip%00"} {
		c.Delete(t, "/api/backup/"+bad).ExpectError(t, 400, "Invalid backup filename")
		c.Get(t, "/api/backup/download/"+bad).ExpectError(t, 400, "Invalid backup filename")
	}
	c.Delete(t, "/api/backup/uploads/a..zip").ExpectError(t, 400, "Invalid filename")
	c.Delete(t, "/api/backup/uploads/x.txt").ExpectError(t, 400, "Invalid filename")
	c.Delete(t, "/api/backup/uploads/missing-"+randHex(3)+".zip").ExpectError(t, 404, "File not found")
	c.Get(t, "/api/backup/download/missing-"+randHex(3)+".zip").ExpectError(t, 404, "Backup not found")
	c.Get(t, "/api/backup/critical-status").Expect(t, 200, Obj{"critical": false, "seal": nil})
	c.Post(t, "/api/backup/clear-critical-restore", map[string]any{"password": "anything"}).ExpectError(t, 400, "No critical restore seal is active")
	for _, p := range []string{"/api/backup", "/api/backup/uploads", "/api/backup/critical-status", "/api/backup/download/x.zip"} {
		mainInst.Anon().Get(t, p).ExpectError(t, 401, "Authentication required")
	}
}

func TestBackupUploads(t *testing.T) {
	c := admin.Client()
	data := restoreZip(t, false)
	got := c.Post(t, "/api/backup/upload", backupUpload(data, "../../evil name.zip")).Expect(t, 200, Obj{
		"success": true, "filename": Re(`^restore-\d+-[0-9a-f]{16}\.zip$`), "size": len(data), "message": "Backup uploaded successfully",
	}).(map[string]any)
	name := got["filename"].(string)
	ups := c.Get(t, "/api/backup/uploads").Expect(t, 200, ArrayOf(Obj{"filename": Str, "size": Int, "created_at": ISOms}, 1)).([]any)
	match(t, "upload entry", Obj{"filename": name, "size": len(data), "created_at": ISOms}, findBy(ups, "filename", name))
	// Uploads are not server-created backups.
	if findBy(c.Get(t, "/api/backup").Arr(t), "filename", name) != nil {
		t.Fatal("uploaded file listed among backups")
	}
	// application/zip passes the filter regardless of the client filename.
	c.Post(t, "/api/backup/upload", &Multipart{Files: []FilePart{{Field: "backup", Filename: "noext", ContentType: "application/zip", Data: data}}}).
		Expect(t, 200, Obj{"success": true, "filename": Re(`\.zip$`), "size": len(data), "message": "Backup uploaded successfully"})
	c.Post(t, "/api/backup/upload", &Multipart{Fields: [][2]string{{"a", "b"}}}).ExpectError(t, 400, "No backup file uploaded")
	// Multer errors on this route reached the generic error handler in TS;
	// Go answers 400 with the multer message (deliberate fix 1).
	wrongType := c.Post(t, "/api/backup/upload", &Multipart{Files: []FilePart{{Field: "backup", Filename: "x.txt", ContentType: "text/plain", Data: []byte("x")}}})
	wrongField := c.Post(t, "/api/backup/upload", &Multipart{Files: []FilePart{{Field: "file", Filename: "x.zip", ContentType: "application/zip", Data: data}}})
	if targetIsGo() {
		wrongType.ExpectError(t, 400, "Only ZIP files are allowed")
		wrongField.ExpectError(t, 400, "Unexpected file field")
	} else {
		wrongType.ExpectError(t, 500, "Something went wrong!")
		wrongField.ExpectError(t, 500, "Something went wrong!")
	}

	c.Delete(t, "/api/backup/uploads/"+name).Expect(t, 200, Obj{"success": true, "message": "File deleted successfully"})
	c.Delete(t, "/api/backup/uploads/"+name).ExpectError(t, 404, "File not found")
}

// Restore entry points on the shared instance: only auth and validation paths
// (a real restore replaces the database and exits the process).
func TestBackupRestoreValidation(t *testing.T) {
	c := admin.Client()
	needPw := Obj{"error": "Password confirmation required for this operation", "requirePassword": true}
	badPw := Obj{"error": "Invalid password", "requirePassword": true}
	for _, base := range []string{"/api/backup/restore/", "/api/backup/restore-from-upload/"} {
		c.Post(t, base+"x.zip", nil).Expect(t, 403, needPw)
		c.Post(t, base+"x.zip", map[string]any{"password": "wrong"}).Expect(t, 403, badPw)
		// Step-up is checked before the filename.
		c.Post(t, base+"x.txt", map[string]any{"password": "wrong"}).Expect(t, 403, badPw)
		c.Post(t, base+"x.txt", map[string]any{"password": admin.Password}).ExpectError(t, 400, "Invalid backup filename")
		c.Post(t, base+"a..zip", map[string]any{"password": admin.Password}).ExpectError(t, 400, "Invalid backup filename")
		mainInst.Anon().Post(t, base+"x.zip", map[string]any{"password": admin.Password}).ExpectError(t, 401, "Authentication required")
	}
	c.Post(t, "/api/backup/restore/missing-"+randHex(3)+".zip", map[string]any{"password": admin.Password}).ExpectError(t, 404, "Backup file not found")
	c.Post(t, "/api/backup/restore-from-upload/missing-"+randHex(3)+".zip", map[string]any{"password": admin.Password}).ExpectError(t, 404, "Uploaded file not found")

	data := restoreZip(t, false)
	c.Post(t, "/api/backup/restore-upload", backupUpload(data, "r.zip")).Expect(t, 403, needPw)
	c.Post(t, "/api/backup/restore-upload", backupUpload(data, "r.zip", "password", "wrong")).Expect(t, 403, badPw)
	c.Post(t, "/api/backup/restore-upload", &Multipart{Fields: [][2]string{{"password", admin.Password}}}).ExpectError(t, 400, "No backup file uploaded")
	c.Post(t, "/api/backup/restore-upload", &Multipart{Files: []FilePart{{Field: "backup", Filename: "r.txt", ContentType: "text/plain", Data: []byte("x")}}}).
		ExpectError(t, 400, "Only ZIP files are allowed")
	// A JSON body is not multipart: the password is read from it, but there is no file.
	c.Post(t, "/api/backup/restore-upload", map[string]any{"password": admin.Password}).ExpectError(t, 400, "No backup file uploaded")
	mainInst.Anon().Post(t, "/api/backup/restore-upload", backupUpload(data, "r.zip", "password", admin.Password)).ExpectError(t, 401, "Authentication required")
	// Failed attempts released the lock and never entered maintenance.
	c.Get(t, "/api/projects").ExpectStatus(t, 200)
	// Rejected restore uploads are deleted, not kept.
	for _, u := range c.Get(t, "/api/backup/uploads").Arr(t) {
		if strings.Contains(u.(map[string]any)["filename"].(string), ".restored-") {
			t.Fatalf("validation failures must not leave restored uploads: %v", u)
		}
	}
}
