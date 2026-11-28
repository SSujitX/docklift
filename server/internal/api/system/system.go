// Package system serves /api/system:
// host statistics, the public IP, the release check, SSE logs of the DockLift
// containers, the step-up protected host actions (purge, reboot, reset,
// system update, upgrade) and the password-gated command runner.
package system

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/client"

	"docklift/internal/api/logs"
	"docklift/internal/app"
	"docklift/internal/auth"
	"docklift/internal/dockerx"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
	"docklift/internal/sysinfo"
	"docklift/internal/version"
)

// coreContainers is every container of the DockLift stack (docker-compose.yml),
// restarted by POST /reset.
var coreContainers = []string{"docklift-backend", "docklift-nginx-proxy", "docklift-certbot"}

// selfContainer is the container this process runs in; /reset restarts it last.
const selfContainer = "docklift-backend"

// logServices maps the friendly names of GET /logs/:service to containers,
// in the order the error message lists them.
var logServices = []struct{ name, container string }{
	{"backend", "docklift-backend"},
	{"proxy", "docklift-nginx-proxy"},
	{"certbot", "docklift-certbot"},
}

// Delays of the simulated host actions on development machines.
const (
	simulatedDelay      = 2 * time.Second
	simulatedResetDelay = 1500 * time.Millisecond
	resetDelay          = time.Second
	resetTimeout        = 2 * time.Minute
	dockerPingTimeout   = 5 * time.Second
)

// Docker is the part of the Engine API client the system routes use;
// *client.Client implements it.
type Docker interface {
	logs.Docker
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerRestart(ctx context.Context, containerID string, options client.ContainerRestartOptions) (client.ContainerRestartResult, error)
}

// Options are the dependencies of the system routes that Mount cannot take
// from app.Deps. Zero fields take production defaults.
type Options struct {
	// Images replaces the keep-2 image retention of Deps.Deploy used by
	// POST /purge. With neither, purge answers 500 "Failed to complete purge
	// operation".
	Images ImageCleanup
	// Docker replaces Deps.Docker.
	Docker Docker
	// Host replaces the platform's privileged host actions.
	Host Host
	// Source replaces the host statistics reader.
	Source sysinfo.Source
	// GOOS replaces runtime.GOOS (the dev-machine simulations).
	GOOS string
	// Shell replaces /bin/bash for POST /execute.
	Shell string
	// GitHubURL and PublicIPURL replace the release and IP probes.
	GitHubURL   string
	PublicIPURL string
	// HTTPClient performs those probes.
	HTTPClient *http.Client
	// SimulatedDelay replaces the 2 s / 1.5 s delays of the simulations.
	SimulatedDelay *time.Duration
}

type handlers struct {
	d       *app.Deps
	images  ImageCleanup
	docker  Docker
	host    Host
	stats   *sysinfo.Collector
	version *versionChecker
	goos    string
	shell   string
	delay   *time.Duration
}

// Mount registers the /api/system routes on r, which is already scoped to
// /api/system and wrapped in the group's authentication middleware (SSE query
// token for GET /logs/*, session JWT otherwise). Docker comes from
// Deps.Docker and purge's image retention from Deps.Deploy.
func Mount(r chi.Router, d *app.Deps) {
	MountWith(r, d, Options{})
}

// MountWith is Mount with explicit dependencies.
func MountWith(r chi.Router, d *app.Deps, opts Options) {
	h := &handlers{d: d, images: opts.Images, docker: opts.Docker, host: opts.Host, goos: opts.GOOS, shell: opts.Shell, delay: opts.SimulatedDelay}
	if h.docker == nil && d.Docker != nil {
		h.docker = d.Docker
	}
	if h.images == nil && d.Deploy != nil {
		h.images = runtimeImages{rt: d.Deploy}
	}
	if h.host == nil {
		h.host = newHost(d.Background, d.Log)
	}
	if h.goos == "" {
		h.goos = runtime.GOOS
	}
	if h.shell == "" {
		h.shell = executeShell
	}
	src := opts.Source
	if src == nil {
		src = sysinfo.NewHostSource()
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: versionProbeTimeout}
	}
	h.stats = sysinfo.NewCollector(d.Background, src, d.Log, h.goos, sysinfo.Options{PublicIPURL: opts.PublicIPURL, HTTPClient: httpClient})
	h.version = &versionChecker{background: d.Background, current: version.Version, url: GitHubLatestReleaseURL, http: httpClient, log: d.Log, now: time.Now}
	if opts.GitHubURL != "" {
		h.version.url = opts.GitHubURL
	}

	r.Get("/stats", h.statsHandler)
	r.Get("/logs/{service}", h.logStream)
	r.Get("/quick", h.quick)
	r.Get("/ip", h.ip)
	r.Post("/purge", h.purge)
	r.Post("/reboot", h.reboot)
	r.Post("/reset", h.reset)
	r.Post("/execute", h.execute)
	r.Get("/version", h.versionHandler)
	r.Post("/update-system", h.updateSystem)
	r.Post("/upgrade", h.upgrade)
}

