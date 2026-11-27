package deploy

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestDockerSlug(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"My Service!", 20, "my-service"},
		{"Python Smoke", 24, "python-smoke"},
		{"", 24, "app"},
		{"!!!", 24, "app"},
		{"--a--b--", 24, "a-b"},
		{"ÄÖÜ shop", 24, "shop"},
		{"İstanbul", 24, "i-stanbul"}, // JS lowercases İ to "i̇"
		{"KELVINK", 24, "kelvink"},
		{"abcdefghijklmnopqrstuvwxyz", 24, "abcdefghijklmnopqrstuvwx"},
		{"abcdefghijklmnopqrstuvw-xyz", 24, "abcdefghijklmnopqrstuvw"},
		{"rocket 🚀 app", 24, "rocket-app"},
		{"UPPER_case.dots", 24, "upper-case-dots"},
	}
	for _, c := range cases {
		if got := DockerSlug(c.in, c.max); got != c.want {
			t.Errorf("DockerSlug(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestNames(t *testing.T) {
	id := "53b01966-1234-4abc-8def-0123456789ab"
	if got := ShortProjectID(id); got != "53b01966" {
		t.Errorf("ShortProjectID = %q", got)
	}
	if got := ComposeProjectName("Python Smoke", id); got != "dl-python-smoke-53b01966" {
		t.Errorf("ComposeProjectName = %q", got)
	}
	if got := ProjectNetworkName(id); got != "dl-net-53b01966" {
		t.Errorf("ProjectNetworkName = %q", got)
	}
	if got := ServiceContainerName("Python Smoke", id, "web-app"); got != "dl_python-smoke_53b01966_web_app" {
		t.Errorf("ServiceContainerName = %q", got)
	}
	long := ServiceContainerName(strings.Repeat("x", 80), id, strings.Repeat("s", 80))
	if len(long) != 57 || long != "dl_"+strings.Repeat("x", 24)+"_53b01966_"+strings.Repeat("s", 20) {
		t.Errorf("long container name %q (%d)", long, len(long))
	}
	if got := ComposeProjectAliases("Python Smoke", id); !reflect.DeepEqual(got, []string{"dl-python-smoke-53b01966", id}) {
		t.Errorf("aliases %v", got)
	}
	if got := ShortPathHash("a/Dockerfile"); !regexp.MustCompile(`^[a-f0-9]{4}$`).MatchString(got) {
		t.Errorf("ShortPathHash %q", got)
	}
	// compose.naming.test.ts
	if key := StorageVolumeComposeKey("My Service!", 0, "dl-abc123-data"); !regexp.MustCompile(`^storage_my-service_0_[a-f0-9]{4}$`).MatchString(key) {
		t.Errorf("StorageVolumeComposeKey %q", key)
	}
	if got := PersistentVolumeName(id, "data"); got != "dl-53b01966-data-"+ShortPathHash("data") {
		t.Errorf("PersistentVolumeName %q", got)
	}
	if PersistentVolumeName(id, "a-b") == PersistentVolumeName(id, "a_b") {
		t.Error("a-b and a_b must not collide")
	}
	want := []string{"com.docker.compose.project=dl-db-53b01966", "com.docklift.project=" + id, ManagedDatabaseVolumeLabel}
	if got := VolumeLabels("db", id, ManagedDatabaseVolumeLabel); !reflect.DeepEqual(got, want) {
		t.Errorf("VolumeLabels %v", got)
	}
	if got := DockliftImageTag("abcdef12-9999-4000-8000-000000000001", "Web App", "deadbeef-1111-4000-8000-000000000002"); got != "docklift-abcdef12-web-app:deadbeef" {
		t.Errorf("DockliftImageTag %q", got)
	}
}

type namingInput struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Service string `json:"service"`
	Deploy  string `json:"deploy"`
	Index   int    `json:"index"`
}

type namingOutput struct {
	Slug         string   `json:"slug"`
	Slug16       string   `json:"slug16"`
	Short        string   `json:"short"`
	Compose      string   `json:"compose"`
	Network      string   `json:"network"`
	Container    string   `json:"container"`
	Aliases      []string `json:"aliases"`
	Hash         string   `json:"hash"`
	StorageKey   string   `json:"storageKey"`
	ImageTag     string   `json:"imageTag"`
	IsAppImage   bool     `json:"isAppImage"`
	DefaultCreds struct {
		Database string `json:"database"`
	} `json:"creds"`
}

const namingScript = nodePrelude + `
const n = await load('lib/naming.js');
const img = await load('lib/imageCleanup.js');
const eng = await load('lib/databaseEngines.js');
done(input.map((c) => ({
  slug: n.dockerSlug(c.project),
  slug16: n.dockerSlug(c.project, 16),
  short: n.shortProjectId(c.id),
  compose: n.composeProjectName(c.project, c.id),
  network: n.projectNetworkName(c.id),
  container: n.serviceContainerName(c.project, c.id, c.service),
  aliases: n.composeProjectAliases(c.project, c.id),
  hash: n.shortPathHash(c.project),
  storageKey: n.storageVolumeComposeKey(c.service, c.index, c.project),
  imageTag: img.dockliftImageTag(c.id, c.service, c.deploy),
  isAppImage: img.isDockliftAppImage(img.dockliftImageTag(c.id, c.service, c.deploy)),
  creds: eng.defaultDatabaseCredentials(c.project, 'pw'),
})));
`

// TestNamingMatchesTypeScript runs the TS naming functions on awkward inputs
// and requires byte-identical results: existing installs have Docker objects
// named by these rules.
func TestNamingMatchesTypeScript(t *testing.T) {
	projects := []string{"Python Smoke", "", "   ", "My App!!", "İstanbul Ünïcode", "日本語のプロジェクト", "rocket 🚀🚀 launch",
		strings.Repeat("long-name-", 10), "a--b__c..d", "UPPER", "x", "-leading-and-trailing-", "123", "on", "Ǆemal", "ﬁle ligature",
		"tab\tname", "new\nline", "KKelvin", "ß straße", "Σίσυφος", "İİİ"}
	ids := []string{"53b01966-1234-4abc-8def-0123456789ab", "abcdef12-9999-4000-8000-000000000001", "a-b-c-d-e-f-g-h-i-j", "short"}
	services := []string{"app", "Web App", "a-b", "api_v2", strings.Repeat("svc", 20), "", "日本", "db"}
	var in []namingInput
	for i, p := range projects {
		in = append(in, namingInput{Project: p, ID: ids[i%len(ids)], Service: services[i%len(services)],
			Deploy: "deadbeef-1111-4000-8000-000000000002", Index: i})
	}
	var want []namingOutput
	decodeJSON(t, runNode(t, namingScript, in), &want)
	for i, c := range in {
		got := namingOutput{
			Slug: Slug(c.Project), Slug16: DockerSlug(c.Project, 16), Short: ShortProjectID(c.ID),
			Compose: ComposeProjectName(c.Project, c.ID), Network: ProjectNetworkName(c.ID),
			Container: ServiceContainerName(c.Project, c.ID, c.Service), Aliases: ComposeProjectAliases(c.Project, c.ID),
			Hash: ShortPathHash(c.Project), StorageKey: StorageVolumeComposeKey(c.Service, c.Index, c.Project),
			ImageTag: DockliftImageTag(c.ID, c.Service, c.Deploy),
		}
		got.IsAppImage = IsDockliftAppImage(got.ImageTag)
		got.DefaultCreds.Database = DefaultDatabaseCredentials(c.Project, "pw").Database
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("input %+v\n got  %+v\n want %+v", c, got, want[i])
		}
	}
}
