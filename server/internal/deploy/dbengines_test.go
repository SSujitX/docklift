package deploy

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func mustEngine(t *testing.T, id string) Engine {
	t.Helper()
	e, ok := GetEngine(id)
	if !ok {
		t.Fatalf("engine %s", id)
	}
	return e
}

func TestEngineCatalog(t *testing.T) {
	if len(ListEngines()) != 5 {
		t.Fatal("catalog")
	}
	pg := mustEngine(t, EnginePostgres)
	if len(pg.Versions) < 2 || !regexp.MustCompile(`^postgres:\d`).MatchString(pg.Image) || pg.Image != "postgres:18-alpine" {
		t.Fatalf("postgres %+v", pg)
	}
	pg.Versions[0].Tag = "mutated"
	if mustEngine(t, EnginePostgres).Versions[0].Tag != "18-alpine" {
		t.Fatal("GetEngine must return a copy")
	}
}

func TestResolveEngineImage(t *testing.T) {
	pg := mustEngine(t, EnginePostgres)
	got, err := ResolveEngineImage(pg, strp("15-alpine"), nil)
	if err != nil || got != (ResolvedImage{Image: "postgres:15-alpine", Tag: "15-alpine"}) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := ResolveEngineImage(pg, strp("nope"), nil); err == nil || !strings.HasPrefix(err.Error(), "Invalid version for PostgreSQL. Try one of: 18-alpine, 17-alpine") {
		t.Fatalf("err %v", err)
	}
	if got, _ := ResolveEngineImage(pg, nil, nil); got.Image != "postgres:18-alpine" {
		t.Fatalf("default %+v", got)
	}
	if got, _ := ResolveEngineImage(pg, strp(" postgres:19 "), nil); got.Tag != "19" {
		t.Fatalf("pattern match with repo prefix %+v", got)
	}
	many := make([]string, 13)
	for i := range many {
		many[i] = "t" + itoa(i)
	}
	if _, err := ResolveEngineImage(pg, strp("bad-tag"), many); err == nil || !strings.HasSuffix(err.Error(), "t11…") {
		t.Fatalf("err %v", err)
	}
}

func TestManagedServiceMarker(t *testing.T) {
	marker := ManagedServiceMarker("redis", "redis:7-alpine")
	if id, img, ok := ParseManagedServiceMarker(marker); !ok || id != "redis" || img != "redis:7-alpine" {
		t.Fatalf("%q %q %v", id, img, ok)
	}
	redis := mustEngine(t, EngineRedis)
	if ImageForManagedService(redis, marker) != "redis:7-alpine" {
		t.Fatal("pinned image")
	}
	// A bare marker pins the legacy image — never a silent major bump.
	if ImageForManagedService(redis, "[managed:redis]") != LegacyManagedImages[EngineRedis] || LegacyManagedImages[EngineRedis] == redis.Image {
		t.Fatal("legacy pin")
	}
	if ImageForManagedService(mustEngine(t, EnginePostgres), "[managed:postgres]") != "postgres:16-alpine" ||
		ImageForManagedService(mustEngine(t, EngineMongoDB), "[managed:mongodb]") != "mongo:7" {
		t.Fatal("legacy defaults")
	}
	for _, bad := range []string{"", "Dockerfile", "[managed:]", "[managed:a|b]x", "[managed:a|]"} {
		if _, _, ok := ParseManagedServiceMarker(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestVolumeMountForEngine(t *testing.T) {
	pg := mustEngine(t, EnginePostgres)
	for img, want := range map[string]string{
		"postgres:16-alpine": "/var/lib/postgresql/data", "postgres:17": "/var/lib/postgresql/data",
		"postgres:18-alpine": "/var/lib/postgresql", "postgres:18": "/var/lib/postgresql", "postgres:latest": "/var/lib/postgresql",
		"postgres:9.6": "/var/lib/postgresql/data", "postgres:custom": "/var/lib/postgresql/data", "": "/var/lib/postgresql",
	} {
		if got := VolumeMountForEngine(pg, img); got != want {
			t.Errorf("%q -> %s", img, got)
		}
	}
	if VolumeMountForEngine(mustEngine(t, EngineMySQL), "mysql:8.4") != "/var/lib/mysql" {
		t.Fatal("mysql")
	}
	if m, ok := MajorFromImageTag("18-alpine"); !ok || m != 18 {
		t.Fatal("major")
	}
	if _, ok := MajorFromImageTag("latest"); ok {
		t.Fatal("latest has no major")
	}
}

func TestEngineEnvAndURLs(t *testing.T) {
	pg := mustEngine(t, EnginePostgres)
	creds := Credentials{Username: "docklift", Password: "s3cret!", Database: "myapp"}
	if got := EngineRuntimeEnv(pg, creds); !reflect.DeepEqual(got, []EnvPair{{"POSTGRES_USER", "docklift"}, {"POSTGRES_PASSWORD", "s3cret!"}, {"POSTGRES_DB", "myapp"}}) {
		t.Fatalf("env %v", got)
	}
	if got := ConnectionURLFor(pg, "dl_myapp_aaaaaaaa_db", creds); got != "postgresql://docklift:s3cret!@dl_myapp_aaaaaaaa_db:5432/myapp" {
		t.Fatal(got)
	}
	redis := mustEngine(t, EngineRedis)
	rc := Credentials{Password: "p@ss", Database: "0"}
	if got := EngineCommand(redis, rc); !reflect.DeepEqual(got, []string{"redis-server", "--requirepass", "p@ss", "--appendonly", "yes"}) {
		t.Fatal(got)
	}
	if got := ConnectionURLFor(redis, "dbhost", rc); got != "redis://:p%40ss@dbhost:6379" {
		t.Fatal(got)
	}
	if EngineCommand(pg, creds) != nil {
		t.Fatal("postgres has no command")
	}
	mysql := mustEngine(t, EngineMySQL)
	dc := DefaultDatabaseCredentials("Shop App", "pw")
	back, ok := CredentialsFromEnv(mysql, envMap(EngineRuntimeEnv(mysql, dc)))
	if !ok || back != dc || dc.Database != "shop_app" || dc.Username != "docklift" {
		t.Fatalf("round trip %+v %+v", back, dc)
	}
	if _, ok := CredentialsFromEnv(pg, map[string]string{"POSTGRES_USER": "u", "POSTGRES_PASSWORD": ""}); ok {
		t.Fatal("incomplete credentials")
	}
	mongo := mustEngine(t, EngineMongoDB)
	if c, ok := CredentialsFromEnv(mongo, map[string]string{"MONGO_INITDB_ROOT_USERNAME": "u", "MONGO_INITDB_ROOT_PASSWORD": "p"}); !ok || c.Database != "admin" {
		t.Fatalf("mongo default db %+v", c)
	}
	if c, ok := CredentialsFromEnv(redis, map[string]string{"REDIS_PASSWORD": "x"}); !ok || c != (Credentials{Password: "x", Database: "0"}) {
		t.Fatal("redis creds")
	}
	if _, err := BuildConnectionURL(Engine{ID: "sqlite"}, "h", creds); err == nil {
		t.Fatal("unsupported engine")
	}
	if got := ManagedCredentialEnvKeys("mariadb"); len(got) != 4 || len(ManagedCredentialEnvKeys("x")) != 0 {
		t.Fatal("credential keys")
	}
	pw, err := GenerateDatabasePassword()
	if err != nil || !regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`).MatchString(pw) {
		t.Fatalf("password %q %v", pw, err)
	}
}

func envMap(pairs []EnvPair) map[string]string {
	m := map[string]string{}
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	return m
}