// logsDocker is the Docker client for log streams (nil when none).
func (h *handlers) logsDocker() logs.Docker {
	if h.docker == nil {
		return nil
	}
	return h.docker
}

// GET /stats — every host statistic (cached 3 s).
func (h *handlers) statsHandler(w http.ResponseWriter, r *http.Request) {
	s, err := h.stats.Stats(r.Context())
	if err != nil {
		h.d.Log.Error("Failed to get system stats", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch system statistics")
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// GET /logs/:service — SSE logs of a DockLift container.
func (h *handlers) logStream(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	i := slices.IndexFunc(logServices, func(s struct{ name, container string }) bool { return s.name == service })
	if i < 0 {
		names := make([]string, len(logServices))
		for j, s := range logServices {
			names[j] = s.name
		}
		httpx.Error(w, http.StatusBadRequest, "Invalid service name. Available: "+strings.Join(names, ", "))
		return
	}
	logs.Stream(w, r, h.logsDocker(), logServices[i].container, logs.Tail(r))
}

// GET /quick — CPU and memory percent for the header widget.
func (h *handlers) quick(w http.ResponseWriter, r *http.Request) {
	q, err := h.stats.Quick(r.Context())
	if err != nil {
		h.d.Log.Error("Failed to fetch quick stats", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch quick stats")
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}

// GET /ip — the server's public IP ("N/A" when unknown).
func (h *handlers) ip(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, struct {
		IP string `json:"ip"`
	}{h.stats.PublicIP(r.Context())})
}

// GET /version — current and latest release (?refresh=1 forces a probe).
func (h *handlers) versionHandler(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh")
	httpx.JSON(w, http.StatusOK, h.version.Get(r.Context(), refresh == "1" || refresh == "true"))
}

type messageBody struct {
	Message string `json:"message"`
}

type failureBody struct {
	Error   string `json:"error"`
	Details string `json:"details"`
}

// simulate waits like the dev-mode branches, then answers msg.
func (h *handlers) simulate(w http.ResponseWriter, r *http.Request, delay time.Duration, msg string) {
	if h.delay != nil {
		delay = *h.delay
	}
	select {
	case <-time.After(delay):
	case <-r.Context().Done():
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Message: msg})
}

// stepUp runs requireStepUpPassword; false means the response is written.
func (h *handlers) stepUp(w http.ResponseWriter, r *http.Request, failMsg string) bool {
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.d.Log.Error(failMsg, "error", err)
		httpx.JSON(w, http.StatusInternalServerError, failureBody{Error: failMsg, Details: err.Error()})
		return false
	}
	return ok
}

func (h *handlers) devMachine() bool { return h.goos == "windows" || h.goos == "darwin" }

// POST /reboot — reboot the host (simulated on Windows/macOS).
func (h *handlers) reboot(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to initiate reboot"
	if !h.stepUp(w, r, failMsg) {
		return
	}
	h.d.Log.Info("[AUDIT] System reboot initiated from IP: " + httpx.ClientIP(r))
	if h.devMachine() {
		h.d.Log.Info("Reboot requested (Dev Mode: Simulation)")
		h.simulate(w, r, simulatedDelay, "Dev Mode: Simulated Server Reboot successful.")
		return
	}
	if err := h.host.Reboot(); err != nil {
		h.d.Log.Error("Reboot error", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, failureBody{Error: failMsg, Details: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Message: "System is rebooting now..."})
}

// POST /reset — restart the DockLift containers (simulated on Windows).
func (h *handlers) reset(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to reset services"
	if !h.stepUp(w, r, failMsg) {
		return
	}
	h.d.Log.Info("[AUDIT] System reset initiated from IP: " + httpx.ClientIP(r))
	if h.goos == "windows" {
		h.d.Log.Info("Reset Service requested (Dev Mode: Simulation)")
		h.simulate(w, r, simulatedResetDelay, "Dev Mode: Simulated Service Reset complete.")
		return
	}
	if err := h.pingDocker(r.Context()); err != nil {
		if logs.IsDockerUnavailable(err) {
			h.d.Log.Warn("Reset error: Docker unavailable", "error", err)
			httpx.Error(w, http.StatusServiceUnavailable, logs.DockerUnavailableMessage)
			return
		}
		h.d.Log.Error("Reset error", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, failureBody{Error: failMsg, Details: err.Error()})
		return
	}
	h.d.Log.Info("Reset triggered. Restarting: " + strings.Join(coreContainers, " "))
	go h.restartCoreContainers()
	httpx.JSON(w, http.StatusOK, messageBody{Message: "Services reset command sent successfully (Restarting in 1s)"})
}

func (h *handlers) pingDocker(ctx context.Context) error {
	if h.docker == nil {
		return logs.ErrDockerNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, dockerPingTimeout)
	defer cancel()
	_, err := h.docker.Ping(ctx, client.PingOptions{})
	return err
}

// restartCoreContainers is `sleep 1 && docker restart <core containers>`,
// except that this process's own container goes last: restarting it first
// (as 2.x did) killed the restart before it reached the
// other containers. Containers that do not exist (for example a certbot
// container that was never created) are skipped.
func (h *handlers) restartCoreContainers() {
	time.Sleep(resetDelay)
	// Detached from shutdown: restarting our own container stops this process.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(h.d.Background), resetTimeout)
	defer cancel()
	order := make([]string, 0, len(coreContainers))
	for _, c := range coreContainers {
		if c != selfContainer {
			order = append(order, c)
		}
	}
	order = append(order, selfContainer)
	for _, name := range order {
		_, err := h.docker.ContainerRestart(ctx, name, client.ContainerRestartOptions{})
		if dockerx.IsNotFound(err) {
			continue
		}
		if err != nil {
			h.d.Log.Error("Reset execution ended: restart "+name+" failed", "error", err)
		}
	}
}

// POST /update-system — apt update/upgrade on the host (simulated on
// Windows/macOS).
func (h *handlers) updateSystem(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to start system update"
	if !h.stepUp(w, r, failMsg) {
		return
	}
	h.d.Log.Info("[AUDIT] System update initiated from IP: " + httpx.ClientIP(r))
	if h.devMachine() {
		h.d.Log.Info("System update requested (Dev Mode: Simulation)")
		h.simulate(w, r, simulatedDelay, "Dev Mode: Simulated system update complete.")
		return
	}
	if err := h.host.UpdateSystem(); err != nil {
		h.d.Log.Error("System update error", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, failureBody{Error: failMsg, Details: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Message: "System update started on host. This may take several minutes."})
}

// POST /upgrade — run upgrade.sh on the host (simulated on Windows/macOS).
func (h *handlers) upgrade(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to start upgrade"
	if !h.stepUp(w, r, failMsg) {
		return
	}
	h.d.Log.Info("[AUDIT] Docklift upgrade initiated from IP: " + httpx.ClientIP(r))
	if h.devMachine() {
		h.d.Log.Info("Docklift upgrade requested (Dev Mode: Simulation)")
		h.simulate(w, r, simulatedDelay, "Dev Mode: Simulated upgrade complete. No changes made to system.")
		return
	}
	if err := h.host.Upgrade(); err != nil {
		h.d.Log.Error("Upgrade error", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, failureBody{Error: failMsg, Details: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, messageBody{Message: "Docklift upgrade started. The application will restart shortly."})
}

type passwordRequiredBody struct {
	Error           string `json:"error"`
	RequirePassword bool   `json:"requirePassword"`
}

type executeBody struct {
	Output string  `json:"output"`
	Error  *string `json:"error,omitempty"`
}

// POST /execute — run a shell command after re-verifying the account
// password (the JWT alone is not enough).
func (h *handlers) execute(w http.ResponseWriter, r *http.Request) {
	body := httpx.BodyFrom(r)
	command, isString := body.Get("command").String()
	if !isString || command == "" {
		httpx.Error(w, http.StatusBadRequest, "Command is required")
		return
	}
	authUser, _ := auth.UserFrom(r.Context())
	if authUser.UserID == "" {
		httpx.Error(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	if handled, ok := h.d.Auth.TokenStepUp(w, r); handled {
		// An API token has no password to re-enter: only a token created
		// with allow_dangerous may run commands.
		if !ok {
			return
		}
		h.d.Log.Info(fmt.Sprintf("[TERMINAL] API token %q (%s) of %q executed: %s", authUser.Token.Name, authUser.Token.Prefix, authUser.Email, js.Substring(command, 200)))
		h.runCommand(w, r, command)
		return
	}
	password, isString := body.Get("password").String()
	if !isString || password == "" {
		httpx.JSON(w, http.StatusUnauthorized, passwordRequiredBody{Error: "Password required for terminal access", RequirePassword: true})
		return
	}
	user, err := h.d.Store.GetUser(r.Context(), authUser.UserID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusForbidden, "No user account found")
		return
	}
	if err != nil {
		// 2.x answered every failure as command output.
		h.d.Log.Error("execute: user lookup failed", "error", err)
		msg := err.Error()
		httpx.JSON(w, http.StatusOK, executeBody{Output: "", Error: &msg})
		return
	}
	if !h.d.Auth.VerifyStepUpPassword(w, r, user.ID, user.Password, password) {
		return
	}
	h.d.Log.Info(fmt.Sprintf("[TERMINAL] User %q executed: %s", user.Email, js.Substring(command, 200)))
	h.runCommand(w, r, command)
}

// runCommand runs an authorised /execute command and answers its output.
func (h *handlers) runCommand(w http.ResponseWriter, r *http.Request, command string) {
	res := runShell(r.Context(), h.shell, command)
	if res.Failed {
		msg := res.Stderr
		if msg == "" {
			msg = res.Message
		}
		if msg == "" {
			msg = "Command failed"
		}
		httpx.JSON(w, http.StatusOK, executeBody{Output: res.Stdout, Error: &msg})
		return
	}
	out := executeBody{Output: res.Stdout}
	if out.Output == "" {
		out.Output = res.Stderr
	}
	if out.Output == "" {
		out.Output = "(no output)"
	}
	if res.Stderr != "" && res.Stdout == "" {
		out.Error = &res.Stderr
	}
	httpx.JSON(w, http.StatusOK, out)
}
