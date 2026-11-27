package projects

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClampPort(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{"8080", 8080}, {"70000", 65535}, {"abc", 3000}, {"0", 3000}, {"-4", 1}, {"443.9", 443},
		{"  12x", 12}, {"+7", 7}, {"-0", 3000}, {"", 3000}, {nil, 3000}, {true, 3000},
		{json.Number("99999"), 65535}, {json.Number("1e2"), 100}, {json.Number("1e21"), 1},
		{json.Number("-5"), 1}, {[]any{"80", "90"}, 80}, {"99999999999999999999999", 65535},
	}
	for _, c := range cases {
		if got := clampPort(val{v: c.in, defined: true}); got != c.want {
			t.Errorf("clampPort(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := clampPort(val{}); got != 3000 {
		t.Errorf("undefined -> %d", got)
	}
}

func TestValString(t *testing.T) {
	if got := (val{}).jsString(); got != "undefined" {
		t.Fatal(got)
	}
	if got := (val{v: json.Number("8080"), defined: true}).jsString(); got != "8080" {
		t.Fatal(got)
	}
}

func TestTruthy(t *testing.T) {
	truthy := []any{"x", true, json.Number("1"), json.Number("-0.5"), []any{}, map[string]any{}}
	falsy := []any{nil, "", false, json.Number("0"), json.Number("-0")}
	for _, v := range truthy {
		if !(val{v: v, defined: true}).truthy() {
			t.Errorf("%#v should be truthy", v)
		}
	}
	for _, v := range falsy {
		if (val{v: v, defined: true}).truthy() {
			t.Errorf("%#v should be falsy", v)
		}
	}
	if (val{}).truthy() {
		t.Error("undefined is falsy")
	}
}

func TestBaseDirectoryAndDockerfilePath(t *testing.T) {
	for in, want := range map[any]string{"  src  ": "src", "": ".", "   ": ".", nil: ".", json.Number("5"): "5"} {
		if got := baseDirectory(val{v: in, defined: true}); got != want {
			t.Errorf("baseDirectory(%#v) = %q, want %q", in, got, want)
		}
	}
	if got := dockerfilePath(val{v: " docker/Dockerfile ", defined: true}); got == nil || *got != "docker/Dockerfile" {
		t.Errorf("got %v", got)
	}
	// A truthy blank string becomes "" (String(v).trim()), not null.
	if got := dockerfilePath(val{v: "  ", defined: true}); got == nil || *got != "" {
		t.Errorf("blank -> %v", got)
	}
	for _, v := range []any{"", nil, false} {
		if got := dockerfilePath(val{v: v, defined: true}); got != nil {
			t.Errorf("%#v -> %v", v, *got)
		}
	}
}

func TestValidDomainList(t *testing.T) {
	good := []string{"", "example.com", "App.Contract-Test.dev, www.contract-test.dev", "a.b,,c.d", " a.io "}
	bad := []string{"bad domain", "a.com;evil", "localhost", "-bad.example.test", "bad-.example.test", "bad_domain", "a..b", "a.b, x"}
	for _, s := range good {
		if !validDomainList(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if validDomainList(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestValidGithubRepoURL(t *testing.T) {
	good := []string{"https://github.com/a/b", "http://github.com/a/b.git", "https://www.github.com/a/b/tree/main",
		"HTTPS://GitHub.com/a/b", "https://github.com:443/a/b", "https://github.com/a/./b"}
	bad := []string{"https://gitlab.com/a/b", "https://github.com/only-owner", "https://github.com/owner.git", "git@github.com:a/b.git",
		"ftp://github.com/a/b", "https://github.com.evil.test/a/b", "https://x-access-token:tok@github.com/a/b",
		"https://user@github.com/a/b", "https://github.com/a/..", "not a url", "https:github.com/a/b"}
	for _, s := range good {
		if !validGithubRepoURL(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if validGithubRepoURL(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestValidateMountPath(t *testing.T) {
	ok := map[any]string{"/data": "/data", "/data///": "/data", " /var/lib/x ": "/var/lib/x", "/etcetera/data/": "/etcetera/data",
		"/var/runner": "/var/runner"}
	for in, want := range ok {
		got, err := validateMountPath(val{v: in, defined: true})
		if err != nil || got != want {
			t.Errorf("%#v -> %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []any{"relative/path", "C:/data", "/data:ro", "/da\x00ta", json.Number("5")} {
		if _, err := validateMountPath(val{v: in, defined: true}); err != errMountNotAbsolute {
			t.Errorf("%#v -> %v", in, err)
		}
	}
	for _, in := range []any{nil, "", "/", "//", "/etc", "/etc/", "/proc/1", "/sys", "/dev/shm", "/var/run", "/var/run/docker.sock"} {
		if _, err := validateMountPath(val{v: in, defined: true}); err != errMountBlocked {
			t.Errorf("%#v -> %v", in, err)
		}
	}
}

func TestParseDotenv(t *testing.T) {
	content := "# comment\r\nA=1\r\n  B = \"two words\"  \nA=dup\nC='x=y'\nEMPTY=\nNOEQ\n\n"
	pairs, _, ok := parseDotenv(content)
	if !ok {
		t.Fatal("unexpected invalid key")
	}
	var got []string
	for _, p := range pairs {
		got = append(got, p.key+"="+p.value)
	}
	if want := "A=1|B=two words|C=x=y|EMPTY="; strings.Join(got, "|") != want {
		t.Fatalf("got %q", strings.Join(got, "|"))
	}
	for in, bad := range map[string]string{"OK=1\n1BAD=2": "1BAD", "export FOO=bar": "export FOO", "=novalue": ""} {
		if _, key, ok := parseDotenv(in); ok || key != bad {
			t.Errorf("%q -> %q %v", in, key, ok)
		}
	}
}
