package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"docklift/internal/execx"
)

func intp(n int) *int { return &n }

func TestResolveProjectBuild(t *testing.T) {
	t.Run("auto prefers a repository Dockerfile", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "Dockerfile"), "FROM node:24-alpine\nEXPOSE 4321\n")
		res, err := ResolveProjectBuild(root, BuildSettings{BuildType: "auto", InternalPort: intp(3000)})
		if err != nil {
			t.Fatal(err)
		}
		if res.ResolvedType != BuildDockerfile || deref(res.Services[0].DockerfilePath) != "Dockerfile" || res.Services[0].InternalPort != 4321 {
			t.Fatalf("%+v", res)
		}
		if res.Detected != "1 repository Dockerfile" || res.BaseDirectory != "." || res.Services[0].ContextPath != "." {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("auto falls back to Railpack and reports manifests", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "package.json"), `{"scripts":{"start":"node index.js"}}`)
		res, err := ResolveProjectBuild(root, BuildSettings{BuildType: "auto", InternalPort: intp(8080)})
		if err != nil {
			t.Fatal(err)
		}
		if res.ResolvedType != BuildRailpack || !reflect.DeepEqual(res.Manifests, []string{"package.json"}) || res.Services[0].InternalPort != 8080 {
			t.Fatalf("%+v", res)
		}
		if res.Detected != "Railpack source (package.json)" || res.DockerfilePath != nil || res.Services[0].DockerfilePath != nil {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("base directory scopes monorepo detection", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "package.json"), `{"private":true}`)
		writeFile(t, filepath.Join(root, "apps", "web", "package.json"), `{"scripts":{"start":"vite"}}`)
		writeFile(t, filepath.Join(root, "apps", "api", "Dockerfile"), "FROM x\n")
		res, err := ResolveProjectBuild(root, BuildSettings{BuildType: "auto", BaseDirectory: "apps/web", InternalPort: intp(4173)})
		if err != nil {
			t.Fatal(err)
		}
		if res.ResolvedType != BuildRailpack || res.BaseDirectory != "apps/web" || res.Services[0].ContextPath != "apps/web" || res.Services[0].InternalPort != 4173 {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("monorepo Dockerfiles below the base directory", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "apps", "api", "Dockerfile"), "FROM x\nEXPOSE 0\n")
		writeFile(t, filepath.Join(root, "apps", "web", "Dockerfile"), "FROM x\nEXPOSE 5173\n")
		res, err := ResolveProjectBuild(root, BuildSettings{BuildType: "weird", BaseDirectory: "apps", InternalPort: intp(70000)})
		if err != nil {
			t.Fatal(err)
		}
		if res.RequestedType != BuildAuto || len(res.Services) != 2 || res.Detected != "2 repository Dockerfiles" {
			t.Fatalf("%+v", res)
		}
		api, web := res.Services[0], res.Services[1]
		if api.Name != "api" || api.ContextPath != "apps/api" || deref(api.DockerfilePath) != "apps/api/Dockerfile" || api.InternalPort != 3000 {
			t.Fatalf("api %+v", api)
		}
		if web.InternalPort != 5173 || deref(res.DockerfilePath) != "apps/api/Dockerfile" {
			t.Fatalf("web %+v", web)
		}
	})
	t.Run("configured Dockerfile", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "svc", "docker", "Prod.Dockerfile"), "FROM x\n")
		res, err := ResolveProjectBuild(root, BuildSettings{BuildType: "dockerfile", BaseDirectory: " svc ", DockerfilePath: strp(" docker\\Prod.Dockerfile ")})
		if err != nil {
			t.Fatal(err)
		}
		want := ResolvedBuildService{Name: "app", Builder: BuildDockerfile, ContextPath: "svc", DockerfilePath: strp("svc/docker/Prod.Dockerfile"), InternalPort: 3000}
		if res.Detected != "Configured Dockerfile" || !reflect.DeepEqual(res.Services[0], want) {
			t.Fatalf("%+v", res.Services[0])
		}
		if _, err := ResolveProjectBuild(root, BuildSettings{BuildType: "dockerfile"}); err == nil || err.Error() != "Dockerfile not found: Dockerfile" {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("build paths cannot escape the project", func(t *testing.T) {
		root := t.TempDir()
		for _, s := range []BuildSettings{
			{BuildType: "dockerfile", DockerfilePath: strp("../Dockerfile")},
			{BuildType: "dockerfile", DockerfilePath: strp("a/../../Dockerfile")},
			{BuildType: "auto", BaseDirectory: "/etc"},
			{BuildType: "auto", BaseDirectory: "..\\x"},
		} {
			if _, err := ResolveProjectBuild(root, s); err == nil || !strings.Contains(err.Error(), "inside the project") {
				t.Errorf("%+v: err %v", s, err)
			}
		}
		if _, err := ResolveProjectBuild(root, BuildSettings{BaseDirectory: "missing"}); err == nil || err.Error() != "Base directory not found: missing" {
			t.Errorf("err %v", err)
		}
	})
	t.Run("symlinked base directory outside the project", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("symlinks need privileges on Windows")
			}
			t.Fatal(err)
		}
		if _, err := ResolveProjectBuild(root, BuildSettings{BaseDirectory: "link"}); err == nil || !strings.Contains(err.Error(), "symlinks must stay inside") {
			t.Fatalf("err %v", err)
		}
	})
}

