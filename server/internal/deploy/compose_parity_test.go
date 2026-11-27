package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// awkwardValues are env values that trip YAML emitters and parsers.
var awkwardValues = []string{
	"", " ", "  leading", "trailing  ", " both ",
	"1_000", "0123", "0x1F", "0o17", "1e3", "-1", "+1", ".5", "1.", "12:30", "12:30:45", "190:20:30", "5500:22",
	"yes", "no", "on", "off", "y", "n", "Y", "N", "Yes", "NO", "On", "OFF", "true", "false", "True", "FALSE",
	"null", "Null", "NULL", "~", ".nan", ".inf", "-.inf", ".NaN",
	"2026-01-02", "2026-01-02T03:04:05Z", "2001-12-14 21:59:43.10 -5",
	"key: value", "a:b", "a: ", "- item", "? q", "[1, 2]", "{a: 1}", "a, b",
	"https://user:pa:ss@example.com:8443/p?q=1#frag", "redis://:p%40ss@host:6379", "#comment", "a #b", "a#b",
	`"double"`, `'single'`, `it's`, `say "hi"`, `back\slash`, `\n literal`, `C:\Users\x`,
	"$", "$$", "${VAR}", "$VAR", "$${ESCAPED}", "${VAR:-default}", "a$b",
	"$2b$12$C6UzMDM.H6dfI/f/IKcEeO5oMvOcMNHiE7uqlUU5J6JESl3aM2K6q",
	"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\nBKcwggSjAgEAAoIBAQC7\n-----END PRIVATE KEY-----\n",
	"line1\nline2", "trailing newline\n", "\nleading newline", "two\n\nblank", "  indented\n  block",
	"tab\tinside", "\ttab-first", "tab-last\t",
	"unicode é ü ß 中文", "emoji 🚀🔥", "zero\u200bwidth", "nbsp\u00a0x", "bom\ufeffx", "ls\u2028ps\u2029",
	`{"json":true,"n":[1,2,{"k":"v"}]}`, `[{"a":1}]`,
	"*anchor", "&ref", "!tag", "|pipe", ">folded", "%percent", "-dash", "?question", "@at", "#hash", "`backtick", ",comma", "[bracket", "{brace",
	strings.Repeat("x", 300), strings.Repeat("word ", 60),
	"crlf\r\nline", "cr\ronly", "ctrl\x01\x1b[0m", "nul\x00byte", "del\x7f",
	"=", "a=b=c", "...", "---", "--- doc", "... end",
}

type parityService struct {
	Name          string         `json:"name"`
	Image         string         `json:"image"`
	InternalPort  int            `json:"internal_port"`
	Port          *int           `json:"port"`
	ContainerName string         `json:"container_name"`
	Volumes       []parityVolume `json:"volumes,omitempty"`
	Command       []string       `json:"command,omitempty"`
	InjectPortEnv *bool          `json:"injectPortEnv,omitempty"`
}

type parityVolume struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
}

type parityEnv struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsBuildArg  bool   `json:"is_build_arg"`
	IsRuntime   bool   `json:"is_runtime"`
	ServiceName string `json:"service_name"`
}

type parityOptions struct {
	ProjectID       string   `json:"projectId"`
	PublishHostPort bool     `json:"publishHostPort"`
	MemLimit        string   `json:"memLimit,omitempty"`
	CPUs            *float64 `json:"cpus,omitempty"`
}

type parityCase struct {
	Services []parityService `json:"services"`
	Env      []parityEnv     `json:"env"`
	Options  parityOptions   `json:"options"`
}

func ptr[T any](v T) *T { return &v }

