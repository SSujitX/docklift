package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanDockerfilesDedupesCollidingNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "b", "Dockerfile"), "FROM scratch\nEXPOSE 3000\n")
	writeFile(t, filepath.Join(root, "a-b", "Dockerfile"), "FROM scratch\nEXPOSE 4000\n")
	services, err := ScanDockerfiles(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 {
		t.Fatalf("services %+v", services)
	}
	names := []string{services[0].Name, services[1].Name}
	hashed := regexp.MustCompile(`^a-b-[a-f0-9]{4}$`)
	if names[0] == names[1] || !(names[0] == "a-b" && hashed.MatchString(names[1]) || names[1] == "a-b" && hashed.MatchString(names[0])) {
		t.Fatalf("names %v", names)
	}
	// Byte-ordered traversal: "a" (a/b) before "a-b".
	if services[0].Name != "a-b" || services[0].InternalPort != 3000 || services[1].InternalPort != 4000 {
		t.Fatalf("order/ports %+v", services)
	}
	if services[1].Name != "a-b-"+ShortPathHash(filepath.Join("a-b", "Dockerfile")) {
		t.Fatalf("hash suffix %q", services[1].Name)
	}
}

func TestDedupeScannedServicesRejectsPersistentDuplicates(t *testing.T) {
	df := "a/Dockerfile"
	services := []ScannedService{
		{Name: "app", DockerfilePath: df, ContextPath: "a", InternalPort: 3000},
		{Name: "app-" + ShortPathHash(df), DockerfilePath: "b/Dockerfile", ContextPath: "b", InternalPort: 3000},
		{Name: "app", DockerfilePath: df, ContextPath: "a", InternalPort: 3000},
	}
	if _, err := DedupeScannedServices(services); err == nil || !strings.Contains(err.Error(), "Duplicate service name") {
		t.Fatalf("err %v", err)
	}
	if out, err := DedupeScannedServices(nil); err != nil || len(out) != 0 {
		t.Fatalf("empty: %v %v", out, err)
	}
}

func TestScanDockerfilesRejectsReservedNames(t *testing.T) {
	for _, dir := range []string{"docklift-backend", "Docklift-Nginx-Proxy", "docklift-certbot", "docklift", filepath.Join("docklift-backend.docklift_network", "x")} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "web", "Dockerfile"), "FROM x\n")
			writeFile(t, filepath.Join(root, dir, "Dockerfile"), "FROM x\n")
			services, err := ScanDockerfiles(root, 2)
			var reserved *ReservedServiceNameError
			if !errors.As(err, &reserved) || services != nil {
				t.Fatalf("services %+v err %v", services, err)
			}
			if !strings.Contains(err.Error(), "is reserved") || !strings.Contains(err.Error(), filepath.ToSlash(filepath.Join(dir, "Dockerfile"))) {
				t.Fatalf("message %q", err)
			}
		})
	}
	// Names that merely contain the word stay allowed.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "my-docklift-app", "Dockerfile"), "FROM x\n")
	if services, err := ScanDockerfiles(root, 2); err != nil || len(services) != 1 {
		t.Fatalf("services %+v err %v", services, err)
	}
}

func TestRenderRuntimeComposeRejectsReservedNames(t *testing.T) {
	_, err := RenderRuntimeCompose([]RuntimeService{{Name: "docklift-backend", Image: "x", InternalPort: 8000, ContainerName: "dl_p_1234abcd_docklift-backend"}}, nil, RuntimeComposeOptions{ProjectID: "p"})
	var reserved *ReservedServiceNameError
	if !errors.As(err, &reserved) {
		t.Fatalf("err %v", err)
	}
}