func TestNormalizeBuildType(t *testing.T) {
	for in, want := range map[any]string{"dockerfile": "dockerfile", "railpack": "railpack", "auto": "auto", "Dockerfile": "auto", nil: "auto", 1.0: "auto"} {
		if got := NormalizeBuildType(in); got != want {
			t.Errorf("NormalizeBuildType(%v) = %q", in, got)
		}
	}
}

func TestSummarizeBuildFailure(t *testing.T) {
	msg := SummarizeBuildFailure("docker", "npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.", 1)
	if !strings.Contains(msg, `Run "npm install"`) || !strings.Contains(msg, "commit the updated package-lock.json") {
		t.Fatal(msg)
	}
	if msg := SummarizeBuildFailure("docker", "error: lockfile had changes, but lockfile is frozen\nERR_PNPM_OUTDATED_LOCKFILE  Cannot install with \"frozen-lockfile\" because pnpm-lock.yaml is not up to date", 1); !strings.HasPrefix(msg, "The dependency lockfile") {
		t.Fatal(msg)
	}
	if msg := SummarizeBuildFailure("railpack", "boom", 2); msg != "railpack exited with code 2" {
		t.Fatal(msg)
	}
}

// recordingRunner records commands and their environment additions.
type recordingRunner struct {
	cmds []execx.Cmd
	fail map[string]int
	out  string
}

func (r *recordingRunner) run(_ context.Context, c execx.Cmd) (execx.Result, error) {
	r.cmds = append(r.cmds, c)
	if r.out != "" {
		_, _ = c.Stdout.Write([]byte(r.out))
	}
	if code, ok := r.fail[c.Name]; ok {
		return execx.Result{ExitCode: code}, &execx.ExitError{Name: c.Name, Code: code}
	}
	return execx.Result{}, nil
}