func parityFixture() []parityCase {
	var env []parityEnv
	for i, v := range awkwardValues {
		env = append(env, parityEnv{Key: fmt.Sprintf("V_%03d", i), Value: v, IsRuntime: true})
	}
	env = append(env,
		parityEnv{Key: "BUILD_ONLY", Value: "x", IsBuildArg: true, IsRuntime: false},
		parityEnv{Key: "SHARED", Value: "shared", IsRuntime: true},
		parityEnv{Key: "SHARED", Value: "api override", IsRuntime: true, ServiceName: "api"},
		parityEnv{Key: "PORT", Value: "9999", IsRuntime: true},
		parityEnv{Key: "WEB_ONLY", Value: "0777", IsRuntime: true, ServiceName: "web"},
	)
	pid := "abcdef12-3456-4789-8abc-def012345678"
	return []parityCase{
		{
			Services: []parityService{
				{Name: "api", Image: "docklift-abcdef12-api:deadbeef", InternalPort: 22, Port: ptr(5500), ContainerName: "dl_shop_abcdef12_api",
					Volumes: []parityVolume{{Key: "storage_api_0_1a2b", Name: "dl-abcdef12-data-1a2b", MountPath: "/app/data"}, {Key: "storage_api_1_ffff", Name: "dl-abcdef12-cache-ffff", MountPath: "/var/lib/x y"}}},
				{Name: "web", Image: "docklift-abcdef12-web:deadbeef", InternalPort: 59, Port: ptr(5501), ContainerName: "dl_shop_abcdef12_web"},
				{Name: "123", Image: "node:20", InternalPort: 3000, Port: nil, ContainerName: "dl_shop_abcdef12_123"},
				{Name: "on", Image: "nginx:1.27", InternalPort: 60, Port: ptr(5600), ContainerName: "dl_shop_abcdef12_on",
					Command: append([]string{"redis-server", "--requirepass"}, awkwardValues[:40]...)},
			},
			Env:     env,
			Options: parityOptions{ProjectID: pid, PublishHostPort: true, MemLimit: "512m", CPUs: ptr(0.5)},
		},
		{
			Services: []parityService{{Name: "db", Image: "postgres:18-alpine", InternalPort: 5432, Port: ptr(5432), ContainerName: "dl_db_abcdef12_db",
				InjectPortEnv: ptr(false), Command: []string{"postgres", "-c", "max_connections=200"},
				Volumes: []parityVolume{{Key: "storage_db_0_aaaa", Name: "dl-abcdef12-data-aaaa", MountPath: "/var/lib/postgresql"}}}},
			Env:     []parityEnv{{Key: "POSTGRES_PASSWORD", Value: "p@ss:w#rd 'x' \"y\"", IsRuntime: true}, {Key: "EMPTY", Value: "", IsRuntime: true}},
			Options: parityOptions{ProjectID: pid, PublishHostPort: false, CPUs: ptr(2.0)},
		},
		{
			Services: []parityService{{Name: "app", Image: "node:20", InternalPort: 3000, ContainerName: "dl_test_app"}},
			Options:  parityOptions{},
		},
	}
}

func (c parityCase) goInputs() ([]RuntimeService, []EnvVar, RuntimeComposeOptions) {
	var svcs []RuntimeService
	for _, s := range c.Services {
		rs := RuntimeService{Name: s.Name, Image: s.Image, InternalPort: s.InternalPort, Port: s.Port, ContainerName: s.ContainerName,
			Command: s.Command, NoPortEnv: s.InjectPortEnv != nil && !*s.InjectPortEnv}
		for _, v := range s.Volumes {
			rs.Volumes = append(rs.Volumes, RuntimeVolume{Key: v.Key, Name: v.Name, MountPath: v.MountPath})
		}
		svcs = append(svcs, rs)
	}
	var env []EnvVar
	for _, e := range c.Env {
		env = append(env, EnvVar{Key: e.Key, Value: e.Value, ServiceName: e.ServiceName, IsBuildArg: e.IsBuildArg, IsRuntime: e.IsRuntime})
	}
	return svcs, env, RuntimeComposeOptions{ProjectID: c.Options.ProjectID, PublishHostPort: c.Options.PublishHostPort, MemLimit: c.Options.MemLimit, CPUs: c.Options.CPUs}
}

const composeScript = nodePrelude + `
import os from 'os';
import path from 'path';
const { generateRuntimeCompose } = await load('services/compose.js');
const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'parity-'));
const out = [];
input.forEach((c, i) => {
  const p = path.join(dir, 'c' + i + '.yml');
  const opts = { ...c.options };
  if (!opts.projectId) delete opts.projectId;
  generateRuntimeCompose(p, c.services, c.env || undefined, opts);
  out.push(fs.readFileSync(p, 'utf8'));
});
fs.rmSync(dir, { recursive: true, force: true });
done(out);
`

