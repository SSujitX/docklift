package projects

import (
	"context"
	"strings"
	"testing"

	"docklift/internal/store"
)

func TestEnvCreateAndList(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.project(store.ProjectCreate{})
	f.service(p.ID, "app", nil)
	base := "/" + p.ID + "/env"

	for _, k := range []any{nil, "", "1ABC", "A-B", "A B", "ÄB", 42, strings.Repeat("K", 129)} {
		f.do("POST", base, map[string]any{"key": k, "value": "v"}).expectError(t, 400, msgInvalidEnvKey)
	}
	f.do("POST", base, nil).expectError(t, 400, msgInvalidEnvKey)
	row := f.do("POST", base, map[string]any{"key": "PLAIN", "value": "  'quoted'  "}).expect(t, 201)
	if row["value"] != "quoted" || row["service_name"] != "" || row["is_build_arg"] != false || row["is_runtime"] != true || row["is_secret"] != false {
		t.Fatalf("defaults %v", row)
	}
	if row = f.do("POST", base, map[string]any{"key": "NUM", "value": 8080}).expect(t, 201); row["value"] != "8080" {
		t.Fatalf("num %v", row)
	}
	if row = f.do("POST", base, map[string]any{"key": "NOVAL"}).expect(t, 201); row["value"] != "" {
		t.Fatalf("noval %v", row)
	}
	row = f.do("POST", base, map[string]any{"key": "SECRET", "value": "s", "is_build_arg": true, "is_runtime": false, "is_secret": "yes"}).expect(t, 201)
	if row["is_build_arg"] != true || row["is_runtime"] != false || row["is_secret"] != true {
		t.Fatalf("flags %v", row)
	}
	row = f.do("POST", base, map[string]any{"key": "NULLS", "is_build_arg": nil, "is_runtime": nil}).expect(t, 201)
	if row["is_build_arg"] != false || row["is_runtime"] != true {
		t.Fatalf("null flags %v", row)
	}
	f.do("POST", base, map[string]any{"key": "BAD", "is_build_arg": "yes"}).expectError(t, 400, "is_build_arg must be a boolean")
	f.do("POST", base, map[string]any{"key": "BAD", "is_runtime": 1}).expectError(t, 400, "is_runtime must be a boolean")
	f.do("POST", base, map[string]any{"key": "S", "service_name": " web "}).expectError(t, 400, "Unknown service: web")
	if row = f.do("POST", base, map[string]any{"key": "PLAIN", "value": "x", "service_name": " app "}).expect(t, 201); row["service_name"] != "app" {
		t.Fatalf("scoped %v", row)
	}
	if row = f.do("POST", base, map[string]any{"key": "BLANK", "service_name": "   "}).expect(t, 201); row["service_name"] != "" {
		t.Fatalf("blank scope %v", row)
	}
	f.do("POST", base, map[string]any{"key": "PLAIN"}).expectError(t, 409, "Environment variable key already exists for this service scope")
	f.do("POST", "/"+missingID+"/env", map[string]any{"key": "A"}).expectError(t, 500, "Failed to create environment variable")

	var keys []string
	for _, e := range f.do("GET", base, nil).arr(t) {
		m := e.(map[string]any)
		keys = append(keys, m["service_name"].(string)+"/"+m["key"].(string))
	}
	if got := strings.Join(keys, ","); got != "/BLANK,/NOVAL,/NULLS,/NUM,/PLAIN,/SECRET,app/PLAIN" {
		t.Fatalf("order %s", got)
	}
	if got := f.do("GET", base+"?service=%20app%20", nil).arr(t); len(got) != 1 {
		t.Fatalf("scoped list %v", got)
	}
	if got := f.do("GET", base+"?service=", nil).arr(t); len(got) != 6 {
		t.Fatalf("shared list %d", len(got))
	}
	if got := f.do("GET", base+"?service=app&service=x", nil).arr(t); len(got) != 7 {
		t.Fatalf("repeated ?service is no filter: %d", len(got))
	}
}

