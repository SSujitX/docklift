package contract

import (
	"strings"
	"testing"
)

const invalidEnvKeyMsg = "Invalid env key. Use letters, digits, underscore; must start with a letter or _."

func TestEnvVariables(t *testing.T) {
	c := admin.Client()
	project, svc := deployedProject(t, c, "env")
	id := project["id"].(string)
	base := "/api/projects/" + id + "/env"
	if svc["name"] != "app" {
		t.Fatalf("service name = %v", svc["name"])
	}

	t.Run("empty list", func(t *testing.T) {
		other := createProject(t, c, uniq("env-empty"), dockerfileApp())
		c.Get(t, "/api/projects/"+other["id"].(string)+"/env").Expect(t, 200, Arr{})
		c.Get(t, "/api/projects/"+missingProjectID+"/env").Expect(t, 200, Arr{})
	})
	t.Run("key validation", func(t *testing.T) {
		for _, k := range []any{nil, "", "1ABC", "A-B", "A B", "A.B", "ÄB", 42, strings.Repeat("K", 129)} {
			c.Post(t, base, map[string]any{"key": k, "value": "v"}).ExpectError(t, 400, invalidEnvKeyMsg)
		}
		c.Post(t, base, map[string]any{"key": "_" + strings.Repeat("K", 127), "value": "v"}).ExpectStatus(t, 201)
	})
	t.Run("create shared with defaults and value normalisation", func(t *testing.T) {
		c.Post(t, base, map[string]any{"key": "PLAIN", "value": "  spaced  "}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "PLAIN", "value": "spaced"}))
		c.Post(t, base, map[string]any{"key": "DQ", "value": `"quoted value"`}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "DQ", "value": "quoted value"}))
		c.Post(t, base, map[string]any{"key": "SQ", "value": "'single'"}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "SQ", "value": "single"}))
		c.Post(t, base, map[string]any{"key": "HALFQ", "value": `"half`}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "HALFQ", "value": `"half`}))
		c.Post(t, base, map[string]any{"key": "NUM", "value": 8080}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "NUM", "value": "8080"}))
		c.Post(t, base, map[string]any{"key": "NOVAL"}).Expect(t, 201, envRow(Obj{"project_id": id, "key": "NOVAL", "value": ""}))
	})
	t.Run("flags", func(t *testing.T) {
		c.Post(t, base, map[string]any{"key": "BUILD_ONLY", "value": "1", "is_build_arg": true, "is_runtime": false}).
			Expect(t, 201, envRow(Obj{"project_id": id, "key": "BUILD_ONLY", "value": "1", "is_build_arg": true, "is_runtime": false}))
		c.Post(t, base, map[string]any{"key": "BUILD_SECRET", "value": "s3cr3t", "is_build_arg": true, "is_secret": "yes"}).
			Expect(t, 201, envRow(Obj{"project_id": id, "key": "BUILD_SECRET", "value": "s3cr3t", "is_build_arg": true, "is_secret": true}))
		c.Post(t, base, map[string]any{"key": "NULL_FLAGS", "value": "x", "is_build_arg": nil, "is_runtime": nil, "is_secret": nil}).
			Expect(t, 201, envRow(Obj{"project_id": id, "key": "NULL_FLAGS", "value": "x"}))
	})
	t.Run("service scope", func(t *testing.T) {
		c.Post(t, base, map[string]any{"key": "SCOPED", "value": "x", "service_name": "web"}).ExpectError(t, 400, "Unknown service: web")
		c.Post(t, base, map[string]any{"key": "SCOPED", "value": "x", "service_name": " web "}).ExpectError(t, 400, "Unknown service: web")
		c.Post(t, base, map[string]any{"key": "PLAIN", "value": "app-only", "service_name": " app "}).
			Expect(t, 201, envRow(Obj{"project_id": id, "service_name": "app", "key": "PLAIN", "value": "app-only"}))
		c.Post(t, base, map[string]any{"key": "BLANKSCOPE", "value": "x", "service_name": "   "}).
			Expect(t, 201, envRow(Obj{"project_id": id, "service_name": "", "key": "BLANKSCOPE", "value": "x"}))
	})
	t.Run("duplicates", func(t *testing.T) {
		c.Post(t, base, map[string]any{"key": "PLAIN", "value": "again"}).ExpectError(t, 409, "Environment variable key already exists for this service scope")
		c.Post(t, base, map[string]any{"key": "PLAIN", "value": "again", "service_name": "app"}).ExpectError(t, 409, "Environment variable key already exists for this service scope")
		// Keys are case-sensitive.
		c.Post(t, base, map[string]any{"key": "plain", "value": "lower"}).ExpectStatus(t, 201)
	})
	t.Run("list ordering and service filter", func(t *testing.T) {
		all := c.Get(t, base).Arr(t)
		var keys []string
		for _, e := range all {
			m := e.(map[string]any)
			match(t, "env row", envRow(Obj{"project_id": id, "service_name": Str, "key": Str, "value": Str, "is_build_arg": Bool, "is_runtime": Bool, "is_secret": Bool}), m)
			keys = append(keys, m["service_name"].(string)+"/"+m["key"].(string))
		}
		want := []string{"/BLANKSCOPE", "/BUILD_ONLY", "/BUILD_SECRET", "/DQ", "/HALFQ", "/NOVAL", "/NULL_FLAGS", "/NUM", "/PLAIN", "/SQ", "/_" + strings.Repeat("K", 127), "/plain", "app/PLAIN"}
		if strings.Join(keys, ",") != strings.Join(want, ",") {
			t.Fatalf("order = %v\nwant    %v", keys, want)
		}
		appOnly := c.Get(t, base+"?service=app").Arr(t)
		if len(appOnly) != 1 || appOnly[0].(map[string]any)["value"] != "app-only" {
			t.Fatalf("?service=app = %v", appOnly)
		}
		if got := len(c.Get(t, base+"?service=%20app%20").Arr(t)); got != 1 {
			t.Fatalf("?service is trimmed: got %d rows", got)
		}
		if got := len(c.Get(t, base+"?service=").Arr(t)); got != len(all)-1 {
			t.Fatalf("?service= (shared only) = %d rows, want %d", got, len(all)-1)
		}
		c.Get(t, base+"?service=nope").Expect(t, 200, Arr{})
	})
	t.Run("bulk import", func(t *testing.T) {
		bulk := base + "/bulk"
		c.Post(t, bulk, map[string]any{}).ExpectError(t, 400, "Content is required")
		c.Post(t, bulk, map[string]any{"content": 5}).ExpectError(t, 400, "Content is required")
		c.Post(t, bulk, map[string]any{"content": "A=1", "service_name": "web"}).ExpectError(t, 400, "Unknown service: web")
		c.Post(t, bulk, map[string]any{"content": "# only comments\n\nNOEQUALS\n"}).ExpectError(t, 400, "No valid KEY=VALUE pairs found")
		c.Post(t, bulk, map[string]any{"content": "OK=1\n1BAD=2"}).ExpectError(t, 400, "Invalid env key: 1BAD")
		c.Post(t, bulk, map[string]any{"content": "export FOO=bar"}).ExpectError(t, 400, "Invalid env key: export FOO")
		c.Post(t, bulk, map[string]any{"content": "=novalue"}).ExpectError(t, 400, "Invalid env key: ")
		content := "# comment\r\nBULK_A=1\r\n  BULK_B = \"two words\"  \nBULK_A=ignored-duplicate\nBULK_C='x=y'\nBULK_EMPTY=\n"
		c.Post(t, bulk, map[string]any{"content": content}).Expect(t, 201, Obj{"count": 4, "message": "Added 4 environment variable(s)"})
		rows := c.Get(t, base+"?service=").Arr(t)
		want := map[string]string{"BULK_A": "1", "BULK_B": "two words", "BULK_C": "x=y", "BULK_EMPTY": ""}
		for k, v := range want {
			row := findBy(rows, "key", k)
			if row == nil {
				t.Fatalf("bulk key %s missing", k)
			}
			// Bulk defaults to build arg + runtime (single create defaults is_build_arg=false).
			match(t, "bulk row "+k, envRow(Obj{"project_id": id, "key": k, "value": v, "is_build_arg": true, "is_runtime": true}), row)
		}
		c.Post(t, bulk, map[string]any{"content": "BULK_F=1", "is_build_arg": false, "is_runtime": false, "service_name": "app"}).
			Expect(t, 201, Obj{"count": 1, "message": "Added 1 environment variable(s)"})
		match(t, "scoped bulk", envRow(Obj{"project_id": id, "service_name": "app", "key": "BULK_F", "value": "1"}).merge(Obj{"is_runtime": false}),
			findBy(c.Get(t, base+"?service=app").Arr(t), "key", "BULK_F"))
		// A conflicting key stops the import but earlier keys stay created.
		c.Post(t, bulk, map[string]any{"content": "BULK_G=1\nBULK_A=2\nBULK_H=3"}).ExpectError(t, 409, "Environment variable already exists: BULK_A")
		rows = c.Get(t, base+"?service=").Arr(t)
		if findBy(rows, "key", "BULK_G") == nil || findBy(rows, "key", "BULK_H") != nil {
			t.Fatalf("partial bulk import: want BULK_G created and BULK_H not")
		}
	})
	t.Run("delete", func(t *testing.T) {
		row := findBy(c.Get(t, base).Arr(t), "key", "NOVAL")
		eid := row["id"].(string)
		other := createProject(t, c, uniq("env-other"), dockerfileApp())
		c.Delete(t, "/api/projects/"+other["id"].(string)+"/env/"+eid).ExpectError(t, 404, "Environment variable not found")
		c.Delete(t, base+"/"+eid).Expect(t, 200, Obj{"status": "deleted"})
		c.Delete(t, base+"/"+eid).ExpectError(t, 404, "Environment variable not found")
	})
	t.Run("missing project", func(t *testing.T) {
		c.Post(t, "/api/projects/"+missingProjectID+"/env", map[string]any{"key": "A", "value": "1"}).ExpectError(t, 500, "Failed to create environment variable")
		c.Post(t, "/api/projects/"+missingProjectID+"/env/bulk", map[string]any{"content": "A=1"}).ExpectError(t, 500, "Failed to bulk import environment variables")
	})
	mainInst.Anon().Get(t, base).ExpectError(t, 401, "Authentication required")
}

// merge returns a copy of o with over applied.
func (o Obj) merge(over Obj) Obj {
	out := Obj{}
	for k, v := range o {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}