// TestRuntimeComposeMatchesTypeScript generates the same fixtures with the
// TS generateRuntimeCompose (js-yaml dump) and with RenderRuntimeCompose and
// compares the parsed documents node by node: same structure, same resolved
// tags, same values.
func TestRuntimeComposeMatchesTypeScript(t *testing.T) {
	cases := parityFixture()
	var tsDocs []string
	decodeJSON(t, runNode(t, composeScript, cases), &tsDocs)
	if len(tsDocs) != len(cases) {
		t.Fatalf("node returned %d documents", len(tsDocs))
	}
	for i, c := range cases {
		svcs, env, opts := c.goInputs()
		goDoc, err := RenderRuntimeCompose(svcs, env, opts)
		if err != nil {
			t.Fatal(err)
		}
		var tsNode, goNode yaml.Node
		if err := yaml.Unmarshal([]byte(tsDocs[i]), &tsNode); err != nil {
			t.Fatalf("case %d: parse TS output: %v", i, err)
		}
		if err := yaml.Unmarshal(goDoc, &goNode); err != nil {
			t.Fatalf("case %d: parse Go output: %v\n%s", i, err, goDoc)
		}
		if diff := compareNodes("$", &tsNode, &goNode); diff != "" {
			t.Errorf("case %d: %s\n--- TS ---\n%s\n--- Go ---\n%s", i, diff, tsDocs[i], goDoc)
		}
		var tsAny, goAny any
		if err := yaml.Unmarshal([]byte(tsDocs[i]), &tsAny); err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(goDoc, &goAny); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tsAny, goAny) {
			t.Errorf("case %d: decoded documents differ", i)
		}
	}
}