func TestBuildRejectsReservedAndInvalidVariables(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Dockerfile"), "FROM scratch\n")
	svc := ResolvedBuildService{Name: "app", Builder: BuildDockerfile, ContextPath: ".", DockerfilePath: strp("Dockerfile"), InternalPort: 3000}
	for _, c := range []struct {
		env  EnvVar
		want string
	}{
		{EnvVar{Key: "DOCKER_HOST", Value: "tcp://attacker", IsBuildArg: true}, "reserved by DockLift"},
		{EnvVar{Key: "buildkit_x", Value: "x", IsBuildArg: true}, "reserved by DockLift"},
		{EnvVar{Key: "PATH", Value: "/x", IsBuildArg: true}, "reserved by DockLift"},
		{EnvVar{Key: "BAD-NAME", Value: "x"}, "Invalid environment variable name: BAD-NAME"},
	} {
		rr := &recordingRunner{}
		err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, StatePath: filepath.Join(root, ".state"), Service: svc,
			ImageTag: "unused:test", EnvVars: []EnvVar{c.env}, Run: rr.run})
		if err == nil || !strings.Contains(err.Error(), c.want) || len(rr.cmds) != 0 {
			t.Errorf("%+v: err %v cmds %d", c.env, err, len(rr.cmds))
		}
	}
	// A reserved name is fine as a runtime-only variable.
	rr := &recordingRunner{}
	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, Service: svc, ImageTag: "t:1",
		EnvVars: []EnvVar{{Key: "PORT", Value: "1", IsRuntime: true}}, Run: rr.run}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDockerfileArgsAndSecrets(t *testing.T) {
	root := t.TempDir()
	svc := ResolvedBuildService{Name: "web", Builder: BuildDockerfile, ContextPath: "apps/web", DockerfilePath: strp("apps/web/Dockerfile"), InternalPort: 3000}
	abs, _ := filepath.Abs(root)
	ctxDir := filepath.Join(abs, "apps", "web")
	df := filepath.Join(abs, "apps", "web", "Dockerfile")

	rr := &recordingRunner{}
	var log bytes.Buffer
	env := []EnvVar{{Key: "PUBLIC_URL", Value: "https://x", IsBuildArg: true}, {Key: "RUNTIME", Value: "r", IsRuntime: true}}
	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, Service: svc, ImageTag: "docklift-abcdef12-web:deadbeef", EnvVars: env, Log: &log, Run: rr.run}); err != nil {
		t.Fatal(err)
	}
	want := []string{"build", "--progress", "plain", "-t", "docklift-abcdef12-web:deadbeef", "-f", df, "--build-arg", "PUBLIC_URL=https://x", ctxDir}
	if c := rr.cmds[0]; c.Name != "docker" || !reflect.DeepEqual(c.Args, want) || c.Dir != ctxDir || len(c.ExtraEnv) != 0 {
		t.Fatalf("cmd %+v", c)
	}
	if log.String() != "🐳 Building web with apps/web/Dockerfile\n" {
		t.Fatalf("log %q", log.String())
	}

	rr = &recordingRunner{}
	log.Reset()
	env = append(env, EnvVar{Key: "NPM_TOKEN", Value: "s3cr3t-value", IsBuildArg: true, IsSecret: true})
	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, Service: svc, ImageTag: "t:1", EnvVars: env, Log: &log, Run: rr.run}); err != nil {
		t.Fatal(err)
	}
	want = []string{"buildx", "build", "--load", "--progress", "plain", "-t", "t:1", "-f", df, "--build-arg", "PUBLIC_URL=https://x", "--secret", "id=NPM_TOKEN,env=NPM_TOKEN", ctxDir}
	c := rr.cmds[0]
	if !reflect.DeepEqual(c.Args, want) || !reflect.DeepEqual(c.ExtraEnv, []string{"NPM_TOKEN=s3cr3t-value"}) {
		t.Fatalf("cmd %+v", c)
	}
	if strings.Contains(strings.Join(c.Args, " "), "s3cr3t") || strings.Contains(log.String(), "s3cr3t") {
		t.Fatal("secret value leaked into argv or log")
	}
	if !strings.HasPrefix(log.String(), "🔐 Passing NPM_TOKEN as BuildKit secret (not build-arg)\n") {
		t.Fatalf("log %q", log.String())
	}

	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, Service: ResolvedBuildService{Name: "x", Builder: BuildDockerfile}, Run: rr.run}); err == nil || err.Error() != "Resolved Dockerfile path is missing" {
		t.Fatalf("err %v", err)
	}
}

