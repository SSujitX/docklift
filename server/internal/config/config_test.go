package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestDefaults(t *testing.T) {
	c, err := FromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Port", c.Port, 8000},
		{"DataPath", c.DataPath, mustAbs(t, "data")},
		{"DeploymentsPath", c.DeploymentsPath, mustAbs(t, "deployments")},
		{"BackupPath", c.BackupPath, mustAbs(t, filepath.Join("data", "backups"))},
		{"DockerNetwork", c.DockerNetwork, "docklift_network"},
		{"GitHubPrivateKeyPath", c.GitHubPrivateKeyPath, "./github-app.pem"},
		{"FrontendURL", c.FrontendURL, "http://localhost:3000"},
		{"NginxConfPath", c.NginxConfPath, mustAbs(t, filepath.Join("nginx-proxy", "conf.d"))},
		{"PortRangeStart", c.PortRangeStart, 5500},
		{"PortRangeEnd", c.PortRangeEnd, 5600},
		{"LetsEncryptPath", c.LetsEncryptPath, mustAbs(t, filepath.Join("nginx-proxy", "certbot", "conf"))},
		{"CertbotContainer", c.CertbotContainer, "docklift-certbot"},
		{"CertbotStaging", c.CertbotStaging, false},
		{"NginxProxyContainer", c.NginxProxyContainer, "docklift-nginx-proxy"},
		{"TerminalAllowNoOrigin", c.TerminalAllowNoOrigin, false},
		{"CORSOrigins", c.CORSOrigins, []string(nil)},
		{"DatabaseURL", c.DatabaseURL, DefaultDatabaseURL},
		{"DatabasePath", c.DatabasePath, filepath.Clean("/app/data/docklift.db")},
	}
	for _, ch := range checks {
		if !reflect.DeepEqual(ch.got, ch.want) {
			t.Errorf("%s = %#v, want %#v", ch.name, ch.got, ch.want)
		}
	}
}

func TestOverrides(t *testing.T) {
	dir := t.TempDir()
	c, err := FromEnv(env(map[string]string{
		"PORT":                             "9001",
		"DATA_PATH":                        filepath.Join(dir, "d"),
		"DEPLOYMENTS_PATH":                 filepath.Join(dir, "deps"),
		"DOCKER_NETWORK":                   "net1",
		"DOCKLIFT_GITHUB_APP_ID":           "42",
		"DOCKLIFT_GITHUB_CLIENT_ID":        "cid",
		"DOCKLIFT_GITHUB_CLIENT_SECRET":    "cs",
		"DOCKLIFT_GITHUB_PRIVATE_KEY_PATH": "keys/app.pem",
		"DOCKLIFT_FRONTEND_URL":            "https://panel.example.com",
		"PORT_RANGE_START":                 "7000",
		"PORT_RANGE_END":                   "7100",
		"CERTBOT_EMAIL":                    "ops@example.com",
		"CERTBOT_STAGING":                  "1",
		"CORS_ORIGIN":                      "https://a.example, https://b.example",
		"NGINX_PROXY_CONTAINER":            "proxy",
		"TERMINAL_ALLOW_NO_ORIGIN":         "true",
		"JWT_SECRET":                       "j",
		"INTERNAL_API_SECRET":              "i",
		"DATABASE_URL":                     "file:" + filepath.ToSlash(filepath.Join(dir, "x.db")) + "?connection_limit=1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9001 || c.PortRangeStart != 7000 || c.PortRangeEnd != 7100 {
		t.Errorf("ports = %d %d %d", c.Port, c.PortRangeStart, c.PortRangeEnd)
	}
	if c.BackupPath != filepath.Join(dir, "d", "backups") {
		t.Errorf("BackupPath = %s, want default under DATA_PATH", c.BackupPath)
	}
	if c.GitHubPrivateKeyPath != "keys/app.pem" {
		t.Errorf("GitHubPrivateKeyPath must not be resolved, got %s", c.GitHubPrivateKeyPath)
	}
	if !c.CertbotStaging || !c.TerminalAllowNoOrigin {
		t.Errorf("booleans not parsed: staging=%v noOrigin=%v", c.CertbotStaging, c.TerminalAllowNoOrigin)
	}
	if want := []string{"https://a.example", " https://b.example"}; !reflect.DeepEqual(c.CORSOrigins, want) {
		t.Errorf("CORSOrigins = %#v, want %#v", c.CORSOrigins, want)
	}
	if c.DatabasePath != filepath.Join(dir, "x.db") {
		t.Errorf("DatabasePath = %s", c.DatabasePath)
	}
	if c.JWTSecret != "j" || c.InternalAPISecret != "i" || c.GitHubAppID != "42" || c.CertbotEmail != "ops@example.com" {
		t.Errorf("string settings not read: %+v", c)
	}
}

func TestBooleanFlagsAreStrict(t *testing.T) {
	for _, v := range []string{"TRUE", "yes", "0", "false"} {
		c, err := FromEnv(env(map[string]string{"CERTBOT_STAGING": v, "TERMINAL_ALLOW_NO_ORIGIN": v}))
		if err != nil {
			t.Fatal(err)
		}
		if c.CertbotStaging || c.TerminalAllowNoOrigin {
			t.Errorf("%q must not enable the flags", v)
		}
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		in      string
		radix10 bool
		want    int
		wantErr bool
	}{
		{"8000", false, 8000, false},
		{"  8080abc", false, 8080, false},
		{"0x1F", false, 31, false},
		{"0x1F", true, 0, false},
		{"-5", true, -5, false},
		{"+7", true, 7, false},
		{"abc", false, 0, true},
		{"", false, 0, true},
		{"99999999999", false, 0, true},
	}
	for _, c := range cases {
		got, err := parseInt("X", c.in, c.radix10)
		if (err != nil) != c.wantErr || (err == nil && got != c.want) {
			t.Errorf("parseInt(%q, %v) = %d, %v; want %d, err=%v", c.in, c.radix10, got, err, c.want, c.wantErr)
		}
	}
	if _, err := FromEnv(env(map[string]string{"PORT": "http"})); err == nil {
		t.Error("invalid PORT must fail")
	}
}

func TestDatabasePathFromURL(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "a.db")
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"file:" + abs, abs, false},
		{"file:" + abs + "?socket_timeout=10", abs, false},
		{"file:./data/docklift.db", mustAbs(t, filepath.Join("prisma", "data", "docklift.db")), false},
		{"file:dev.db", mustAbs(t, filepath.Join("prisma", "dev.db")), false},
		{"file:../x.db", mustAbs(t, "x.db"), false},
		{"postgres://x", "", true},
		{"file:", "", true},
	}
	for _, c := range cases {
		got, err := DatabasePathFromURL(c.url)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("DatabasePathFromURL(%q) = %q, %v; want %q, err=%v", c.url, got, err, c.want, c.wantErr)
		}
	}
}