// TestRuntimeComposeRoundTripsEveryValue checks independently of node that
// every awkward env value and command argument parses back as the identical
// string.
func TestRuntimeComposeRoundTripsEveryValue(t *testing.T) {
	c := parityFixture()[0]
	svcs, env, opts := c.goInputs()
	out, err := RenderRuntimeCompose(svcs, env, opts)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]any `yaml:"environment"`
			Command     []any          `yaml:"command"`
			Ports       []any          `yaml:"ports"`
			Cpus        any            `yaml:"cpus"`
		} `yaml:"services"`
		Volumes map[string]struct {
			External any `yaml:"external"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	api := doc.Services["api"]
	for i, v := range awkwardValues {
		key := fmt.Sprintf("V_%03d", i)
		got, ok := api.Environment[key].(string)
		want := strings.ToValidUTF8(v, "\uFFFD")
		if !ok || got != want {
			t.Errorf("%s: got %#v, want %q", key, api.Environment[key], want)
		}
	}
	if api.Environment["PORT"] != "22" || api.Environment["SHARED"] != "api override" || api.Environment["BUILD_ONLY"] != nil {
		t.Errorf("env PORT=%#v SHARED=%#v BUILD_ONLY=%#v", api.Environment["PORT"], api.Environment["SHARED"], api.Environment["BUILD_ONLY"])
	}
	if !reflect.DeepEqual(api.Ports, []any{"5500:22"}) || !reflect.DeepEqual(doc.Services["web"].Ports, []any{"5501:59"}) {
		t.Errorf("ports %#v %#v", api.Ports, doc.Services["web"].Ports)
	}
	if doc.Services["123"].Ports != nil {
		t.Error("service without host port must not publish")
	}
	on := doc.Services["on"]
	for i, v := range on.Command[2:] {
		if s, ok := v.(string); !ok || s != strings.ToValidUTF8(awkwardValues[i], "\uFFFD") {
			t.Errorf("command[%d] = %#v", i+2, v)
		}
	}
	if api.Cpus != "0.5" || doc.Volumes["storage_api_0_1a2b"].External != true {
		t.Errorf("cpus %#v external %#v", api.Cpus, doc.Volumes["storage_api_0_1a2b"].External)
	}
}

func TestWriteRuntimeCompose(t *testing.T) {
	dir := t.TempDir()
	p := RuntimeComposePath(dir, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if want := filepath.Join(dir, ".docklift", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "compose.yml"); p != want {
		t.Fatalf("path %s", p)
	}
	svcs := []RuntimeService{{Name: "app", Image: "node:20", InternalPort: 3000, ContainerName: "dl_test_app"}}
	if err := WriteRuntimeCompose(p, svcs, nil, RuntimeComposeOptions{ProjectID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var generated struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(b, &generated); err != nil {
		t.Fatal(err)
	}
	app := generated.Services["app"]
	// compose.defaults.test.ts: no cap_drop, no hard mem/cpu caps by default.
	for _, k := range []string{"cap_drop", "mem_limit", "cpus", "ports", "volumes", "command"} {
		if _, ok := app[k]; ok {
			t.Errorf("unexpected %s", k)
		}
	}
	if !reflect.DeepEqual(app["security_opt"], []any{"no-new-privileges:true"}) {
		t.Errorf("security_opt %#v", app["security_opt"])
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

// compareNodes compares two parsed YAML trees: kinds, resolved tags and
// values must match; mappings are compared as unordered key sets.
func compareNodes(path string, a, b *yaml.Node) string {
	for a.Kind == yaml.DocumentNode || a.Kind == yaml.AliasNode {
		if a.Kind == yaml.AliasNode {
			a = a.Alias
		} else {
			a = a.Content[0]
		}
	}
	for b.Kind == yaml.DocumentNode || b.Kind == yaml.AliasNode {
		if b.Kind == yaml.AliasNode {
			b = b.Alias
		} else {
			b = b.Content[0]
		}
	}
	if a.Kind != b.Kind {
		return fmt.Sprintf("%s: kind %v vs %v", path, a.Kind, b.Kind)
	}
	switch a.Kind {
	case yaml.ScalarNode:
		if a.ShortTag() != b.ShortTag() || a.Value != b.Value {
			return fmt.Sprintf("%s: %s %q vs %s %q", path, a.ShortTag(), a.Value, b.ShortTag(), b.Value)
		}
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return fmt.Sprintf("%s: %d vs %d items", path, len(a.Content), len(b.Content))
		}
		for i := range a.Content {
			if d := compareNodes(fmt.Sprintf("%s[%d]", path, i), a.Content[i], b.Content[i]); d != "" {
				return d
			}
		}
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return fmt.Sprintf("%s: %d vs %d keys", path, len(a.Content)/2, len(b.Content)/2)
		}
		bm := map[string][2]*yaml.Node{}
		for i := 0; i < len(b.Content); i += 2 {
			bm[b.Content[i].ShortTag()+" "+b.Content[i].Value] = [2]*yaml.Node{b.Content[i], b.Content[i+1]}
		}
		for i := 0; i < len(a.Content); i += 2 {
			k := a.Content[i].ShortTag() + " " + a.Content[i].Value
			other, ok := bm[k]
			if !ok {
				return fmt.Sprintf("%s: key %s missing in Go output", path, k)
			}
			if d := compareNodes(path+"."+a.Content[i].Value, a.Content[i+1], other[1]); d != "" {
				return d
			}
		}
	}
	return ""
}

func TestCompareNodesDetectsDifferences(t *testing.T) {
	parse := func(s string) *yaml.Node {
		var n yaml.Node
		if err := yaml.Unmarshal([]byte(s), &n); err != nil {
			t.Fatal(err)
		}
		return &n
	}
	for _, c := range [][2]string{
		{"a: '1'", "a: 1"},
		{"a: 'yes'", "a: \"no\""},
		{"a: [x]", "a: [x, y]"},
		{"a: {b: c}", "a: {b: c, d: e}"},
		{"a: {b: c}", "a: {x: c}"},
		{"a: true", "a: 'true'"},
	} {
		if compareNodes("$", parse(c[0]), parse(c[1])) == "" {
			t.Errorf("%q vs %q compared equal", c[0], c[1])
		}
	}
	if d := compareNodes("$", parse("a: {b: c, d: [1]}"), parse("{d: [1], b: c}")); d == "" {
		t.Error("different nesting compared equal")
	}
	if d := compareNodes("$", parse("a: {b: c, d: [1]}"), parse("a: {d: [1], \"b\": \"c\"}")); d != "" {
		t.Errorf("key order and quoting must not matter: %s", d)
	}
}
