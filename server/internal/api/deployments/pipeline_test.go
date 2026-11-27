package deployments

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"docklift/internal/deploy"
	"docklift/internal/store"
)

// syncBuffer is a goroutine-safe client sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("%s: missing %q in:\n%s", what, w, got)
		}
	}
}

func (e *env) start(t *testing.T, p *store.Project) (*Run, *syncBuffer) {
	t.Helper()
	out := &syncBuffer{}
	run, err := e.s.StartDeploy(context.Background(), p.ID, DeployOptions{Output: out, Host: "example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return run, out
}

func (e *env) assertUnlocked(t *testing.T, pid string) {
	t.Helper()
	if e.s.rt.Tracker.IsDeploying(pid) {
		t.Fatal("project lock still held")
	}
	if _, ok := e.s.rt.Tracker.ActiveDeployment(pid); ok {
		t.Fatal("process still registered")
	}
}

func TestDeploySuccess(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)

	d := e.deployment(t, run.DeploymentID)
	if strVal(d.Status) != deploy.StatusSuccess || run.Status() != deploy.StatusSuccess {
		t.Fatalf("status = %s / %s, logs:\n%s", strVal(d.Status), run.Status(), strVal(d.Logs))
	}
	tags, ok := deploy.ParseImageTags(d.ImageTags)
	want := deploy.DockliftImageTag(p.ID, "app", run.DeploymentID)
	if got, _ := tags.Get("app"); !ok || got != want {
		t.Fatalf("image_tags = %s, want app=%s", string(*d.ImageTags), want)
	}
	if d.FinishedAt == nil {
		t.Fatal("finished_at not set")
	}
	mustContain(t, "row logs", strVal(d.Logs), "🚀 Starting deployment...\n", "📦 BUILD: 1 repository Dockerfile", "✅ DEPLOY SUCCESSFUL!", "🔗 Edge proxy attached")
	mustContain(t, "client", out.String(), "🚀 Starting deployment...\n", "📊 Deployment complete! Status: SUCCESS ✅", "Container started")
	proj, _ := e.d.Store.GetProject(context.Background(), p.ID)
	if strVal(proj.Status) != deploy.StatusRunning {
		t.Fatalf("project status = %s", strVal(proj.Status))
	}
	if e.docker.called("compose up") != 1 || e.docker.called("connect dl-net-") != 1 {
		t.Fatalf("calls = %v", e.docker.calls)
	}
	e.assertUnlocked(t, p.ID)
}

func TestDeployBuildFailure(t *testing.T) {
	e := setup(t)
	e.docker.buildFail = true
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	d := e.deployment(t, run.DeploymentID)
	if strVal(d.Status) != deploy.StatusFailed {
		t.Fatalf("status = %s", strVal(d.Status))
	}
	mustContain(t, "row logs", strVal(d.Logs), "\n❌ Error: docker exited with code 1\n")
	mustContain(t, "client", out.String(), "\n❌ Error: docker exited with code 1\n")
	if e.docker.called("compose up") != 0 {
		t.Fatal("compose up must not run after a failed build")
	}
	proj, _ := e.d.Store.GetProject(context.Background(), p.ID)
	if strVal(proj.Status) != deploy.StatusError {
		t.Fatalf("project status = %s", strVal(proj.Status))
	}
	e.assertUnlocked(t, p.ID)
}

func TestDeployComposeFailure(t *testing.T) {
	e := setup(t)
	e.docker.upCode = 2
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusFailed {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "❌ DEPLOY FAILED (Exit Code: 2)", "📊 Deployment complete! Status: FAILED ❌")
	e.assertUnlocked(t, p.ID)
}

func TestDeployComposeDidNotRun(t *testing.T) {
	e := setup(t)
	e.docker.upErr = errors.New("start docker: not found")
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusFailed {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "❌ Docker execution error: start docker: not found")
	e.assertUnlocked(t, p.ID)
}

func TestDeployProxyAttachFailure(t *testing.T) {
	e := setup(t)
	e.docker.connErr = errors.New("no proxy")
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusFailed {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "❌ Edge proxy attach FAILED: no proxy", "❌ DEPLOY FAILED (edge proxy not attached)")
}

func TestDeployRefusals(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.s.StartDeploy(ctx, "missing", DeployOptions{}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	p, err := e.d.Store.CreateProject(ctx, store.ProjectCreate{Name: "nofiles"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.StartDeploy(ctx, p.ID, DeployOptions{}); !errors.Is(err, ErrFilesNotFound) {
		t.Fatalf("no files: %v", err)
	}
	q := e.project(t, "busy", store.ProjectCreate{}, dockerfileApp)
	if _, err := e.d.Store.CreateDeployment(ctx, store.DeploymentCreate{ProjectID: q.ID, Status: store.Ptr(deploy.StatusInProgress)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.StartDeploy(ctx, q.ID, DeployOptions{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("in_progress row: %v", err)
	}
	c := e.project(t, "cancelling", store.ProjectCreate{}, dockerfileApp)
	_, end := e.s.rt.Tracker.BeginCancel(c.ID)
	if _, err := e.s.StartDeploy(ctx, c.ID, DeployOptions{}); !errors.Is(err, ErrCancelling) {
		t.Fatalf("cancelling: %v", err)
	}
	end()
}

func TestConcurrentStartsAdmitOne(t *testing.T) {
	e := setup(t)
	e.docker.buildGate = newGate()
	p := e.project(t, "race", store.ProjectCreate{}, dockerfileApp)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var runs []*Run
	busy := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run, err := e.s.StartDeploy(context.Background(), p.ID, DeployOptions{})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				runs = append(runs, run)
			case errors.Is(err, ErrBusy):
				busy++
			default:
				t.Errorf("unexpected error %v", err)
			}
		}()
	}
	wg.Wait()
	if len(runs) != 1 || busy != 11 {
		t.Fatalf("admitted %d, busy %d", len(runs), busy)
	}
	rows, _ := e.d.Store.ListDeployments(context.Background(), store.DeploymentFilter{ProjectID: &p.ID}, store.OrderNone, store.Page{})
	if len(rows) != 1 {
		t.Fatalf("rows = %d (refused starts must not create rows)", len(rows))
	}
	waitGate(t, e.docker.buildGate)
	close(e.docker.buildGate.release)
	waitRun(t, runs[0])
	if runs[0].Status() != deploy.StatusSuccess {
		t.Fatalf("status = %s", runs[0].Status())
	}
}

func (e *env) cancelNow(t *testing.T, pid string) string {
	t.Helper()
	out := &syncBuffer{}
	run, err := e.s.Cancel(context.Background(), pid, out)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, run)
	return out.String()
}

func TestCancelDuringBuild(t *testing.T) {
	e := setup(t)
	e.docker.buildGate = newGate()
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitGate(t, e.docker.buildGate)
	cancelOut := e.cancelNow(t, p.ID)
	waitRun(t, run)
	mustContain(t, "cancel", cancelOut, "🛑 Stopping in-flight docker compose process...\n", "✅ Cancelled — containers down. Ready for a fresh deploy.\n")
	d := e.deployment(t, run.DeploymentID)
	if strVal(d.Status) != deploy.StatusCancelled {
		t.Fatalf("status = %s", strVal(d.Status))
	}
	mustContain(t, "row logs", strVal(d.Logs), "\n❌ Deployment cancelled\n")
	// The killed build returns context.Canceled; the client is told the
	// deployment was cancelled, not the raw Go error (seen on a real daemon).
	mustContain(t, "client", out.String(), "\n❌ Error: Deployment cancelled\n")
	if strings.Contains(out.String(), "context canceled") {
		t.Fatalf("client sees the raw context error:\n%s", out.String())
	}
	if e.docker.called("compose up") != 0 {
		t.Fatal("compose up ran after cancel")
	}
	e.assertUnlocked(t, p.ID)
	proj, _ := e.d.Store.GetProject(context.Background(), p.ID)
	if strVal(proj.Status) != deploy.StatusStopped {
		t.Fatalf("project status = %s", strVal(proj.Status))
	}
	// A fresh deploy works afterwards.
	e.docker.buildGate = nil
	again, _ := e.start(t, p)
	waitRun(t, again)
	if again.Status() != deploy.StatusSuccess {
		t.Fatalf("redeploy after cancel = %s", again.Status())
	}
}

func TestCancelDuringComposeUp(t *testing.T) {
	e := setup(t)
	e.docker.upGate = newGate()
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitGate(t, e.docker.upGate)
	e.cancelNow(t, p.ID)
	waitRun(t, run)
	if run.Status() != deploy.StatusCancelled {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "Deploy lost project lock")
	if strings.Contains(out.String(), "DEPLOY SUCCESSFUL") {
		t.Fatal("cancelled deploy reported success")
	}
	e.assertUnlocked(t, p.ID)
}

func TestCancelDuringManagedPull(t *testing.T) {
	e := setup(t)
	e.docker.pullGate = newGate()
	p := e.project(t, "pg", store.ProjectCreate{ProjectType: store.Ptr("database"), DBEngine: store.Ptr(deploy.EnginePostgres)}, nil)
	for k, v := range map[string]string{"POSTGRES_USER": "u", "POSTGRES_PASSWORD": "pgsecretpw", "POSTGRES_DB": "d"} {
		if _, err := e.d.Store.CreateEnvVariable(context.Background(), store.EnvCreate{ProjectID: p.ID, Key: k, Value: v}); err != nil {
			t.Fatal(err)
		}
	}
	run, _ := e.start(t, p)
	waitGate(t, e.docker.pullGate)
	e.cancelNow(t, p.ID)
	waitRun(t, run)
	if run.Status() != deploy.StatusCancelled {
		t.Fatalf("status = %s", run.Status())
	}
	e.assertUnlocked(t, p.ID)
}

func TestManagedDatabaseDeploy(t *testing.T) {
	e := setup(t)
	p := e.project(t, "pg", store.ProjectCreate{ProjectType: store.Ptr("database"), DBEngine: store.Ptr(deploy.EnginePostgres)}, nil)
	for k, v := range map[string]string{"POSTGRES_USER": "u", "POSTGRES_PASSWORD": "pgsecretpw", "POSTGRES_DB": "d"} {
		if _, err := e.d.Store.CreateEnvVariable(context.Background(), store.EnvCreate{ProjectID: p.ID, Key: k, Value: v}); err != nil {
			t.Fatal(err)
		}
	}
	e.docker.connErr = errors.New("proxy down")
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusSuccess {
		t.Fatalf("status = %s\n%s", run.Status(), out.String())
	}
	mustContain(t, "client", out.String(), "🗄️  MANAGED DATABASE: PostgreSQL", "✅ Image ready", "Edge proxy attach skipped/failed (OK for internal databases)")
	d := e.deployment(t, run.DeploymentID)
	if d.ImageTags != nil {
		t.Fatalf("managed DB rows carry no docklift tags: %s", string(*d.ImageTags))
	}
	svc, err := e.d.Store.FindService(context.Background(), store.ServiceFilter{ProjectID: &p.ID}, store.OrderNone)
	if err != nil || !strings.HasPrefix(svc.DockerfilePath, "[managed:postgres|") {
		t.Fatalf("service = %+v, %v", svc, err)
	}
}

func TestDeployPanicReachesTerminalStatus(t *testing.T) {
	e := setup(t)
	e.docker.panicOnUp = true
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusFailed {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "❌ Error: internal error")
	e.assertUnlocked(t, p.ID)
}

func TestDeployRedactsSecrets(t *testing.T) {
	e := setup(t)
	e.docker.buildOut = "step 1: using supersecretvalue now\n"
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	if _, err := e.d.Store.CreateEnvVariable(context.Background(), store.EnvCreate{ProjectID: p.ID, Key: "API_KEY", Value: "supersecretvalue", IsSecret: store.Ptr(true)}); err != nil {
		t.Fatal(err)
	}
	run, out := e.start(t, p)
	waitRun(t, run)
	d := e.deployment(t, run.DeploymentID)
	for what, s := range map[string]string{"client": out.String(), "row": strVal(d.Logs)} {
		if strings.Contains(s, "supersecretvalue") {
			t.Fatalf("%s log leaks the secret", what)
		}
		mustContain(t, what, s, "using *** now")
	}
}

func TestGitHubPullUsesTokenAndScrubs(t *testing.T) {
	e := setup(t)
	e.s.SetGitHubApp(fakeGitHub{token: "ghs_tokenvalue123"})
	e.git.origin = "https://x-access-token:old@github.com/acme/app.git"
	p := e.project(t, "gh", store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("https://github.com/acme/app.git")}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusSuccess {
		t.Fatalf("status = %s\n%s", run.Status(), out.String())
	}
	if e.git.pullCreds == nil || e.git.pullCreds.Password != "ghs_tokenvalue123" {
		t.Fatalf("pull credentials = %+v", e.git.pullCreds)
	}
	if e.git.origin != "https://github.com/acme/app.git" {
		t.Fatalf("origin not scrubbed: %s", e.git.origin)
	}
	d := e.deployment(t, run.DeploymentID)
	if strings.Contains(out.String(), "ghs_tokenvalue123") || strings.Contains(strVal(d.Logs), "ghs_tokenvalue123") {
		t.Fatal("token leaked into the deploy log")
	}
	mustContain(t, "client", out.String(), "🔑 Refreshed GitHub access token")
	if strVal(d.CommitMessage) != "head msg" {
		t.Fatalf("commit message = %q", strVal(d.CommitMessage))
	}
}

// A legacy http:// row still deploys: the URL is normalised to https, the
// origin is reset to the stored repository before the pull, and the token
// is bound to that origin (a checkout whose origin was pointed elsewhere
// never receives it).
func TestGitHubLegacyHTTPURLNormalisedAndOriginPinned(t *testing.T) {
	e := setup(t)
	e.s.SetGitHubApp(fakeGitHub{token: "ghs_tokenvalue123"})
	e.git.origin = "https://evil.test/acme/app.git"
	p := e.project(t, "gh", store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("http://github.com/acme/app.git")}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusSuccess {
		t.Fatalf("status = %s\n%s", run.Status(), out.String())
	}
	const want = "https://github.com/acme/app.git"
	if e.git.pullOrigin != want || e.git.pullExpected != want {
		t.Fatalf("pull origin = %q, expected origin = %q", e.git.pullOrigin, e.git.pullExpected)
	}
	if e.git.pullCreds == nil {
		t.Fatal("legacy http row got no token")
	}
}

// A github_url that is not https://github.com/<owner>/<repo> (e.g. a path
// that merely contains "github.com/") never gets a token.
func TestGitHubTokenOnlyForGitHubHost(t *testing.T) {
	e := setup(t)
	e.s.SetGitHubApp(fakeGitHub{token: "ghs_tokenvalue123"})
	p := e.project(t, "gh", store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("https://evil.test/github.com/acme/app.git")}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if e.git.pullCreds != nil {
		t.Fatalf("token sent for a non-GitHub URL\n%s", out.String())
	}
}

func TestGitHubScrubFailureFailsDeploy(t *testing.T) {
	e := setup(t)
	e.s.SetGitHubApp(fakeGitHub{token: "ghs_tokenvalue123"})
	e.git.scrubErr = errors.New("locked config")
	p := e.project(t, "gh", store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("https://github.com/acme/app.git")}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	if run.Status() != deploy.StatusFailed {
		t.Fatalf("status = %s", run.Status())
	}
	mustContain(t, "client", out.String(), "❌ Failed to scrub Git credentials: locked config", "❌ Error: Failed to scrub Git credentials after pull")
}

func TestGitHubWithoutAppWarns(t *testing.T) {
	e := setup(t)
	p := e.project(t, "gh", store.ProjectCreate{SourceType: store.Ptr("github"), GithubURL: store.Ptr("https://github.com/acme/app.git")}, dockerfileApp)
	run, out := e.start(t, p)
	waitRun(t, run)
	mustContain(t, "client", out.String(), "⚠️ Token refresh warning: GitHub App is not configured")
	if e.git.pullCreds != nil {
		t.Fatal("pull must run unauthenticated")
	}
}

func TestKeepTwoImageRetention(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	var tags []string
	for range 3 {
		run, _ := e.start(t, p)
		waitRun(t, run)
		if run.Status() != deploy.StatusSuccess {
			t.Fatalf("status = %s", run.Status())
		}
		tags = append(tags, deploy.DockliftImageTag(p.ID, "app", run.DeploymentID))
	}
	if e.docker.ImageExists(context.Background(), tags[0]) {
		t.Fatal("oldest image must be removed")
	}
	for _, tag := range tags[1:] {
		if !e.docker.ImageExists(context.Background(), tag) {
			t.Fatalf("image %s must be kept", tag)
		}
	}
}

func TestStopAndRestart(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	ctx := context.Background()
	if _, err := e.s.Stop(ctx, p.ID, nil); !errors.Is(err, ErrNotDeployed) {
		t.Fatalf("stop before deploy: %v", err)
	}
	if _, err := e.s.Restart(ctx, p.ID, nil); !errors.Is(err, ErrNotDeployed) {
		t.Fatalf("restart before deploy: %v", err)
	}
	run, _ := e.start(t, p)
	waitRun(t, run)

	out := &syncBuffer{}
	stop, err := e.s.Stop(ctx, p.ID, out)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, stop)
	mustContain(t, "stop", out.String(), "🛑 STOPPING PROJECT", "✅ Teardown verified for", "✅ STOP SUCCESSFUL!")
	if stop.Status() != deploy.StatusSuccess {
		t.Fatalf("stop row = %s", stop.Status())
	}
	proj, _ := e.d.Store.GetProject(ctx, p.ID)
	if strVal(proj.Status) != deploy.StatusStopped {
		t.Fatalf("project = %s", strVal(proj.Status))
	}

	e.docker.downOK = false
	out = &syncBuffer{}
	stop, _ = e.s.Stop(ctx, p.ID, out)
	waitRun(t, stop)
	mustContain(t, "failed stop", out.String(), "❌ Teardown incomplete for", "🔗 Reconnected edge proxy after failed stop", "❌ STOP FAILED")

	out = &syncBuffer{}
	restart, err := e.s.Restart(ctx, p.ID, out)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, restart)
	mustContain(t, "restart", out.String(), "🔄 RESTARTING PROJECT", "✅ RESTART SUCCESSFUL!")
	if restart.Status() != deploy.StatusSuccess {
		t.Fatalf("restart row = %s", restart.Status())
	}
}

func TestComposeNeverRunsInCheckout(t *testing.T) {
	e := setup(t)
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	ctx := context.Background()
	run, _ := e.start(t, p)
	waitRun(t, run)
	stop, err := e.s.Stop(ctx, p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, stop)
	restart, err := e.s.Restart(ctx, p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, restart)

	want, err := e.s.rt.ComposeWorkDir()
	if err != nil {
		t.Fatal(err)
	}
	e.docker.mu.Lock()
	dirs := append([]string(nil), e.docker.dirs...)
	e.docker.mu.Unlock()
	// up + legacy alias down, two aliases per stop, restart.
	if len(dirs) < 4 {
		t.Fatalf("compose calls = %v", dirs)
	}
	for _, d := range dirs {
		if d != want {
			t.Fatalf("compose ran in %q, want %q (calls %v)", d, want, dirs)
		}
	}
	if entries, err := os.ReadDir(want); err != nil || len(entries) != 0 {
		t.Fatalf("compose working dir must stay empty: %v %v", entries, err)
	}
}

func TestLifecycleRefusedWhileDeploying(t *testing.T) {
	e := setup(t)
	e.docker.buildGate = newGate()
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	run, _ := e.start(t, p)
	waitGate(t, e.docker.buildGate)
	ctx := context.Background()
	if _, err := e.s.Stop(ctx, p.ID, nil); !errors.Is(err, ErrStopWhileDeploying) {
		t.Fatalf("stop: %v", err)
	}
	if _, err := e.s.Restart(ctx, p.ID, nil); !errors.Is(err, ErrRestartWhileDeploying) {
		t.Fatalf("restart: %v", err)
	}
	if _, err := e.s.Rollback(ctx, p.ID, "x", nil); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("rollback: %v", err)
	}
	close(e.docker.buildGate.release)
	waitRun(t, run)
}

