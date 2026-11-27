package deployments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"docklift/internal/db"
	"docklift/internal/deploy"
	"docklift/internal/dockerx"
	"docklift/internal/gitops"
	"docklift/internal/nginx"
	"docklift/internal/store"
)

var (
	bar50 = strings.Repeat("━", 50)
	bar40 = strings.Repeat("─", 40)
)

// errCancelled is the pipeline's "Deployment cancelled" abort.
var errCancelled = errors.New("Deployment cancelled")

// DeployOptions configure StartDeploy.
type DeployOptions struct {
	// Trigger is stored on the row ("" = "manual"); e.g. "webhook", "redeploy".
	Trigger string
	// CommitMessage is stored on the row; "" reads the HEAD subject of
	// GitHub/public checkouts.
	CommitMessage string
	// Host is the hostname printed in endpoint lines ("" = "localhost").
	Host string
	// Output receives the live deploy log (nil = none). Writes to it must
	// not block; the route passes a buffered client stream.
	Output io.Writer
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// StartDeploy admits a deploy of the project and runs it in the background
// (deployProject): pull, build, runtime compose, compose up, proxy attach,
// domains, status and keep-2 image cleanup. It returns a *RequestError
// (ErrProjectNotFound, ErrFilesNotFound, ErrCancelling, ErrBusy) when the
// deploy is refused; other errors are failures before the pipeline started
// (the route answers 500). At most one deploy, rollback or cancel runs per
// project. ctx bounds only the admission; the deploy itself runs on the
// process background context.
func (s *Service) StartDeploy(ctx context.Context, projectID string, opts DeployOptions) (*Run, error) {
	unlock := s.startMu.lock(projectID)
	defer unlock()

	p, err := s.store.GetProject(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	projectPath := s.rt.ProjectPath(projectID)
	if !exists(projectPath) {
		return nil, ErrFilesNotFound
	}
	t := s.rt.Tracker
	if t.IsCancelling(projectID) {
		return nil, ErrCancelling
	}
	if t.IsDeploying(projectID) {
		return nil, ErrBusy
	}
	if busy, err := s.inProgress(ctx, projectID); err != nil {
		return nil, err
	} else if busy {
		return nil, ErrBusy
	}

	var commitMessage *string
	if opts.CommitMessage != "" {
		commitMessage = &opts.CommitMessage
	} else if src := strVal(p.SourceType); src == "github" || src == "public" {
		if msg, ok := s.git.LastCommitMessage(ctx, projectPath); ok {
			commitMessage = &msg
		}
	}
	trigger := opts.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	dctx := s.dbCtx()
	row, err := s.store.CreateDeployment(dctx, store.DeploymentCreate{
		ProjectID: projectID, Status: store.Ptr(deploy.StatusInProgress), Trigger: &trigger,
		CommitMessage: commitMessage, Logs: store.Ptr("🚀 Starting deployment...\n"),
	})
	if err != nil {
		return nil, err
	}
	if _, ok := t.Acquire(projectID, row.ID); !ok {
		now := db.Now()
		_, _ = s.store.UpdateDeployment(dctx, row.ID, store.DeploymentUpdate{
			Status: store.Some(store.Ptr(deploy.StatusFailed)), FinishedAt: store.Some(&now),
			Logs: store.Some(store.Ptr("❌ Another deployment is already running for this project\n")),
		})
		return nil, ErrBusy
	}
	if err := s.rt.SetProjectAndServicesStatus(dctx, projectID, deploy.StatusBuilding); err != nil {
		logs := "\n❌ Error: " + err.Error() + "\n"
		s.rt.FailDeployment(dctx, projectID, row.ID, &logs)
		t.Release(projectID, row.ID)
		return nil, err
	}

	l := newDeployLog(opts.Output)
	l.startSync(dctx, s.store, row.ID)
	run := newRun(row.ID)
	s.running.Add(1)
	go s.runDeploy(run, p, l, opts)
	return run, nil
}

// inProgress reports whether the project has an in_progress deployment row.
func (s *Service) inProgress(ctx context.Context, projectID string) (bool, error) {
	status := deploy.StatusInProgress
	_, err := s.store.FindDeployment(ctx, store.DeploymentFilter{ProjectID: &projectID, Status: &status}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// runDeploy runs the pipeline and guarantees, whatever happens (errors,
// panics), that the row reaches a terminal status, the lock and the process
// registration are released and Done is closed.
func (s *Service) runDeploy(run *Run, p *store.Project, l *deployLog, opts DeployOptions) {
	defer s.running.Done()
	defer s.finish(run)
	pid, id := p.ID, run.DeploymentID
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("deploy pipeline panicked", "project", pid, "deployment", id, "panic", fmt.Sprint(rec))
			s.failDeploy(pid, id, l, errors.New("internal error"))
		}
		l.endSync()
		s.ensureTerminal(id, l)
		t := s.rt.Tracker
		t.ClearProcess(pid, id)
		t.Release(pid, id)
		t.ForgetCancelled(id)
	}()
	if err := s.deploy(p, id, l, opts); err != nil {
		s.failDeploy(pid, id, l, err)
	}
}

// ensureTerminal marks the row failed if it is still in_progress (a path
// that lost the lock without the cancel updating the row, or a panic).
func (s *Service) ensureTerminal(id string, l *deployLog) {
	inProgress := deploy.StatusInProgress
	now := db.Now()
	logs := l.final()
	_, _ = s.store.UpdateDeployments(s.dbCtx(), store.DeploymentFilter{ID: &id, Status: &inProgress}, store.DeploymentUpdate{
		Status: store.Some(store.Ptr(deploy.StatusFailed)), FinishedAt: store.Some(&now), Logs: store.Some(&logs),
	})
}

// failDeploy is the catch block of deployProject: a cancelled deployment is
// recorded as cancelled, anything else as failed (never overwriting a
// cancel); the client sees the error line either way.
func (s *Service) failDeploy(pid, id string, l *deployLog, err error) {
	s.log.Error("deployment failed", "project", pid, "deployment", id, "error", err)
	t := s.rt.Tracker
	cancelled := t.WasBuildCancelled(pid, id) || t.IsMarkedCancelled(id) || !t.Owns(pid, id)
	t.ClearProcess(pid, id)
	l.endSync()
	dctx := s.dbCtx()
	if cancelled {
		l.record("\n❌ Deployment cancelled\n")
		logs := l.final()
		now := db.Now()
		_, _ = s.store.UpdateDeployment(dctx, id, store.DeploymentUpdate{
			Status: store.Some(store.Ptr(deploy.StatusCancelled)), Logs: store.Some(&logs), FinishedAt: store.Some(&now),
		})
	} else {
		l.record("\n❌ Error: " + err.Error() + "\n")
		logs := l.final()
		s.rt.FailDeployment(dctx, pid, id, &logs)
	}
	t.ForgetCancelled(id)
	t.Release(pid, id)
	l.toClient("\n❌ Error: " + err.Error() + "\n")
}

// process runs fn as the project's registered command (cancel kills it).
func (s *Service) process(pid, id string, fn func(ctx context.Context) error) error {
	ctx, finish := s.rt.Tracker.StartProcess(s.bg, pid, id)
	defer finish()
	return fn(ctx)
}

// svcData is one service of the deploy (servicesData).
type svcData struct {
	Name           string
	DockerfilePath string
	InternalPort   int
	Port           *int
	ContainerName  string
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// deploy runs the pipeline up to and including compose up. A returned error
// goes to failDeploy; once compose ran, the outcome is handled here.
func (s *Service) deploy(p *store.Project, id string, l *deployLog, opts DeployOptions) error {
	pid := p.ID
	ctx := s.dbCtx()
	t := s.rt.Tracker
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	projectPath := s.rt.ProjectPath(pid)
	composeProject := deploy.ComposeProjectName(p.Name, pid)

	l.line("🚀 Starting deployment...\n")

	if strVal(p.SourceType) == "github" && strVal(p.GithubURL) != "" {
		if err := s.pullGitHub(p, id, l); err != nil {
			return err
		}
	}

	if src := strVal(p.SourceType); (src == "github" || src == "public") && gitops.IsRepository(projectPath) {
		if sha, ok := s.git.CommitSHA(ctx, projectPath); ok {
			if _, err := s.store.UpdateDeployment(ctx, id, store.DeploymentUpdate{CommitSHA: store.Some(&sha)}); err == nil {
				logf("📌 Commit: %s\n", sha[:min(12, len(sha))])
			}
		}
	}

	if t.IsMarkedCancelled(id) {
		return errCancelled
	}

	managed := strVal(p.ProjectType) == "database" && strVal(p.DBEngine) != ""
	var engine deploy.Engine
	if managed {
		var ok bool
		if engine, ok = deploy.GetEngine(*p.DBEngine); !ok {
			return fmt.Errorf("Unknown managed database engine: %s", *p.DBEngine)
		}
	}

	envRows, err := s.store.ListEnvVariables(ctx, store.EnvFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		return err
	}
	envAll := deploy.EnvVarsFromStore(envRows)
	for _, v := range envAll {
		if v.IsSecret {
			l.redact(v.Value)
		}
	}
	volumes, err := s.store.ListVolumes(ctx, pid, store.OrderCreatedAsc)
	if err != nil {
		return err
	}
	composePath := s.rt.ComposePath(pid)

	var runtimeServices []deploy.RuntimeService
	var services []svcData
	if managed {
		rs, sd, err := s.prepareManaged(p, id, engine, envAll, volumes, l)
		if err != nil {
			return err
		}
		runtimeServices, services = []deploy.RuntimeService{rs}, []svcData{sd}
	} else {
		runtimeServices, services, err = s.buildApp(p, id, envAll, volumes, l)
		if err != nil {
			return err
		}
	}

	if managed {
		logf("\n%s\n", bar40)
		l.line("📝 Generating docker-compose.yml...\n")
		if len(envAll) > 0 {
			logf("   🔐 Including %d database credential env var(s)\n", len(envAll))
		}
	}
	if t.IsMarkedCancelled(id) || !t.Owns(pid, id) {
		return errCancelled
	}
	if err := deploy.WriteRuntimeCompose(composePath, runtimeServices, envAll,
		deploy.RuntimeComposeOptions{ProjectID: pid, PublishHostPort: p.PublishHostPort}); err != nil {
		return err
	}
	l.line("✅ DockLift runtime compose created outside the repository\n")
	logf("   Network: %s (proxy attached after up)\n\n", deploy.ProjectNetworkName(pid))
	logf("%s\n", bar40)
	l.line("🚀 Starting containers...\n")
	logf("   Compose project: %s\n", composeProject)
	logf("%s\n\n", bar40)

	// Tear down legacy UUID-named compose projects once (pre-slug naming).
	for _, alias := range deploy.ComposeProjectAliases(p.Name, pid) {
		if alias == composeProject {
			continue
		}
		_, _ = s.docker.Compose(s.bg, dockerx.ComposeCmd{Project: alias, Dir: projectPath, Args: []string{"down", "--remove-orphans"}, Timeout: composeDownTimeout})
	}

	if t.IsMarkedCancelled(id) || s.rt.IsDeploymentCancelled(ctx, pid, id) || !t.Owns(pid, id) {
		return errCancelled
	}
	files, err := s.rt.ComposeFiles(pid)
	if err != nil {
		return err
	}

	procCtx, finishProc := t.StartProcess(s.bg, pid, id)
	res, runErr := s.docker.Compose(procCtx, dockerx.ComposeCmd{
		Project: composeProject, Files: files, Dir: projectPath,
		Args: []string{"up", "-d", "--remove-orphans"}, Output: l,
	})
	killed := procCtx.Err() != nil
	finishProc()
	if runErr != nil && !killed {
		s.composeError(pid, id, l, runErr)
		return nil
	}
	var code *int
	if !killed {
		code = &res.ExitCode
	}
	s.afterCompose(p, id, l, code, opts, managed, engine, runtimeServices, services)
	return nil
}

// composeDownTimeout bounds the best-effort legacy teardown of deploys.
const composeDownTimeout = 60 * time.Second

// composeError is the onError callback of compose up: the command did not run.
func (s *Service) composeError(pid, id string, l *deployLog, err error) {
	t := s.rt.Tracker
	ctx := s.dbCtx()
	l.line("\n❌ Docker execution error: " + err.Error() + "\n")
	superseded := !t.Owns(pid, id)
	l.endSync()
	if superseded || s.rt.IsDeploymentCancelled(ctx, pid, id) {
		logs := l.final()
		now := db.Now()
		_, _ = s.store.UpdateDeployment(ctx, id, store.DeploymentUpdate{
			Status: store.Some(store.Ptr(deploy.StatusCancelled)), Logs: store.Some(&logs), FinishedAt: store.Some(&now),
		})
	} else {
		logs := l.final()
		s.rt.FailDeployment(ctx, pid, id, &logs)
	}
	t.ForgetCancelled(id)
	t.Release(pid, id)
}

// pullGitHub refreshes the installation token and pulls (the token is sent
// as a request header, never stored in the checkout), then scrubs and
// verifies the origin remote as the TS code did.
func (s *Service) pullGitHub(p *store.Project, id string, l *deployLog) error {
	pid := p.ID
	projectPath := s.rt.ProjectPath(pid)
	cleanURL := *p.GithubURL
	var creds *gitops.Credentials
	tokenSet := false
	if owner, repo, ok := gitops.GitHubOwnerRepo(cleanURL); ok {
		token, err := s.installationToken(owner, repo)
		if err != nil {
			l.line("⚠️ Token refresh warning: " + err.Error() + "\n")
		} else {
			l.redact(token)
			creds = gitops.InstallationCredentials(token)
			tokenSet = true
			l.line("🔑 Refreshed GitHub access token\n")
		}
	}
	pullErr := s.process(pid, id, func(ctx context.Context) error {
		return s.git.Pull(ctx, projectPath, l, gitops.PullOptions{Ref: strVal(p.GithubBranch), Credentials: creds})
	})
	scrubFailed := false
	if tokenSet {
		ctx := s.dbCtx()
		err := s.git.ScrubOrigin(ctx, projectPath, cleanURL)
		if err == nil {
			err = s.git.VerifyNoCredentials(ctx, projectPath)
		}
		if err != nil {
			scrubFailed = true
			l.line("❌ Failed to scrub Git credentials: " + err.Error() + "\n")
			_ = s.git.SetOriginURL(ctx, projectPath, cleanURL)
		}
	}
	if pullErr != nil {
		return pullErr
	}
	if scrubFailed {
		return errors.New("Failed to scrub Git credentials after pull")
	}
	return nil
}

// errNoGitHubApp is the token warning when no GitHub App is wired.
var errNoGitHubApp = errors.New("GitHub App is not configured")

func (s *Service) installationToken(owner, repo string) (string, error) {
	a := s.githubApp()
	if a == nil {
		return "", errNoGitHubApp
	}
	ctx := s.bg
	installID, err := a.InstallationIDForRepo(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return a.InstallationToken(ctx, installID)
}

// prepareManaged creates or updates the managed database service and pulls
// its image.
func (s *Service) prepareManaged(p *store.Project, id string, engine deploy.Engine, envAll []deploy.EnvVar,
	volumes []*store.PersistentVolume, l *deployLog) (deploy.RuntimeService, svcData, error) {
	pid := p.ID
	ctx := s.dbCtx()
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	name := engine.ServiceName
	svc, err := s.store.FindService(ctx, store.ServiceFilter{ProjectID: &pid, Name: &name}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		svc, err = nil, nil
	}
	if err != nil {
		return deploy.RuntimeService{}, svcData{}, err
	}
	marker := ""
	if svc != nil {
		marker = svc.DockerfilePath
	}
	image := deploy.ImageForManagedService(engine, marker)

	logf("\n%s\n", bar50)
	logf("🗄️  MANAGED DATABASE: %s\n", engine.Label)
	logf("   Image: %s\n", image)
	logf("%s\n\n", bar50)
	if p.PublishHostPort {
		l.line("  🔓 Host port publish enabled (exposes the DB on the host — prefer linking)\n")
	} else {
		l.line("  🔒 Host ports off — link this database to apps (Docker DNS), or opt-in publish\n")
	}

	containerName := deploy.ServiceContainerName(p.Name, pid, name)
	if svc == nil {
		var port *int
		if p.PublishHostPort {
			n, err := s.rt.AllocatePort(ctx, pid)
			if err != nil {
				return deploy.RuntimeService{}, svcData{}, err
			}
			port = &n
		}
		svc, err = s.store.CreateService(ctx, store.ServiceCreate{
			ProjectID: pid, Name: name, DockerfilePath: deploy.ManagedServiceMarker(engine.ID, image),
			ContainerName: &containerName, InternalPort: store.Ptr(engine.Port), Port: port, Status: store.Ptr(deploy.StatusBuilding),
		})
	} else {
		upd := store.ServiceUpdate{
			Status:         store.Some(store.Ptr(deploy.StatusBuilding)),
			DockerfilePath: store.Some(deploy.ManagedServiceMarker(engine.ID, image)),
			InternalPort:   store.Some(store.Ptr(engine.Port)),
			ContainerName:  store.Some(&containerName),
		}
		if old := strVal(svc.ContainerName); old != containerName && old != "" {
			logf("     🛠️ Migrating container name → %s\n", containerName)
			s.docker.RemoveContainer(s.bg, old)
		}
		if p.PublishHostPort {
			if svc.Port == nil {
				n, err := s.rt.AllocatePort(ctx, pid)
				if err != nil {
					return deploy.RuntimeService{}, svcData{}, err
				}
				upd.Port = store.Some(&n)
			}
		} else if svc.Port != nil {
			if err := s.rt.ReleaseServicePort(ctx, pid, *svc.Port); err != nil {
				return deploy.RuntimeService{}, svcData{}, err
			}
			upd.Port = store.Null[int]()
		}
		svc, err = s.store.UpdateService(ctx, svc.ID, upd)
	}
	if err != nil {
		return deploy.RuntimeService{}, svcData{}, err
	}

	logf("  🐳 %s: %s\n", svc.Name, image)
	logf("     Internal port: %d\n", engine.Port)
	logf("     Container: %s\n", strVal(svc.ContainerName))
	logf("\n%s\n", bar40)
	logf("📥 Pulling %s...\n", image)
	if err := s.process(pid, id, func(ctx context.Context) error {
		return s.docker.PullImage(ctx, image, dockerx.PullOptions{Output: l, Timeout: dockerx.DefaultPullTimeout})
	}); err != nil {
		return deploy.RuntimeService{}, svcData{}, err
	}
	if s.rt.IsDeploymentCancelled(ctx, pid, id) {
		return deploy.RuntimeService{}, svcData{}, errCancelled
	}
	l.final()
	l.syncNow()
	l.line("✅ Image ready\n")

	envMap := map[string]string{}
	for _, v := range envAll {
		envMap[v.Key] = v.Value
	}
	creds, ok := deploy.CredentialsFromEnv(engine, envMap)
	if !ok {
		return deploy.RuntimeService{}, svcData{}, errors.New("Managed database credentials missing from project env")
	}
	l.redact(creds.Password)
	correctMount := deploy.VolumeMountForEngine(engine, image)
	for _, v := range volumes {
		if v.ServiceName != svc.Name || v.MountPath == correctMount {
			continue
		}
		if engine.ID == deploy.EnginePostgres && v.MountPath == "/var/lib/postgresql/data" && correctMount == "/var/lib/postgresql" {
			l.line("  ⚠️ Postgres 18+ was stored with mount /var/lib/postgresql/data.\n" +
				"     Official images require /var/lib/postgresql. Updating the mount path.\n" +
				"     If this DB already wrote data under the old mount, it may live on an\n" +
				"     anonymous Docker volume and will NOT appear after redeploy — recreate\n" +
				"     + restore from backup if you need that data.\n")
		} else {
			logf("     🛠️ Updating volume mount %s → %s\n", v.MountPath, correctMount)
		}
		if _, err := s.store.UpdateVolumeMountPath(ctx, v.ID, correctMount); err != nil {
			return deploy.RuntimeService{}, svcData{}, err
		}
		v.MountPath = correctMount
	}

	sd := svcData{Name: svc.Name, DockerfilePath: svc.DockerfilePath, InternalPort: engine.Port, Port: svc.Port, ContainerName: strVal(svc.ContainerName)}
	rs := deploy.RuntimeService{
		Name: svc.Name, Image: image, InternalPort: engine.Port, Port: svc.Port, ContainerName: sd.ContainerName,
		Command: deploy.EngineCommand(engine, creds), NoPortEnv: true, Volumes: runtimeVolumes(volumes, svc.Name),
	}
	return rs, sd, nil
}

// runtimeVolumes are the compose mounts of a service's persistent volumes.
func runtimeVolumes(volumes []*store.PersistentVolume, service string) []deploy.RuntimeVolume {
	var out []deploy.RuntimeVolume
	for _, v := range volumes {
		if v.ServiceName != service {
			continue
		}
		out = append(out, deploy.RuntimeVolume{Key: deploy.StorageVolumeComposeKey(service, len(out), v.Name), Name: v.Name, MountPath: v.MountPath})
	}
	return out
}

// buildApp resolves the build, reconciles the service rows and builds every
// service image.
func (s *Service) buildApp(p *store.Project, id string, envAll []deploy.EnvVar, volumes []*store.PersistentVolume,
	l *deployLog) ([]deploy.RuntimeService, []svcData, error) {
	pid := p.ID
	ctx := s.dbCtx()
	t := s.rt.Tracker
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	projectPath := s.rt.ProjectPath(pid)
	internalPort := p.InternalPort
	resolved, err := deploy.ResolveProjectBuild(projectPath, deploy.BuildSettings{
		BuildType: p.BuildType, BaseDirectory: p.BaseDirectory, DockerfilePath: p.DockerfilePath, InternalPort: &internalPort,
	})
	if err != nil {
		return nil, nil, err
	}
	dfPath := func(b deploy.ResolvedBuildService) string {
		if b.DockerfilePath != nil && *b.DockerfilePath != "" {
			return *b.DockerfilePath
		}
		return "[railpack]"
	}

	logf("\n%s\n", bar50)
	logf("📦 BUILD: %s\n", resolved.Detected)
	logf("%s\n\n", bar50)

	active := make([]string, 0, len(resolved.Services))
	for _, b := range resolved.Services {
		active = append(active, b.Name)
	}
	stale, err := s.store.ListServices(ctx, store.ServiceFilter{ProjectID: &pid, NameNotIn: active}, store.OrderNone)
	if err != nil {
		return nil, nil, err
	}
	for _, st := range stale {
		logf("  🧹 Removing stale service: %s\n", st.Name)
		s.edge.CleanupServiceDomain(s.bg, st.ID)
		if c := strVal(st.ContainerName); c != "" {
			s.docker.RemoveContainer(s.bg, c)
		}
		if st.Port != nil {
			if err := s.rt.ReleaseServicePort(ctx, pid, *st.Port); err != nil {
				return nil, nil, err
			}
		}
		if err := s.store.DeleteService(ctx, st.ID); err != nil {
			return nil, nil, err
		}
	}

	if p.PublishHostPort {
		l.line("  🔓 Host port publish enabled for this project\n")
	} else {
		l.line("  🔒 Host ports off — reach services via domain / nginx-proxy (opt-in publish_host_port)\n")
	}

	var services []svcData
	for i, b := range resolved.Services {
		path := dfPath(b)
		icon := "🐳"
		if b.Builder == deploy.BuildRailpack {
			icon = "🛤️"
		}
		logf("  %s %s: %s\n", icon, b.Name, path)
		logf("     Internal port: %d\n", b.InternalPort)
		sd, err := s.reconcileService(p, b, path, i == 0, l)
		if err != nil {
			return nil, nil, err
		}
		services = append(services, sd)
	}

	logf("\n%s\n", bar40)
	l.line("📝 Generating docker-compose.yml...\n")
	if len(envAll) > 0 {
		shared := 0
		for _, v := range envAll {
			if v.ServiceName == deploy.SharedEnvService {
				shared++
			}
		}
		if scoped := len(envAll) - shared; scoped > 0 {
			logf("   🔐 Including %d environment variable(s) (%d shared, %d service-scoped)\n", len(envAll), shared, scoped)
		} else {
			logf("   🔐 Including %d environment variable(s)\n", len(envAll))
		}
	}

	for _, b := range resolved.Services {
		if b.Builder != deploy.BuildDockerfile {
			continue
		}
		path := dfPath(b)
		full := filepath.Join(projectPath, filepath.FromSlash(path))
		var public, secret []string
		for _, v := range deploy.EnvForService(envAll, b.Name) {
			switch {
			case v.IsBuildArg && v.IsSecret:
				secret = append(secret, v.Key)
			case v.IsBuildArg:
				public = append(public, v.Key)
			}
		}
		if len(public) > 0 {
			if missing := deploy.ValidateDockerBuildArgs(full, public); len(missing) > 0 {
				logf("\n⚠️  WARNING: The following build arguments are configured but missing 'ARG' instructions in %s:\n", path)
				for _, a := range missing {
					logf("    - %s\n", a)
				}
				logf("    These variables will NOT be available during the build process! Please add \"ARG %s\" to your Dockerfile.\n\n", missing[0])
			}
		}
		var missingSecrets []string
		for _, k := range secret {
			if !deploy.DockerfileMountsSecret(full, k) {
				missingSecrets = append(missingSecrets, k+" (in "+path+")")
			}
		}
		if len(missingSecrets) > 0 {
			l.line("\n❌ PREFLIGHT FAILED: secret build vars need Dockerfile mounts:\n")
			for _, item := range missingSecrets {
				logf("    - %s\n", item)
			}
			l.line("    Add: RUN --mount=type=secret,id=<KEY> …\n")
			return nil, nil, fmt.Errorf("Secret build vars missing Dockerfile mounts: %s", strings.Join(missingSecrets, ", "))
		}
	}

	var runtimeServices []deploy.RuntimeService
	for _, sd := range services {
		if t.IsMarkedCancelled(id) {
			return nil, nil, errCancelled
		}
		var plan *deploy.ResolvedBuildService
		for i := range resolved.Services {
			if resolved.Services[i].Name == sd.Name {
				plan = &resolved.Services[i]
				break
			}
		}
		if plan == nil {
			return nil, nil, fmt.Errorf("Build plan missing service %s", sd.Name)
		}
		tag := deploy.DockliftImageTag(pid, sd.Name, id)
		logf("\n%s\n", bar40)
		err := s.process(pid, id, func(ctx context.Context) error {
			return deploy.BuildServiceImage(ctx, deploy.BuildRequest{
				ProjectPath: projectPath, StatePath: s.rt.StatePath(pid), Service: *plan, ImageTag: tag,
				EnvVars: deploy.EnvForService(envAll, sd.Name), Log: l, Run: s.buildRun,
			})
		})
		l.final()
		if err != nil {
			return nil, nil, err
		}
		runtimeServices = append(runtimeServices, deploy.RuntimeService{
			Name: sd.Name, Image: tag, InternalPort: sd.InternalPort, Port: sd.Port, ContainerName: sd.ContainerName,
			Volumes: runtimeVolumes(volumes, sd.Name),
		})
	}
	return runtimeServices, services, nil
}

// reconcileService creates or updates the service row of one build service.
func (s *Service) reconcileService(p *store.Project, b deploy.ResolvedBuildService, path string, first bool, l *deployLog) (svcData, error) {
	pid := p.ID
	ctx := s.dbCtx()
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	name := b.Name
	target := deploy.ServiceContainerName(p.Name, pid, name)
	svc, err := s.store.FindService(ctx, store.ServiceFilter{ProjectID: &pid, Name: &name}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		svc, err = nil, nil
	}
	if err != nil {
		return svcData{}, err
	}
	building := store.Some(store.Ptr(deploy.StatusBuilding))
	if svc == nil {
		var port *int
		if p.PublishHostPort {
			n, err := s.rt.AllocatePort(ctx, pid)
			if err != nil {
				return svcData{}, err
			}
			port = &n
		}
		assignDomain := strVal(p.Domain) != "" && first
		var domain *string
		if assignDomain {
			domain = p.Domain
		}
		svc, err = s.store.CreateService(ctx, store.ServiceCreate{
			ProjectID: pid, Name: name, DockerfilePath: path, ContainerName: &target,
			InternalPort: store.Ptr(b.InternalPort), Port: port, Domain: domain, Status: store.Ptr(deploy.StatusBuilding),
		})
		if err != nil {
			return svcData{}, err
		}
		hostPort := " (no host port)"
		if port != nil {
			hostPort = " (Host port: " + strconv.Itoa(*port) + ")"
		}
		withDomain := ""
		if assignDomain {
			withDomain = " with domain: " + *p.Domain
		}
		logf("     Assigned new service: %s%s%s\n", name, hostPort, withDomain)
		logf("     Container: %s\n", target)
	} else {
		if strVal(svc.ContainerName) != target {
			logf("     🛠️ Migrating container name → %s\n", target)
			logf("     🛑 Removing old container: %s\n", strVal(svc.ContainerName))
			if c := strVal(svc.ContainerName); c != "" {
				s.docker.RemoveContainer(s.bg, c)
			}
			if svc, err = s.store.UpdateService(ctx, svc.ID, store.ServiceUpdate{ContainerName: store.Some(&target)}); err != nil {
				return svcData{}, err
			}
		}
		upd := store.ServiceUpdate{Status: building, DockerfilePath: store.Some(path), InternalPort: store.Some(store.Ptr(b.InternalPort))}
		var msg string
		switch {
		case p.PublishHostPort && (svc.Port == nil || *svc.Port == 0):
			n, err := s.rt.AllocatePort(ctx, pid)
			if err != nil {
				return svcData{}, err
			}
			upd.Port = store.Some(&n)
			msg = fmt.Sprintf("     Assigned host port to service: %s (Port: %d)\n", name, n)
		case p.PublishHostPort:
			msg = fmt.Sprintf("     Updating existing service: %s (Host port: %d)\n", name, *svc.Port)
		case svc.Port != nil:
			if err := s.rt.ReleaseServicePort(ctx, pid, *svc.Port); err != nil {
				return svcData{}, err
			}
			upd.Port = store.Null[int]()
			msg = fmt.Sprintf("     Updating existing service: %s (host port released)\n", name)
		default:
			msg = fmt.Sprintf("     Updating existing service: %s\n", name)
		}
		updated, err := s.store.UpdateService(ctx, svc.ID, upd)
		if err != nil {
			return svcData{}, err
		}
		if upd.Port.Set {
			svc = updated
		}
		l.line(msg)
	}
	return svcData{Name: name, DockerfilePath: path, InternalPort: b.InternalPort, Port: svc.Port, ContainerName: strVal(svc.ContainerName)}, nil
}

// afterCompose is the onClose callback of compose up: it holds the lock
// through proxy attach, status, domains and image cleanup, re-checking on
// every step that a cancel did not supersede the deploy.
func (s *Service) afterCompose(p *store.Project, id string, l *deployLog, code *int, opts DeployOptions,
	managed bool, engine deploy.Engine, runtimeServices []deploy.RuntimeService, services []svcData) {
	pid := p.ID
	ctx := s.dbCtx()
	t := s.rt.Tracker
	logf := func(format string, a ...any) { l.line(fmt.Sprintf(format, a...)) }
	stillOwns := func() bool { return t.Owns(pid, id) }
	l.final()
	l.syncNow()

	markCancelledAndRelease := func() {
		logf("\n%s\n❌ DEPLOY CANCELLED\n%s\n", bar50, bar50)
		l.endSync()
		logs := l.final()
		now := db.Now()
		_, _ = s.store.UpdateDeployment(ctx, id, store.DeploymentUpdate{
			Status: store.Some(store.Ptr(deploy.StatusCancelled)), Logs: store.Some(&logs), FinishedAt: store.Some(&now),
		})
		t.ForgetCancelled(id)
		t.Release(pid, id)
	}
	stopIfSuperseded := func() bool {
		if !stillOwns() {
			l.line("\n⚠️ Deploy lost project lock (cancel/redeploy) — skipping further work\n")
			t.ForgetCancelled(id)
			return true
		}
		if s.rt.IsDeploymentCancelled(ctx, pid, id) {
			markCancelledAndRelease()
			return true
		}
		return false
	}

	if stopIfSuperseded() {
		return
	}
	success := code != nil && *code == 0
	if success {
		if !stillOwns() {
			stopIfSuperseded()
			return
		}
		err := s.rt.ConnectProxyToProjectNetwork(s.bg, pid)
		if !stillOwns() {
			s.rt.DisconnectProxyFromProjectNetwork(s.bg, pid)
			stopIfSuperseded()
			return
		}
		switch {
		case err == nil:
			l.line("🔗 Edge proxy attached to project network\n")
		case managed:
			logf("⚠️ Edge proxy attach skipped/failed (OK for internal databases): %s\n", errOr(err, "failed"))
		default:
			success = false
			logf("\n❌ Edge proxy attach FAILED: %s\n   Domains will NOT be activated (containers may still be running).\n", errOr(err, "failed"))
		}
	}
	if stopIfSuperseded() {
		return
	}

	switch {
	case success:
		host := opts.Host
		if host == "" {
			host = "localhost"
		}
		logf("\n%s\n", bar50)
		l.line("✅ DEPLOY SUCCESSFUL!\n")
		logf("%s\n\n", bar50)
		if managed {
			logf("🗄️  %s is running privately on the project network.\n", engine.Label)
			l.line("   Link it from /databases or a project’s Attach database panel.\n")
			l.line("   Prefer linking over publishing host ports.\n")
		} else {
			l.line("🌐 ENDPOINTS:\n")
			anyHost := false
			for _, sd := range services {
				if sd.Port != nil && *sd.Port != 0 {
					anyHost = true
					logf("  📍 %s: http://%s:%d\n", sd.Name, host, *sd.Port)
				}
			}
			if !anyHost {
				l.line("  📍 Host ports disabled — use your custom domain (nginx-proxy → container DNS)\n")
			}
		}
		if !stillOwns() {
			stopIfSuperseded()
			return
		}
		var linkErr error
		if managed {
			if linkErr = s.rt.ReapplyDatabaseLinksForDatabase(s.bg, pid); linkErr == nil {
				l.line("🔗 Re-applied database network links\n")
			}
		} else {
			_, _, linkErr = s.rt.ReapplyDatabaseLinksForApp(s.bg, pid)
		}
		if linkErr != nil {
			logf("⚠️ Link re-apply warning: %s\n", linkErr.Error())
		}
		if !stillOwns() {
			if !managed {
				_ = s.rt.DisconnectLinkedDatabasesFromApp(s.bg, pid)
			}
			stopIfSuperseded()
			return
		}
		if stopIfSuperseded() {
			return
		}
	case code == nil:
		logf("\n%s\n❌ DEPLOY FAILED (Exit Code: null)\n%s\n", bar50, bar50)
	case *code != 0:
		logf("\n%s\n❌ DEPLOY FAILED (Exit Code: %d)\n%s\n", bar50, *code, bar50)
	default:
		logf("\n%s\n❌ DEPLOY FAILED (edge proxy not attached)\n%s\n", bar50, bar50)
	}

	if stopIfSuperseded() {
		return
	}
	status := deploy.StatusFailed
	var tags deploy.ImageTags
	if success {
		status = deploy.StatusSuccess
		for _, rs := range runtimeServices {
			if strings.HasPrefix(rs.Image, "docklift-") {
				tags.Set(rs.Name, rs.Image)
			}
		}
	}
	l.endSync()
	logs := l.final()
	now := db.Now()
	upd := store.DeploymentUpdate{Status: store.Some(&status), Logs: store.Some(&logs), FinishedAt: store.Some(&now)}
	if tags.Len() > 0 {
		upd.ImageTags = store.Some(tags.JSON())
	}
	cancelled := deploy.StatusCancelled
	n, err := s.store.UpdateDeployments(ctx, store.DeploymentFilter{ID: &id, StatusNot: &cancelled}, upd)
	if err != nil || n == 0 {
		t.ForgetCancelled(id)
		t.Release(pid, id)
		return
	}
	if stopIfSuperseded() {
		return
	}

	if success {
		if !stillOwns() {
			stopIfSuperseded()
			return
		}
		if _, err := s.rt.SyncProjectStatus(s.bg, pid); err != nil {
			logf("⚠️ Status sync warning: %s\n", err.Error())
		}
		if stopIfSuperseded() {
			return
		}
		if err := s.activateServiceDomains(pid, stillOwns); err != nil {
			if errors.Is(err, nginx.ErrCancelled) || !stillOwns() || s.rt.IsDeploymentCancelled(ctx, pid, id) {
				stopIfSuperseded()
				return
			}
			logf("⚠️ Domain activation warning: %s\n", errOr(err, "failed"))
		} else {
			if !stillOwns() {
				stopIfSuperseded()
				return
			}
			l.line("🌐 Nginx domains activated\n")
		}
		if stopIfSuperseded() {
			return
		}
		if stillOwns() && !s.rt.IsDeploymentCancelled(ctx, pid, id) {
			l.line("\n🧹 Post-deploy cleanup...\n")
			res := s.rt.PostDeployCleanup(s.bg, pid, tags, l)
			l.line("   " + res.Message + "\n")
			if stopIfSuperseded() {
				return
			}
		}
	} else {
		if !stillOwns() {
			stopIfSuperseded()
			return
		}
		_ = s.rt.SetProjectAndServicesStatus(ctx, pid, deploy.StatusError)
	}
	t.Release(pid, id)
	if success {
		l.line("\n📊 Deployment complete! Status: SUCCESS ✅\n")
	} else {
		l.line("\n📊 Deployment complete! Status: FAILED ❌\n")
	}
	t.ForgetCancelled(id)
}

// activateServiceDomains writes the vhosts of every service with a domain
// (as running); shouldContinue aborts before each service and nginx step.
func (s *Service) activateServiceDomains(pid string, shouldContinue func() bool) error {
	services, err := s.store.ListServices(s.dbCtx(), store.ServiceFilter{ProjectID: &pid}, store.OrderNone)
	if err != nil {
		return err
	}
	for _, svc := range services {
		if !shouldContinue() {
			return nginx.ErrCancelled
		}
		if strVal(svc.Domain) == "" || strVal(svc.ContainerName) == "" {
			continue
		}
		running := *svc
		running.Status = store.Ptr(deploy.StatusRunning)
		if err := s.edge.UpdateServiceDomain(s.bg, running, nginx.UpdateOptions{
			ShouldContinue: func(context.Context) bool { return shouldContinue() },
		}); err != nil {
			return err
		}
	}
	return nil
}

// errOr is `err?.message || fallback`.
func errOr(err error, fallback string) string {
	if err == nil || err.Error() == "" {
		return fallback
	}
	return err.Error()
}