func TestBuildRailpack(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	abs, _ := filepath.Abs(root)
	svc := ResolvedBuildService{Name: "my app", Builder: BuildRailpack, ContextPath: ".", InternalPort: 8080}
	env := []EnvVar{{Key: "B_TOKEN", Value: "bbb", IsBuildArg: true, IsSecret: true}, {Key: "A_URL", Value: "aaa", IsBuildArg: true}, {Key: "RUN_ONLY", Value: "zzz", IsRuntime: true}}
	rr := &recordingRunner{}
	var log bytes.Buffer
	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, StatePath: state, Service: svc, ImageTag: "docklift-abcdef12-my-app:deadbeef", EnvVars: env, Log: &log, Run: rr.run}); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(state, "my-app-railpack-plan.json")
	info := filepath.Join(state, "my-app-railpack-info.json")
	prep := rr.cmds[0]
	wantPrep := []string{"prepare", abs, "--plan-out", plan, "--info-out", info, "--env", "PORT=8080", "--env", "B_TOKEN=__DOCKLIFT_BUILD_SECRET__", "--env", "A_URL=__DOCKLIFT_BUILD_SECRET__"}
	if prep.Name != "railpack" || !reflect.DeepEqual(prep.Args, wantPrep) || len(prep.ExtraEnv) != 0 {
		t.Fatalf("prepare %+v", prep)
	}
	sum := sha256.Sum256([]byte("A_URL=aaa\nB_TOKEN=bbb"))
	build := rr.cmds[1]
	wantBuild := []string{"buildx", "build", "--load", "--progress", "plain",
		"--build-arg", "BUILDKIT_SYNTAX=ghcr.io/railwayapp/railpack-frontend:v0.33.0", "--build-arg", "PORT=8080",
		"--secret", "id=PORT,env=PORT", "-f", plan, "-t", "docklift-abcdef12-my-app:deadbeef",
		"--build-arg", "secrets-hash=" + hex.EncodeToString(sum[:]),
		"--secret", "id=B_TOKEN,env=B_TOKEN", "--secret", "id=A_URL,env=A_URL", abs}
	if build.Name != "docker" || !reflect.DeepEqual(build.Args, wantBuild) {
		t.Fatalf("build %+v", build.Args)
	}
	if !reflect.DeepEqual(build.ExtraEnv, []string{"PORT=8080", "B_TOKEN=bbb", "A_URL=aaa"}) {
		t.Fatalf("env %v", build.ExtraEnv)
	}
	for _, c := range rr.cmds {
		if j := strings.Join(c.Args, " "); strings.Contains(j, "bbb") || strings.Contains(j, "aaa") || strings.Contains(j, "zzz") {
			t.Fatalf("variable value in argv: %s", j)
		}
	}
	if st, err := os.Stat(state); err != nil || !st.IsDir() {
		t.Fatal("state dir not created")
	}
	wantLog := "🛤️  No Dockerfile found → analyzing . with Railpack\n📦 Railpack plan ready; building docklift-abcdef12-my-app:deadbeef\n"
	if log.String() != wantLog {
		t.Fatalf("log %q", log.String())
	}

	// No build vars: no secrets-hash.
	rr = &recordingRunner{}
	_ = BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, StatePath: state, Service: svc, ImageTag: "t:1", Run: rr.run})
	if slices.ContainsFunc(rr.cmds[1].Args, func(a string) bool { return strings.HasPrefix(a, "secrets-hash=") }) {
		t.Fatal("unexpected secrets-hash")
	}
}

func TestBuildFailureIsSummarized(t *testing.T) {
	root := t.TempDir()
	svc := ResolvedBuildService{Name: "app", Builder: BuildRailpack, ContextPath: ".", InternalPort: 3000}
	rr := &recordingRunner{fail: map[string]int{"railpack": 1}, out: "npm ci can only install packages when in sync"}
	var log bytes.Buffer
	err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, StatePath: t.TempDir(), Service: svc, ImageTag: "t:1", Log: &log, Run: rr.run})
	if err == nil || !strings.Contains(err.Error(), `Run "npm install"`) || len(rr.cmds) != 1 {
		t.Fatalf("err %v cmds %d", err, len(rr.cmds))
	}
	if !strings.Contains(log.String(), "npm ci can only") {
		t.Fatal("build output must stream to the log")
	}
	startErr := func(context.Context, execx.Cmd) (execx.Result, error) {
		return execx.Result{ExitCode: -1}, &execx.StartError{Name: "railpack", Err: errors.New("not found")}
	}
	if err := BuildServiceImage(context.Background(), BuildRequest{ProjectPath: root, StatePath: t.TempDir(), Service: svc, Run: startErr}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err %v", err)
	}
}

func TestSecretsHashSortsLikeJavaScript(t *testing.T) {
	// "Z" (0x5A) sorts before "a" (0x61); astral characters compare by
	// UTF-16 surrogates (0xD83D...) and so sort before U+FFFD.
	vars := []EnvVar{{Key: "a", Value: "1"}, {Key: "Z", Value: "2"}, {Key: "K", Value: "�"}, {Key: "K", Value: "🚀"}}
	sum := sha256.Sum256([]byte("K=🚀\nK=�\nZ=2\na=1"))
	if got := SecretsHash(vars); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %s", got)
	}
}