func TestRollbackGuards(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.git.head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	p := e.project(t, "web", store.ProjectCreate{}, dockerfileApp)
	first, _ := e.start(t, p)
	waitRun(t, first)
	second, _ := e.start(t, p)
	waitRun(t, second)
	e.docker.buildFail = true
	failed, _ := e.start(t, p)
	waitRun(t, failed)

	cases := []struct {
		name, target string
		status       int
	}{
		{"current success", second.DeploymentID, 400},
		{"failed deployment", failed.DeploymentID, 404},
		{"unknown", "nope", 404},
	}
	for _, tc := range cases {
		_, err := e.s.Rollback(ctx, p.ID, tc.target, nil)
		var re *RequestError
		if !errors.As(err, &re) || re.Status != tc.status {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	db, err := e.d.Store.CreateProject(ctx, store.ProjectCreate{Name: "db", ProjectType: store.Ptr("database")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Rollback(ctx, db.ID, first.DeploymentID, nil); err == nil || !strings.Contains(err.Error(), "managed databases") {
		t.Fatalf("database rollback: %v", err)
	}

	// The first image was kept (keep-2); removing it makes the target unrestorable.
	tag := deploy.DockliftImageTag(p.ID, "app", first.DeploymentID)
	e.docker.mu.Lock()
	delete(e.docker.images, tag)
	e.docker.mu.Unlock()
	_, err = e.s.Rollback(ctx, p.ID, first.DeploymentID, nil)
	var re *RequestError
	if !errors.As(err, &re) || re.Status != 409 || !strings.Contains(re.Message, "Image missing for app") {
		t.Fatalf("missing image: %v", err)
	}
	e.assertUnlocked(t, p.ID)
	last, _ := e.d.Store.FindDeployment(ctx, store.DeploymentFilter{ProjectID: &p.ID}, store.OrderCreatedDesc)
	if strVal(last.Trigger) != "rollback" || strVal(last.Status) != deploy.StatusFailed {
		t.Fatalf("missing-image row = %s/%s", strVal(last.Trigger), strVal(last.Status))
	}

	e.docker.mu.Lock()
	e.docker.images[tag] = true
	e.docker.mu.Unlock()
	if err := e.d.Store.InTx(ctx, func(tx *store.Store) error {
		_, err := tx.UpdateDeployment(ctx, first.DeploymentID, store.DeploymentUpdate{CommitSHA: store.Some(store.Ptr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := gitDir(e.s.rt.ProjectPath(p.ID)); err != nil {
		t.Fatal(err)
	}
	out := &syncBuffer{}
	rb, err := e.s.Rollback(ctx, p.ID, first.DeploymentID, out)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, rb)
	mustContain(t, "rollback", out.String(), "⏪ Restoring deployment", "📌 Resetting git to bbbbbbbbbbbb", "✅ ROLLBACK SUCCESSFUL")
	row := e.deployment(t, rb.DeploymentID)
	if strVal(row.Status) != deploy.StatusSuccess || strVal(row.CommitSHA) != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("rollback row = %s %s", strVal(row.Status), strVal(row.CommitSHA))
	}
	if got, _ := deploy.ParseImageTags(row.ImageTags); got.Len() != 1 {
		t.Fatal("rollback row must carry the restored tags")
	}
	e.assertUnlocked(t, p.ID)

	// A failed compose up rewinds git to the previous HEAD.
	e.git.head = "cccccccccccccccccccccccccccccccccccccccc"
	e.docker.upCode = 1
	out = &syncBuffer{}
	rb, err = e.s.Rollback(ctx, p.ID, first.DeploymentID, out)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, rb)
	mustContain(t, "failed rollback", out.String(), "[ERROR] Rollback failed: compose up exited 1", "Git HEAD restored after failed rollback")
	if rb.Status() != deploy.StatusFailed {
		t.Fatalf("failed rollback row = %s", rb.Status())
	}
}

// gitDir makes dir look like a checkout (gitops.IsRepository).
func gitDir(dir string) error { return os.MkdirAll(filepath.Join(dir, ".git"), 0o755) }
