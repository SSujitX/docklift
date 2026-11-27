package contract

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"
)

func seedProjectRow(over Obj) Obj {
	return projectRow(Obj{"status": Str}).merge(over)
}

func pushPayload(ref, cloneURL string, extra map[string]any) []byte {
	p := map[string]any{
		"ref":         ref,
		"repository":  map[string]any{"clone_url": cloneURL, "html_url": strings.TrimSuffix(cloneURL, ".git")},
		"head_commit": map[string]any{"id": "abcdef1234567890", "message": "contract push"},
		"pusher":      map[string]any{"name": "contract"},
	}
	for k, v := range extra {
		if v == nil {
			delete(p, k)
		} else {
			p[k] = v
		}
	}
	b, _ := json.Marshal(p)
	return b
}

func sortedStrings(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	sort.Strings(out)
	return out
}

// TestSeeded runs against an instance booted on testdata/seed.db.gz (an
// existing-install database written by the TS backend's Prisma client). The
// subtests share that state and run in order.
func TestSeeded(t *testing.T) {
	inst := seededInstance(t)
	s := mustLogin(t, inst, seedAdminEmail, seedAdminPassword)
	c := s.Client()
	anon := inst.Anon()

	steps := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{"existing session tokens", func(t *testing.T) {
			match(t, "login pwdv", float64(seedAdminPwdv), sessionClaims(t, s.Token)["pwdv"])
			now := time.Now().Unix()
			minted := signJWT(knownSecrets.JWT, map[string]any{"userId": seedAdminID, "email": seedAdminEmail, "role": "admin", "pwdv": seedAdminPwdv, "iat": now, "exp": now + 600})
			inst.As(minted).Get(t, "/api/auth/me").Expect(t, 200, Obj{"user": Obj{"id": seedAdminID, "name": "Seed Admin", "email": seedAdminEmail, "role": "admin", "created_at": "2026-01-01T00:00:00.000Z"}})
			anon.With("X-Internal-Secret", knownSecrets.Internal).Get(t, "/api/ports").ExpectStatus(t, 200)
			anon.Get(t, "/api/auth/status").Expect(t, 200, Obj{"setupComplete": true, "userCount": 1, "bootstrapRequired": false})
		}},
		{"webhook signature verification", func(t *testing.T) {
			body := pushPayload("refs/heads/main", "https://github.com/nobody/unknown.git", nil)
			sig := "sha256=" + hmacHex(seedWebhookSecret, string(body))
			post := func(b []byte, headers ...string) *Resp {
				return anon.Post(t, "/api/github/webhook", RawBody{"application/json", b}, append([]string{"X-GitHub-Event", "push"}, headers...)...)
			}
			post(body).ExpectError(t, 401, "Missing webhook signature")
			post(body, "X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64)).ExpectError(t, 401, "Invalid signature")
			post(body, "X-Hub-Signature-256", strings.TrimPrefix(sig, "sha256=")).ExpectError(t, 401, "Invalid signature")
			post(body, "X-Hub-Signature-256", "sha256="+hmacHex("wrong-secret", string(body))).ExpectError(t, 401, "Invalid signature")
			post(append(body, ' '), "X-Hub-Signature-256", sig).ExpectError(t, 401, "Invalid signature")
			post(body, "X-Hub-Signature-256", strings.ToUpper(sig)).ExpectError(t, 401, "Invalid signature")
			// Good signature, unknown repository.
			post(body, "X-Hub-Signature-256", sig).Expect(t, 200, Obj{"message": "No matching projects with auto-deploy enabled"})
			// HMAC is over the raw bytes, not a re-serialisation.
			spaced := []byte(strings.Replace(string(body), `"ref":`, `"ref" :  `, 1))
			post(spaced, "X-Hub-Signature-256", "sha256="+hmacHex(seedWebhookSecret, string(spaced))).Expect(t, 200, Obj{"message": "No matching projects with auto-deploy enabled"})
			// Non-push events are acknowledged without a signature check.
			anon.Post(t, "/api/github/webhook", map[string]any{"zen": "x"}, "X-GitHub-Event", "ping").Expect(t, 200, Obj{"message": "Ignoring ping event"})
			anon.Post(t, "/api/github/webhook", map[string]any{"zen": "x"}, "X-GitHub-Event", "ping", "X-Hub-Signature-256", "sha256=bad").Expect(t, 200, Obj{"message": "Ignoring ping event"})
		}},
		{"webhook payload filtering", func(t *testing.T) {
			send := func(b []byte) *Resp {
				return anon.Post(t, "/api/github/webhook", RawBody{"application/json", b}, "X-GitHub-Event", "push", "X-Hub-Signature-256", "sha256="+hmacHex(seedWebhookSecret, string(b)))
			}
			send(pushPayload("refs/heads/main", seedRepo+".git", map[string]any{"deleted": true})).Expect(t, 200, Obj{"message": "Ignoring branch deletion or empty push"})
			send(pushPayload("refs/heads/main", seedRepo+".git", map[string]any{"head_commit": nil})).Expect(t, 200, Obj{"message": "Ignoring branch deletion or empty push"})
			send(pushPayload("refs/heads/main", seedRepo+".git", map[string]any{"repository": map[string]any{}})).Expect(t, 200, Obj{"message": "No repository URL in payload"})
			// Matching repo, non-matching refs: nothing is triggered and no cooldown starts.
			r := send(pushPayload("refs/tags/v1.0.0", seedRepo+".git", nil)).Expect(t, 200, Obj{"message": "No deployments triggered", "triggered": Arr{}, "skipped": AnyArr, "branch": "v1.0.0", "commit": "abcdef1"}).(map[string]any)
			match(t, "tag push skipped", []string{"seed-gh-develop (ref mismatch: v1.0.0 != develop)", "seed-gh-main (ref mismatch: v1.0.0 != main)"}, toAny(sortedStrings(r["skipped"])))
			r = send(pushPayload("refs/pull/7/head", seedRepo, nil)).Expect(t, 200, Obj{"message": "No deployments triggered", "triggered": Arr{}, "skipped": AnyArr, "branch": "refs/pull/7/head", "commit": "abcdef1"}).(map[string]any)
			match(t, "pull ref skipped", []string{"seed-gh-develop (ref mismatch: refs/pull/7/head != develop)", "seed-gh-main (ref mismatch: refs/pull/7/head != main)"}, toAny(sortedStrings(r["skipped"])))
		}},
		{"webhook triggers matching auto-deploy projects with cooldown", func(t *testing.T) {
			b := pushPayload("refs/heads/main", seedRepo+".git", map[string]any{"head_commit": map[string]any{"id": "1234567890abcdef", "message": "deploy me"}})
			sig := "sha256=" + hmacHex(seedWebhookSecret, string(b))
			r := anon.Post(t, "/api/github/webhook", RawBody{"application/json", b}, "X-GitHub-Event", "push", "X-Hub-Signature-256", sig)
			r.Expect(t, 200, Obj{"message": "Auto-deploy triggered", "triggered": Arr{"seed-gh-main"}, "skipped": Arr{"seed-gh-develop (ref mismatch: main != develop)"},
				"branch": "main", "commit": "1234567"})
			r = anon.Post(t, "/api/github/webhook", RawBody{"application/json", b}, "X-GitHub-Event", "push", "X-Hub-Signature-256", sig)
			got := r.Expect(t, 200, Obj{"message": "No deployments triggered", "triggered": Arr{}, "skipped": AnyArr, "branch": "main", "commit": "1234567"}).(map[string]any)
			match(t, "cooldown", []string{"seed-gh-develop (ref mismatch: main != develop)", "seed-gh-main (cooldown)"}, toAny(sortedStrings(got["skipped"])))
			// The triggered internal deploy fails fast (no checkout on disk) and leaves no row.
			time.Sleep(500 * time.Millisecond)
			c.Get(t, "/api/deployments/"+seedGHMain).Expect(t, 200, Arr{})
			c.Post(t, "/api/deployments/"+seedGHMain+"/deploy", nil).ExpectError(t, 400, "Project files not found")
		}},
		{"github app configured state", func(t *testing.T) {
			installURL := "https://github.com/apps/docklift-contract/installations/new"
			c.Get(t, "/api/github/app-status").Expect(t, 200, Obj{"configured": true, "installed": true, "appId": "123456", "appName": "docklift-contract",
				"appSlug": "docklift-contract", "installationId": "987654", "username": "contract-user", "avatarUrl": "https://avatars.example.test/u/1", "installUrl": installURL})
			c.Get(t, "/api/github/status").Expect(t, 200, Obj{"connected": true, "username": "contract-user", "avatar_url": "https://avatars.example.test/u/1",
				"installation_id": "987654", "app_name": "docklift-contract", "app_slug": "docklift-contract", "warning": "Private key not found"})
			c.Post(t, "/api/github/check-installation", nil).Expect(t, 200, Obj{"found": false, "message": "GitHub App credentials not ready. Please complete app creation."})
			c.Get(t, "/api/github/installations").ExpectError(t, 500, "Failed to fetch installations")
			c.Get(t, "/api/github/repos?owner=contract-org").ExpectError(t, 500, "Failed to fetch repositories")
			c.Get(t, "/api/github/branches?repo=contract-org/contract-repo&type=private").ExpectError(t, 403, "Access denied: Could not verify GitHub App installation for this repository.")

			r := c.Post(t, "/api/github/install-session", map[string]any{"returnUrl": "/projects/new"})
			r.Expect(t, 200, Obj{"installUrl": installURL})
			state, _ := stateFromCookie(t, r)
			r = c.Post(t, "/api/github/install-session", map[string]any{"return_url": "javascript:alert(1)"})
			r.Expect(t, 200, Obj{"installUrl": installURL})
			r = c.Get(t, "/api/github/install?return_url=/x")
			expectRedirect(t, r, installURL)
			stateFromCookie(t, r)
			fe := frontendURL + "/settings?github_error="
			// Valid nonce, but the App key is missing: fail closed before GitHub is called.
			expectRedirect(t, anon.Get(t, "/api/github/setup?installation_id=987654&state="+state), fe+"setup_failed")
			c.Get(t, "/api/github/app-status").Expect(t, 200, Obj{"configured": true, "installed": true, "appId": "123456", "appName": Str, "appSlug": Str,
				"installationId": "987654", "username": "contract-user", "avatarUrl": Str, "installUrl": installURL})
		}},
		{"github auto-deploy toggles", func(t *testing.T) {
			c.Get(t, "/api/projects/"+seedGHMain+"/auto-deploy").Expect(t, 200, Obj{"auto_deploy": true, "webhook_secret": strings.Repeat("a", 64),
				"webhook_url": "/api/github/webhook", "available": true})
			c.Get(t, "/api/projects/"+seedGHManual+"/auto-deploy").Expect(t, 200, Obj{"auto_deploy": false, "webhook_secret": nil, "webhook_url": nil, "available": true})
			on := c.Patch(t, "/api/projects/"+seedGHManual+"/auto-deploy", map[string]any{"enabled": true}).
				Expect(t, 200, Obj{"auto_deploy": true, "webhook_secret": Hex(64), "webhook_url": "/api/github/webhook"}).(map[string]any)
			c.Patch(t, "/api/projects/"+seedGHManual+"/auto-deploy", map[string]any{"enabled": false}).
				Expect(t, 200, Obj{"auto_deploy": false, "webhook_secret": on["webhook_secret"], "webhook_url": nil})
			c.Get(t, "/api/projects/"+seedGHManual+"/auto-deploy").Expect(t, 200, Obj{"auto_deploy": false, "webhook_secret": nil, "webhook_url": nil, "available": true})
			c.Patch(t, "/api/projects/"+seedGHManual+"/auto-deploy", map[string]any{"enabled": true}).
				Expect(t, 200, Obj{"auto_deploy": true, "webhook_secret": on["webhook_secret"], "webhook_url": "/api/github/webhook"})
			c.Patch(t, "/api/projects/"+seedGHManual+"/auto-deploy", map[string]any{"enabled": false}).ExpectStatus(t, 200)
		}},
		{"managed database listing and connection", func(t *testing.T) {
			dbRow := func(id, name, engine, status string, port int, created string) Obj {
				return seedProjectRow(Obj{"id": id, "name": name, "description": engine + " seed", "source_type": "managed", "project_type": "database",
					"db_engine": engine, "status": status, "build_type": "dockerfile", "internal_port": port, "created_at": created,
					"services":          ArrayOf(serviceRow(Obj{"project_id": id, "name": "db", "container_name": "dl_" + name + "_5eed0000_db", "port": nil, "internal_port": port, "domain": nil}), 1),
					"databaseLinksAsDb": Arr{}})
			}
			c.Get(t, "/api/databases").Expect(t, 200, Arr{
				dbRow(seedMySQL, "seed-mysql", "mysql", "running", 3306, "2026-01-01T00:08:00.000Z"),
				dbRow(seedRedis, "seed-redis", "redis", "stopped", 6379, "2026-01-01T00:07:00.000Z"),
				dbRow(seedPG, "seed-pg", "postgres", "running", 5432, "2026-01-01T00:06:00.000Z"),
			})
			engine := Obj{"id": "postgres", "label": "PostgreSQL", "description": Str, "imageRepo": "postgres", "versions": ArrayOf(Obj{"tag": Str, "label": Str, "recommended": Optional(true)}, 1),
				"image": "postgres:18-alpine", "port": 5432, "volumeMount": "/var/lib/postgresql/data", "defaultEnvKey": "DATABASE_URL", "serviceName": "db"}
			c.Get(t, "/api/databases/"+seedPG+"/connection").Expect(t, 200, Obj{"engine": engine, "host": "dl_seed-pg_5eed0000_db", "port": 5432, "username": "docklift",
				"database": "seed_pg", "password": "seedpgpass", "connection_url": "postgresql://docklift:seedpgpass@dl_seed-pg_5eed0000_db:5432/seed_pg",
				"internal_only": true, "publish_host_port": false, "host_port": nil, "credentials_init_only": true,
				"note": "Reachable from linked apps on the app Docker network via the host above. Prefer linking over publishing host ports."})
			c.Get(t, "/api/databases/"+seedRedis+"/connection").Expect(t, 200, Obj{"engine": AnyObj, "host": "dl_seed-redis_5eed0000_db", "port": 6379, "username": nil,
				"database": "0", "password": "seedredispass", "connection_url": "redis://:seedredispass@dl_seed-redis_5eed0000_db:6379",
				"internal_only": true, "publish_host_port": false, "host_port": nil, "credentials_init_only": true, "note": Str})
			c.Get(t, "/api/databases/"+seedMySQL+"/connection").ExpectError(t, 404, "Managed database not found or credentials missing")
			c.Get(t, "/api/databases/"+seedApp+"/connection").ExpectError(t, 404, "Managed database not found or credentials missing")
			c.Get(t, "/api/databases/"+seedPG+"/links").Expect(t, 200, Arr{})
			c.Get(t, "/api/databases/"+seedApp+"/links").ExpectError(t, 404, "Database not found")
			c.Get(t, "/api/databases/links/by-app/"+seedApp).Expect(t, 200, Arr{})
		}},
		{"managed credential env is locked", func(t *testing.T) {
			pgEnv := "/api/projects/" + seedPG + "/env"
			msg := "Managed database credentials are set at create time. Official images only apply them on first volume init — recreate the database to rotate."
			for _, k := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
				c.Post(t, pgEnv, map[string]any{"key": k, "value": "x"}).ExpectError(t, 400, msg)
			}
			c.Post(t, pgEnv+"/bulk", map[string]any{"content": "OK_KEY=1\nPOSTGRES_DB=evil"}).ExpectError(t, 400, "Cannot import managed credential key POSTGRES_DB. Recreate the database to rotate credentials.")
			rows := c.Get(t, pgEnv).Arr(t)
			if len(rows) != 3 || findBy(rows, "key", "OK_KEY") != nil {
				t.Fatalf("bulk with a locked key must create nothing: %v", short(rows))
			}
			match(t, "secret flag", envRow(Obj{"project_id": seedPG, "key": "POSTGRES_PASSWORD", "value": "seedpgpass", "is_secret": true}), findBy(rows, "key", "POSTGRES_PASSWORD"))
			c.Delete(t, pgEnv+"/"+findBy(rows, "key", "POSTGRES_DB")["id"].(string)).ExpectError(t, 400, "Cannot delete managed database credentials. Recreate the database to rotate passwords.")
			// Other keys (and other engines' credential names) are fine.
			c.Post(t, pgEnv, map[string]any{"key": "MYSQL_PASSWORD", "value": "x"}).ExpectStatus(t, 201)
			c.Post(t, "/api/projects/"+seedRedis+"/env", map[string]any{"key": "REDIS_PASSWORD", "value": "x"}).ExpectError(t, 400, msg)
		}},
		{"database links", func(t *testing.T) {
			links := "/api/databases/" + seedPG + "/links"
			c.Post(t, links, map[string]any{}).ExpectError(t, 400, "app_project_id is required")
			c.Post(t, links, map[string]any{"app_project_id": "   "}).ExpectError(t, 400, "app_project_id is required")
			c.Post(t, links, map[string]any{"app_project_id": missingProjectID}).ExpectError(t, 404, "App project not found")
			c.Post(t, links, map[string]any{"app_project_id": seedRedis}).ExpectError(t, 404, "App project not found")
			c.Post(t, links, map[string]any{"app_project_id": seedApp, "service_name": "web"}).ExpectError(t, 400, `Service "web" not found on that project`)
			c.Post(t, links, map[string]any{"app_project_id": seedApp, "env_key": "bad-key"}).ExpectError(t, 400, "Invalid env_key")
			c.Post(t, links, map[string]any{"app_project_id": seedApp, "env_key": strings.Repeat("K", 65)}).ExpectError(t, 400, "Invalid env_key")
			c.Post(t, "/api/databases/"+missingProjectID+"/links", map[string]any{"app_project_id": seedApp}).ExpectError(t, 404, "Database not found")
			c.Post(t, "/api/databases/"+seedApp+"/links", map[string]any{"app_project_id": seedApp}).ExpectError(t, 404, "Database not found")
			c.Post(t, "/api/databases/"+seedMySQL+"/links", map[string]any{"app_project_id": seedApp}).ExpectError(t, 400, "Database credentials missing — deploy the database first")
			c.Post(t, "/api/databases/"+seedRedis+"/links", map[string]any{"app_project_id": seedApp}).ExpectError(t, 409, "Database must be running before linking. Deploy it first.")

			appEnv := "/api/projects/" + seedApp + "/env"
			c.Post(t, appEnv, map[string]any{"key": "PG_URL", "value": "manual"}).ExpectStatus(t, 201)
			c.Post(t, links, map[string]any{"app_project_id": seedApp, "env_key": "PG_URL"}).
				ExpectError(t, 409, `"PG_URL" already exists on this project/service. Pass overwrite=true to replace it with the managed database URL.`)
			url := "postgresql://docklift:seedpgpass@dl_seed-pg_5eed0000_db:5432/seed_pg"
			linkObj := Obj{"id": UUID, "database_project_id": seedPG, "app_project_id": seedApp, "service_name": Str, "env_key": Str, "created_at": ISOms}
			created := func(envKey string, service any) Obj {
				return Obj{"link": linkObj.merge(Obj{}), "env_key": envKey, "service_name": service, "network_attached": Bool, "network_error": OrNull(Str), "note": Str}
			}
			ow := c.Post(t, links, map[string]any{"app_project_id": seedApp, "env_key": "PG_URL", "overwrite": true}).Expect(t, 201, created("PG_URL", nil)).(map[string]any)
			if !cfg.docker {
				match(t, "no-docker link", Obj{"network_attached": false, "network_error": NonEmpty, "note": Prefix("Connection URL saved. Network attach pending (")},
					map[string]any{"network_attached": ow["network_attached"], "network_error": ow["network_error"], "note": ow["note"]})
			}
			first := c.Post(t, links, map[string]any{"app_project_id": " " + seedApp + " ", "service_name": " app "}).Expect(t, 201, created("DATABASE_URL", "app")).(map[string]any)
			// Linking again is idempotent (same link row).
			again := c.Post(t, links, map[string]any{"app_project_id": seedApp, "service_name": "app"}).Expect(t, 201, created("DATABASE_URL", "app")).(map[string]any)
			if again["link"].(map[string]any)["id"] != first["link"].(map[string]any)["id"] {
				t.Fatal("relink must reuse the link row")
			}
			env := c.Get(t, appEnv+"?service=app").Arr(t)
			match(t, "injected env", envRow(Obj{"project_id": seedApp, "service_name": "app", "key": "DATABASE_URL", "value": url, "is_build_arg": false, "is_runtime": true, "is_secret": true}),
				findBy(env, "key", "DATABASE_URL"))
			match(t, "overwritten env", envRow(Obj{"project_id": seedApp, "key": "PG_URL", "value": url, "is_secret": true}), findBy(c.Get(t, appEnv+"?service=").Arr(t), "key", "PG_URL"))
			c.Delete(t, appEnv+"/"+findBy(env, "key", "DATABASE_URL")["id"].(string)).ExpectError(t, 400, "This variable is owned by a managed database link. Unlink the database first.")

			listed := c.Get(t, links).Arr(t)
			if len(listed) != 2 {
				t.Fatalf("want 2 links, got %v", short(listed))
			}
			for _, l := range listed {
				match(t, "link with app", linkObj.merge(Obj{"app_project": Obj{"id": seedApp, "name": "seed-app", "status": Str,
					"services": Arr{Obj{"id": seedAppService, "name": "app", "status": Str}}}}), l)
			}
			byApp := c.Get(t, "/api/databases/links/by-app/"+seedApp).Arr(t)
			for _, l := range byApp {
				match(t, "link with db", Obj{"id": UUID, "database_project_id": seedPG, "app_project_id": seedApp, "service_name": Str, "env_key": Str, "created_at": ISOms,
					"database_project": Obj{"id": seedPG, "name": "seed-pg", "status": Str, "db_engine": "postgres"}}, l)
			}
			dbs := c.Get(t, "/api/databases").Arr(t)
			pg := findBy(dbs, "id", seedPG)
			match(t, "db links on listing", ArrayOf(Obj{"id": UUID, "database_project_id": seedPG, "app_project_id": seedApp, "service_name": Str, "env_key": Str, "created_at": ISOms,
				"app_project": Obj{"id": seedApp, "name": "seed-app", "status": Str}}, 2), pg["databaseLinksAsDb"])

			lid := first["link"].(map[string]any)["id"].(string)
			c.Delete(t, "/api/databases/"+seedRedis+"/links/"+lid).ExpectError(t, 404, "Link not found")
			c.Delete(t, "/api/databases/"+seedPG+"/links/"+lid).Expect(t, 200, Obj{"status": "unlinked",
				"note": "Env removed if no other database owns that key. Redeploy the app to drop it from running containers."})
			c.Delete(t, "/api/databases/"+seedPG+"/links/"+lid).ExpectError(t, 404, "Link not found")
			if findBy(c.Get(t, appEnv+"?service=app").Arr(t), "key", "DATABASE_URL") != nil {
				t.Fatal("unlink must remove the injected env")
			}
		}},
		{"rollback guards", func(t *testing.T) {
			rb := func(project, target string) *Resp {
				return c.Post(t, "/api/deployments/"+project+"/rollback", map[string]any{"password": seedAdminPassword, "deploymentId": target})
			}
			hist := c.Get(t, "/api/deployments/"+seedApp).Arr(t)
			match(t, "tagged deployment", deploymentRow(Obj{"id": seedDepLatest, "project_id": seedApp, "status": "success", "trigger": "manual",
				"commit_message": "seed deployment 304", "image_tags": Obj{"app": "docklift-5eed0000-app:5eed0004"}, "logs": "seed log 304\n",
				"created_at": "2026-01-05T00:00:00.000Z", "finished_at": "2026-01-05T00:00:00.000Z"}), hist[0])
			match(t, "untagged deployment", deploymentRow(Obj{"id": seedDepNoTags, "image_tags": nil}), findBy(hist, "id", seedDepNoTags))
			rb(seedApp, seedDepLatest).ExpectError(t, 400, "That deployment is already the current success")
			rb(seedApp, seedDepNoTags).ExpectError(t, 409, "This deployment has no stored image tags (pre-keep-2). Redeploy from git instead.")
			rb(seedApp, seedDepFailed).ExpectError(t, 404, "Target successful deployment not found")
			rb(seedApp, seedPG).ExpectError(t, 404, "Target successful deployment not found")
			rb(seedPG, seedDepOldTagged).ExpectError(t, 400, "Rollback is not supported for managed databases")
			rb(seedGHMain, seedDepOldTagged).ExpectError(t, 404, "Target successful deployment not found")
			t.Run("missing image", func(t *testing.T) {
				requireNoDocker(t)
				rb(seedApp, seedDepOldTagged).ExpectError(t, 409, "Image missing for app: docklift-5eed0000-app:5eed0000. Redeploy from git instead.")
				row := lastDeployment(t, c, seedApp)
				match(t, "rollback row", deploymentRow(Obj{"project_id": seedApp, "status": "failed", "trigger": "rollback", "commit_message": "Rollback: seed deployment 301",
					"commit_sha": nil, "image_tags": Obj{"app": "docklift-5eed0000-app:5eed0000"}, "logs": "❌ Image missing for app: docklift-5eed0000-app:5eed0000\n", "finished_at": ISOms}), row)
				// The lock is released: a second attempt gets the same answer, not a conflict.
				rb(seedApp, seedDepOldTagged).ExpectError(t, 409, "Image missing for app: docklift-5eed0000-app:5eed0000. Redeploy from git instead.")
			})
		}},
		{"purge without docker", func(t *testing.T) {
			requireNoDocker(t)
			c.Post(t, "/api/system/purge", map[string]any{"password": seedAdminPassword}).ExpectError(t, 500, "Failed to complete purge operation")
		}},
		{"project reads resync status from docker", func(t *testing.T) {
			requireNoDocker(t)
			list := c.Get(t, "/api/projects").Arr(t)
			match(t, "seed github project", seedProjectRow(Obj{"id": seedGHMain, "name": "seed-gh-main", "source_type": "github", "github_url": seedRepo, "github_branch": "main",
				"auto_deploy": true, "webhook_secret": strings.Repeat("a", 64), "status": "stopped", "created_at": "2026-01-01T00:01:00.000Z"}), findBy(list, "id", seedGHMain))
			// A "running" database whose container is gone reads back as stopped.
			match(t, "seed pg", seedProjectRow(Obj{"id": seedPG, "name": "seed-pg", "description": "postgres seed", "source_type": "managed", "project_type": "database",
				"db_engine": "postgres", "status": "stopped", "build_type": "dockerfile", "internal_port": 5432}), findBy(list, "id", seedPG))
			c.Post(t, "/api/databases/"+seedPG+"/links", map[string]any{"app_project_id": seedApp}).ExpectError(t, 409, "Database must be running before linking. Deploy it first.")
		}},
		{"github disconnect", func(t *testing.T) {
			c.Post(t, "/api/github/disconnect", nil).Expect(t, 200, Obj{"message": "GitHub disconnected"})
			c.Get(t, "/api/github/status").Expect(t, 200, Obj{"connected": false, "username": nil, "app_name": "docklift-contract", "app_slug": "docklift-contract"})
			c.Get(t, "/api/github/app-status").Expect(t, 200, Obj{"configured": true, "installed": false, "appId": "123456", "appName": "docklift-contract",
				"appSlug": "docklift-contract", "installationId": nil, "username": nil, "avatarUrl": nil, "installUrl": "https://github.com/apps/docklift-contract/installations/new"})
		}},
	}
	for _, st := range steps {
		if !t.Run(st.name, st.fn) {
			return
		}
	}
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
