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