func TestScanDockerfilesRules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Dockerfile"), "FROM node\n")
	writeFile(t, filepath.Join(root, "node_modules", "x", "Dockerfile"), "FROM x\n")
	writeFile(t, filepath.Join(root, "dist", "Dockerfile"), "FROM x\n")
	writeFile(t, filepath.Join(root, "one", "two", "Dockerfile"), "FROM python\nRUN pip install flask\n")
	writeFile(t, filepath.Join(root, "one", "two", "three", "Dockerfile"), "FROM x\n") // depth 3: too deep
	writeFile(t, filepath.Join(root, "svc", "dockerfile"), "FROM x\n")                 // case-sensitive name
	writeFile(t, filepath.Join(root, "api", "Dockerfile"), "# expose 9000 in prod\nFROM x\nexpose 8080\n")
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "Dockerfile"), "FROM evil\n")
	linked := true
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		linked = false
	}
	services, err := ScanDockerfiles(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ScannedService{}
	for _, s := range services {
		got[s.Name] = s
	}
	want := map[string]ScannedService{
		"app":     {Name: "app", DockerfilePath: "Dockerfile", ContextPath: ".", InternalPort: 3000},
		"one-two": {Name: "one-two", DockerfilePath: filepath.Join("one", "two", "Dockerfile"), ContextPath: filepath.Join("one", "two"), InternalPort: 5000},
		"api":     {Name: "api", DockerfilePath: filepath.Join("api", "Dockerfile"), ContextPath: "api", InternalPort: 9000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if !linked && runtime.GOOS != "windows" {
		t.Fatal("symlink setup failed")
	}
}

func TestDetectPortFromDockerfile(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]int{
		"FROM node\nEXPOSE 4321\n":    4321,
		"FROM node\nEXPOSE\t80/tcp\n": 80,
		"FROM x\nRUN npm i next\n":    3000,
		"CMD uvicorn main:app\n":      8000,
		"RUN pip install Flask\n":     5000,
		"RUN pip install Django\n":    8000,
		"FROM golang\n":               3000,
	}
	i := 0
	for content, want := range cases {
		p := filepath.Join(dir, "Dockerfile"+itoa(i))
		i++
		writeFile(t, p, content)
		if got := detectPortFromDockerfile(p); got != want {
			t.Errorf("%q -> %d, want %d", content, got, want)
		}
	}
	if got := detectPortFromDockerfile(filepath.Join(dir, "missing")); got != 3000 {
		t.Errorf("missing -> %d", got)
	}
}

func TestValidateDockerBuildArgs(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	writeFile(t, df, string(rune(0xfeff))+"ARG FIRST\nFROM node\r\n  ARG SECOND=default\r\nRUN echo ARG NOT_DECLARED\narg lower\n\tARG TABBED\n")
	got := ValidateDockerBuildArgs(df, []string{"FIRST", "SECOND", "TABBED", "NOT_DECLARED", "lower", "MISSING"})
	if want := []string{"NOT_DECLARED", "lower", "MISSING"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	if got := ValidateDockerBuildArgs(filepath.Join(dir, "nope"), []string{"X"}); len(got) != 0 {
		t.Fatalf("missing Dockerfile must report nothing: %v", got)
	}
}

func TestDockerfileMountsSecret(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	writeFile(t, df, "FROM node:20\nRUN --mount=type=secret,id=NPM_TOKEN cat /run/secrets/NPM_TOKEN\n")
	if !DockerfileMountsSecret(df, "NPM_TOKEN") || DockerfileMountsSecret(df, "OTHER") {
		t.Fatal("basic detection")
	}
	writeFile(t, df, strings.Join([]string{
		"FROM node:20",
		"RUN --mount=type=secret,id=API_KEY_2,required=true echo",
		"RUN --mount=id=REVERSED,type=secret echo",
		"RUN --mount=type=secret,uid=1000,id=WITH_UID echo",
		"RUN --mount=TYPE = SECRET , ID = spaced echo",
		"RUN --mount=type=cache,id=CACHE_ONLY echo",
		"RUN --mount=type=secret,target=/x,uid=NOT_AN_ID echo",
		"RUN --mount=type=secret,id=a.b+c echo",
	}, "\n"))
	for id, want := range map[string]bool{
		"API_KEY_2": true, "API_KEY": false, "REVERSED": true, "WITH_UID": true, "SPACED": true,
		"CACHE_ONLY": false, "NOT_AN_ID": false, "a.b+c": true, "a.b": false,
	} {
		if got := DockerfileMountsSecret(df, id); got != want {
			t.Errorf("DockerfileMountsSecret(%q) = %v", id, got)
		}
	}
	if DockerfileMountsSecret(filepath.Join(dir, "missing"), "X") {
		t.Fatal("missing file")
	}
}