func TestEnvBulk(t *testing.T) {
	f := newFixture(t, Options{})
	p := f.project(store.ProjectCreate{})
	f.service(p.ID, "app", nil)
	bulk := "/" + p.ID + "/env/bulk"
	f.do("POST", bulk, map[string]any{}).expectError(t, 400, "Content is required")
	f.do("POST", bulk, map[string]any{"content": 5}).expectError(t, 400, "Content is required")
	f.do("POST", bulk, map[string]any{"content": "A=1", "service_name": "web"}).expectError(t, 400, "Unknown service: web")
	f.do("POST", bulk, map[string]any{"content": "# c\n\nNOEQ\n"}).expectError(t, 400, "No valid KEY=VALUE pairs found")
	f.do("POST", bulk, map[string]any{"content": "OK=1\n1BAD=2"}).expectError(t, 400, "Invalid env key: 1BAD")
	f.do("POST", bulk, map[string]any{"content": "A=1", "is_runtime": "no"}).expectError(t, 400, "is_runtime must be a boolean")
	got := f.do("POST", bulk, map[string]any{"content": "A=1\r\nB = \"two\"\nA=dup\n"}).expect(t, 201)
	if got["count"] != float64(2) || got["message"] != "Added 2 environment variable(s)" {
		t.Fatalf("bulk %v", got)
	}
	rows := f.do("GET", "/"+p.ID+"/env", nil).arr(t)
	for _, r := range rows {
		m := r.(map[string]any)
		if m["is_build_arg"] != true || m["is_runtime"] != true {
			t.Fatalf("bulk defaults %v", m)
		}
	}
	f.do("POST", bulk, map[string]any{"content": "G=1\nA=2\nH=3"}).expectError(t, 409, "Environment variable already exists: A")
	key := "G"
	if _, err := f.d.Store.FindEnvVariable(context.Background(), store.EnvFilter{ProjectID: &p.ID, Key: &key}); err != nil {
		t.Fatal("keys before the conflict stay created")
	}
	f.do("POST", "/"+missingID+"/env/bulk", map[string]any{"content": "A=1"}).expectError(t, 500, "Failed to bulk import environment variables")
}

func TestEnvManagedDatabase(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	dbp := f.project(store.ProjectCreate{ProjectType: store.Ptr("database"), DBEngine: store.Ptr("postgres")})
	base := "/" + dbp.ID + "/env"
	f.do("POST", base, map[string]any{"key": "POSTGRES_PASSWORD", "value": "x"}).
		expectError(t, 400, "Managed database credentials are set at create time. Official images only apply them on first volume init — recreate the database to rotate.")
	f.do("POST", base+"/bulk", map[string]any{"content": "OK=1\nPOSTGRES_USER=x"}).
		expectError(t, 400, "Cannot import managed credential key POSTGRES_USER. Recreate the database to rotate credentials.")
	f.do("POST", base, map[string]any{"key": "OTHER", "value": "x"}).expect(t, 201)
	cred, err := f.d.Store.CreateEnvVariable(ctx, store.EnvCreate{ProjectID: dbp.ID, Key: "POSTGRES_DB", Value: "db"})
	if err != nil {
		t.Fatal(err)
	}
	f.do("DELETE", base+"/"+cred.ID, nil).expectError(t, 400, "Cannot delete managed database credentials. Recreate the database to rotate passwords.")
}

func TestEnvDelete(t *testing.T) {
	f := newFixture(t, Options{})
	ctx := context.Background()
	p := f.project(store.ProjectCreate{})
	other := f.project(store.ProjectCreate{})
	dbp := f.project(store.ProjectCreate{ProjectType: store.Ptr("database"), DBEngine: store.Ptr("postgres")})
	e := f.do("POST", "/"+p.ID+"/env", map[string]any{"key": "A"}).expect(t, 201)
	eid := e["id"].(string)
	f.do("DELETE", "/"+other.ID+"/env/"+eid, nil).expectError(t, 404, msgEnvNotFound)
	f.do("DELETE", "/"+p.ID+"/env/"+eid, nil).expect(t, 200)
	f.do("DELETE", "/"+p.ID+"/env/"+eid, nil).expectError(t, 404, msgEnvNotFound)

	linked := f.do("POST", "/"+p.ID+"/env", map[string]any{"key": "DATABASE_URL", "value": "x"}).expect(t, 201)
	if _, err := f.d.Store.UpsertLink(ctx, store.LinkCreate{DatabaseProjectID: dbp.ID, AppProjectID: p.ID, EnvKey: "DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	f.do("DELETE", "/"+p.ID+"/env/"+linked["id"].(string), nil).expectError(t, 400, "This variable is owned by a managed database link. Unlink the database first.")
}