func TestParseDotenv(t *testing.T) {
	src := strings.Join([]string{
		"# comment",
		"DATABASE_URL=\"file:./data/docklift.db\"",
		"PORT=8000 # trailing comment",
		"export EXPORTED=yes",
		"EMPTY=",
		"SINGLE='a # not a comment'",
		"DOUBLE=\"line1\\nline2\"",
		"BACKTICK=`tick`",
		"MULTI=\"first",
		"second\"",
		"COLON: value",
		"SPACED =  padded  ",
		"DUP=1",
		"DUP=2",
		"CRLF=value\r",
	}, "\n")
	got := map[string]string{}
	var order []string
	for _, kv := range ParseDotenv(src) {
		got[kv.Key] = kv.Value
		order = append(order, kv.Key)
	}
	want := map[string]string{
		"DATABASE_URL": "file:./data/docklift.db",
		"PORT":         "8000",
		"EXPORTED":     "yes",
		"EMPTY":        "",
		"SINGLE":       "a # not a comment",
		"DOUBLE":       "line1\nline2",
		"BACKTICK":     "tick",
		"MULTI":        "first\nsecond",
		"COLON":        "value",
		"SPACED":       "padded",
		"DUP":          "2",
		"CRLF":         "value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDotenv:\n got %#v\nwant %#v", got, want)
	}
	if order[0] != "DATABASE_URL" {
		t.Errorf("order not preserved: %v", order)
	}
}

func TestLoadDotenvOverrideRules(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".env", "DL_TEST_BASE=from-env\nDL_TEST_PRESET=from-env\nDL_TEST_LOCAL=from-env\n")
	write(".env.local", "DL_TEST_LOCAL=from-local\nDL_TEST_PRESET2=from-local\n")
	t.Setenv("DL_TEST_PRESET", "process")
	t.Setenv("DL_TEST_PRESET2", "process")
	t.Setenv("DL_TEST_BASE", "")
	os.Unsetenv("DL_TEST_BASE")
	t.Setenv("DL_TEST_LOCAL", "")
	os.Unsetenv("DL_TEST_LOCAL")

	files, err := LoadDotenv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("applied files = %v", files)
	}
	for k, want := range map[string]string{
		"DL_TEST_BASE":    "from-env",   // .env fills unset variables
		"DL_TEST_PRESET":  "process",    // .env never overrides
		"DL_TEST_LOCAL":   "from-local", // .env.local overrides .env
		"DL_TEST_PRESET2": "from-local", // .env.local overrides the process env
	} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	empty := t.TempDir()
	if files, err := LoadDotenv(empty); err != nil || len(files) != 0 {
		t.Errorf("missing files must be ignored: %v %v", files, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Mkdir(filepath.Join(empty, ".env"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDotenv(empty); err == nil {
			t.Error("unreadable .env must be reported")
		}
	}
}
