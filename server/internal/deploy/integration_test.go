package deploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"docklift/internal/db"
	"docklift/internal/dockerx"
	"docklift/internal/execx"
	"docklift/internal/store"
)

// Integration tests against a real Docker daemon. They run only with
// DOCKLIFT_DOCKER=1 (Linux test server with Docker, compose and buildx) and
// clean up every object they create.

const itBaseImage = "alpine:3.20"

type dockerEnv struct {
	t       *testing.T
	ctx     context.Context
	docker  *dockerx.Docker
	runtime *Runtime
	suffix  string
}

func requireDocker(t *testing.T) *dockerEnv {
	t.Helper()
	if os.Getenv("DOCKLIFT_DOCKER") != "1" {
		t.Skip("set DOCKLIFT_DOCKER=1 to run Docker integration tests")
	}
	engine, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := dockerx.New(engine, "docklift_it_"+suffix, "docklift-it-proxy-"+suffix, log)
	if err := d.Ping(ctx); err != nil {
		t.Fatalf("docker not reachable: %v", err)
	}
	if err := d.PullImage(ctx, itBaseImage, dockerx.PullOptions{}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	database, err := db.Open(ctx, filepath.Join(dir, "docklift.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Store: store.New(database), Docker: d, Tracker: NewTracker(), Hub: NewHubTags(nil, log),
		DeploymentsPath: filepath.Join(dir, "deployments"), PortRangeStart: 5500, PortRangeEnd: 5600, Log: log}
	return &dockerEnv{t: t, ctx: ctx, docker: d, runtime: r, suffix: suffix}
}

func (e *dockerEnv) cli(args ...string) string {
	e.t.Helper()
	out, stderr, _, err := execx.Output(e.ctx, execx.Cmd{Name: "docker", Args: args, Timeout: 5 * time.Minute})
	if err != nil {
		e.t.Fatalf("docker %v: %v\n%s", args, err, stderr)
	}
	return out
}

func (e *dockerEnv) cleanup(args ...string) {
	e.t.Cleanup(func() {
		_, _, _, _ = execx.Output(context.Background(), execx.Cmd{Name: "docker", Args: args, Timeout: time.Minute})
	})
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestDockerIntegrationEnsureNetwork(t *testing.T) {
	e := requireDocker(t)
	e.cleanup("network", "rm", e.docker.ControlNetwork)
	if err := e.docker.EnsureNetwork(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.docker.EnsureNetwork(e.ctx); err != nil {
		t.Fatalf("second ensure must be a no-op: %v", err)
	}
	labels := e.cli("network", "inspect", "-f", "{{index .Labels \"com.docklift.role\"}} {{.Driver}}", e.docker.ControlNetwork)
	if strings.TrimSpace(labels) != "control-plane bridge" {
		t.Fatalf("network %q", labels)
	}
}

// TestDockerIntegrationDeployLifecycle builds an image from a Dockerfile,
// runs it through the runtime compose file, checks status/logs/stats/log
// streaming and proxy attachment, then tears down with label verification.
func TestDockerIntegrationDeployLifecycle(t *testing.T) {
	e := requireDocker(t)
	r := e.runtime
	ctx := e.ctx
	p := mustProject(t, r, store.ProjectCreate{Name: "IT Smoke " + e.suffix})
	dep := mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusInProgress)})
	projectPath := r.ProjectPath(p.ID)
	writeFile(t, filepath.Join(projectPath, "Dockerfile"), "FROM "+itBaseImage+"\nARG GREETING\nRUN --mount=type=secret,id=IT_SECRET test -s /run/secrets/IT_SECRET\nCMD [\"sh\", \"-c\", \"echo hello-docklift; sleep 3600\"]\n")

	build, err := ResolveProjectBuild(projectPath, BuildSettings{BuildType: BuildAuto})
	if err != nil || build.ResolvedType != BuildDockerfile {
		t.Fatalf("resolve %+v %v", build, err)
	}
	svc := build.Services[0]
	tag := DockliftImageTag(p.ID, svc.Name, dep.ID)
	e.cleanup("rmi", "-f", tag)
	var buildLog bytes.Buffer
	env := []EnvVar{{Key: "GREETING", Value: "hi", IsBuildArg: true}, {Key: "IT_SECRET", Value: "it-secret-value", IsBuildArg: true, IsSecret: true}}
	if err := BuildServiceImage(ctx, BuildRequest{ProjectPath: projectPath, StatePath: r.StatePath(p.ID), Service: svc, ImageTag: tag, EnvVars: env, Log: &buildLog}); err != nil {
		t.Fatalf("build: %v\n%s", err, buildLog.String())
	}
	if strings.Contains(buildLog.String(), "it-secret-value") {
		t.Fatal("secret value in build log")
	}
	if !e.docker.ImageExists(ctx, tag) {
		t.Fatal("image not built")
	}

	containerName := ServiceContainerName(p.Name, p.ID, svc.Name)
	composeProject := ComposeProjectName(p.Name, p.ID)
	if err := WriteRuntimeCompose(r.ComposePath(p.ID), []RuntimeService{{Name: svc.Name, Image: tag, InternalPort: svc.InternalPort, ContainerName: containerName}},
		[]EnvVar{{Key: "AWKWARD", Value: "yes: ${NOT_SET} $$ 0123", IsRuntime: true}}, RuntimeComposeOptions{ProjectID: p.ID}); err != nil {
		t.Fatal(err)
	}
	files, err := r.ComposeFiles(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.docker.Compose(context.Background(), dockerx.ComposeCmd{Project: composeProject, Files: files, Dir: projectPath, Args: []string{"down", "--remove-orphans"}})
	})
	var upLog bytes.Buffer
	res, err := e.docker.Compose(ctx, dockerx.ComposeCmd{Project: composeProject, Files: files, Dir: projectPath, Args: []string{"up", "-d", "--remove-orphans"}, Output: &upLog})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("compose up: %v code %d\n%s", err, res.ExitCode, upLog.String())
	}
	// Compose interpolates $ in values exactly as it did for the TS-written
	// file: ${NOT_SET} becomes empty and $$ a single $.
	if env := e.cli("exec", containerName, "printenv", "AWKWARD"); strings.TrimRight(env, "\r\n") != "yes:  $ 0123" {
		t.Fatalf("env round trip through compose: %q", env)
	}

	if st := e.docker.ContainerStatus(ctx, containerName); !st.Running || st.Status != "running" {
		t.Fatalf("status %+v", st)
	}
	eventually(t, "container logs", func() bool {
		return strings.Contains(e.docker.ContainerLogs(ctx, containerName, 100), "hello-docklift")
	})
	stats := e.docker.ContainerStats(ctx, containerName)
	if stats == nil || !strings.HasSuffix(stats.MemoryUsage, " MB") || !strings.HasSuffix(stats.MemoryLimit, " MB") {
		t.Fatalf("stats %+v", stats)
	}
	if ref := e.docker.ContainerImageRef(ctx, containerName); ref != tag {
		t.Fatalf("image ref %q", ref)
	}

	streamCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	var events []dockerx.LogEvent
	_ = e.docker.StreamContainerLogs(streamCtx, containerName, 10, func(ev dockerx.LogEvent) error {
		events = append(events, ev)
		if ev.Type == dockerx.LogEventLog && strings.Contains(ev.Message, "hello-docklift") {
			stop()
		}
		return nil
	})
	stop()
	if len(events) < 2 || events[0].Type != dockerx.LogEventConnected || events[len(events)-1].Type != dockerx.LogEventLog {
		t.Fatalf("stream events %+v", events)
	}

	// Edge proxy attachment (a stand-in container plays the proxy).
	e.cli("run", "-d", "--name", e.docker.ProxyContainer, itBaseImage, "sleep", "3600")
	e.cleanup("rm", "-f", e.docker.ProxyContainer)
	if err := r.ConnectProxyToProjectNetwork(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.ConnectProxyToProjectNetwork(ctx, p.ID); err != nil {
		t.Fatalf("already connected must succeed: %v", err)
	}
	nets := e.cli("inspect", "-f", "{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}", e.docker.ProxyContainer)
	if !strings.Contains(nets, ProjectNetworkName(p.ID)) {
		t.Fatalf("proxy networks %q", nets)
	}
	if err := e.docker.ConnectProxy(ctx, "dl-net-doesnotexist"); err == nil {
		t.Fatal("missing network must fail")
	}

	// Teardown with the proxy still attached fails to remove the network;
	// the label probe must see that, and succeed after detaching.
	down := e.docker.ComposeDown(ctx, dockerx.ComposeCmd{Project: composeProject, Files: files, Dir: projectPath, Args: []string{"down"}})
	if down.OK {
		t.Log("compose down removed the network despite the attached proxy (Docker version dependent)")
	}
	r.DisconnectProxyFromProjectNetwork(ctx, p.ID)
	down = e.docker.ComposeDown(ctx, dockerx.ComposeCmd{Project: composeProject, Files: files, Dir: projectPath, Args: []string{"down"}})
	if !down.OK {
		t.Fatalf("teardown not verified: %s", down.Detail())
	}
	if left := e.docker.ProbeComposeProject(ctx, composeProject); !left.Gone() {
		t.Fatalf("leftovers %+v", left)
	}
	if st := e.docker.ContainerStatus(ctx, containerName); st.Status != dockerx.StatusNotFound {
		t.Fatalf("container still present: %+v", st)
	}
}

// TestDockerIntegrationImageRetention keeps the current and previous tagged
// deployments' images and removes older Docklift tags, never foreign ones.
func TestDockerIntegrationImageRetention(t *testing.T) {
	e := requireDocker(t)
	r := e.runtime
	ctx := e.ctx
	p := mustProject(t, r, store.ProjectCreate{Name: "retention"})
	repo := DockliftImageRepo(p.ID, "app")
	foreign := "docklift-it-foreign-" + e.suffix + ":keep"
	all := []string{repo + ":old00001", repo + ":old00002", repo + ":previous", repo + ":current0", foreign}
	for _, tag := range all {
		e.cli("tag", itBaseImage, tag)
		e.cleanup("rmi", "-f", tag)
	}
	for _, tg := range []string{repo + ":old00001", repo + ":old00002", repo + ":previous"} {
		mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusSuccess), ImageTags: tags("app", tg).JSON()})
		time.Sleep(5 * time.Millisecond)
	}
	mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusSuccess), Trigger: strp("stop")})
	current := tags("app", repo+":current0")
	mustDeployment(t, r, store.DeploymentCreate{ProjectID: p.ID, Status: strp(StatusSuccess), ImageTags: current.JSON()})

	var log bytes.Buffer
	res := r.PostDeployCleanup(ctx, p.ID, current, &log)
	if !res.Success {
		t.Fatalf("%+v\n%s", res, log.String())
	}
	for tag, want := range map[string]bool{repo + ":current0": true, repo + ":previous": true, repo + ":old00001": false, repo + ":old00002": false, foreign: true} {
		if got := e.docker.ImageExists(ctx, tag); got != want {
			t.Errorf("%s exists=%v, want %v\n%s", tag, got, want, log.String())
		}
	}
}
