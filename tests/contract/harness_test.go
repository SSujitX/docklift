package contract

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Target selection (see README.md):
//
//	DOCKLIFT_GO_BIN=<path>        boot that binary on a fresh temp dir per instance
//	DOCKLIFT_BASE_URL=http://...  use a running server (DOCKLIFT_DATA_DIR optional)
const (
	// Values the harness passes to every booted server. Tests assert behaviour
	// that depends on them (CORS allowlist, panel redirects, port pool).
	corsAllowedOrigin = "http://cors-allowed.test:4000"
	frontendURL       = "http://frontend.test:3000"
	portRangeStart    = 5500
	portRangeEnd      = 5510
)

// serverEnvKeys are the environment variables the server reads that the
// harness controls. They are stripped from the inherited environment so a
// developer's shell cannot change server behaviour.
var serverEnvKeys = []string{
	"PORT", "DATABASE_URL", "DATA_PATH", "DEPLOYMENTS_PATH", "BACKUP_PATH",
	"NGINX_CONF_PATH", "LETSENCRYPT_PATH", "DOCKER_HOST", "DOCKER_NETWORK", "JWT_SECRET",
	"INTERNAL_API_SECRET", "CORS_ORIGIN", "DOCKLIFT_FRONTEND_URL", "DOCKLIFT_GITHUB_APP_ID",
	"DOCKLIFT_GITHUB_CLIENT_ID", "DOCKLIFT_GITHUB_CLIENT_SECRET", "DOCKLIFT_GITHUB_PRIVATE_KEY_PATH",
	"PORT_RANGE_START", "PORT_RANGE_END", "CERTBOT_CONTAINER", "CERTBOT_EMAIL", "CERTBOT_STAGING",
	"NGINX_PROXY_CONTAINER", "TERMINAL_ALLOW_NO_ORIGIN",
	// Stripped so the default (loopback trusted) applies unless a test sets it.
	"TRUSTED_PROXIES",
}

type config struct {
	target  string // go | remote
	goBin   string
	baseURL string
	dataDir string
	docker  bool
	network bool
	keepTmp bool
}

var (
	cfg     config
	runRoot string

	instMu       sync.Mutex
	allInstances []*Instance

	mainInst *Instance
	admin    *Session
)

// Session is a logged-in user on one instance.
type Session struct {
	Inst     *Instance
	UserID   string
	Name     string
	Email    string
	Password string
	Token    string
}

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	if err := loadConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "contract: config:", err)
		return 2
	}
	var err error
	runRoot, err = os.MkdirTemp("", "docklift-contract-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "contract:", err)
		return 2
	}
	code := 1
	defer func() {
		stopAll()
		cleanupDocker()
		if code != 0 {
			dumpLogs()
		}
		if !cfg.keepTmp {
			removeAllRetry(runRoot)
		} else {
			fmt.Fprintln(os.Stderr, "contract: kept temp dir", runRoot)
		}
	}()

	if cfg.target == "remote" {
		mainInst = &Instance{Name: "remote", BaseURL: strings.TrimRight(cfg.baseURL, "/"), DataDir: cfg.dataDir}
	} else {
		mainInst, err = bootInstance(BootOptions{Name: "main"})
		if err != nil {
			fmt.Fprintln(os.Stderr, "contract: boot main:", err)
			dumpLogs()
			return 2
		}
		if err := startEdgeProxy(mainInst); err != nil {
			fmt.Fprintln(os.Stderr, "contract: edge proxy:", err)
			return 2
		}
	}
	admin, err = ensureAdmin(mainInst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "contract: admin setup:", err)
		code = 2
		return code
	}
	code = m.Run()
	return code
}

func loadConfig() error {
	cfg.target = "go"
	cfg.baseURL = strings.TrimSpace(os.Getenv("DOCKLIFT_BASE_URL"))
	cfg.dataDir = strings.TrimSpace(os.Getenv("DOCKLIFT_DATA_DIR"))
	cfg.goBin = strings.TrimSpace(os.Getenv("DOCKLIFT_GO_BIN"))
	cfg.docker = os.Getenv("DOCKLIFT_DOCKER") == "1"
	cfg.network = os.Getenv("DOCKLIFT_NETWORK") == "1"
	cfg.keepTmp = os.Getenv("DOCKLIFT_KEEP_TMP") == "1"
	if cfg.baseURL != "" {
		cfg.target = "remote"
		return nil
	}
	if cfg.goBin == "" {
		return errors.New("set DOCKLIFT_GO_BIN to a built server binary (or DOCKLIFT_BASE_URL to a running server)")
	}
	abs, err := filepath.Abs(cfg.goBin)
	if err != nil {
		return err
	}
	cfg.goBin = abs
	if _, err := os.Stat(cfg.goBin); err != nil {
		return fmt.Errorf("DOCKLIFT_GO_BIN: %w", err)
	}
	return nil
}

// Instance is one running Docklift server.
type Instance struct {
	Name    string
	BaseURL string
	DataDir string // empty when the data dir is not reachable from the test process
	Root    string
	Dirs    InstanceDirs
	Env     map[string]string

	mu      sync.Mutex
	cmd     *exec.Cmd
	exited  chan struct{}
	logPath string
	logFile *os.File
}

// InstanceDirs are the server paths of a booted instance.
type InstanceDirs struct {
	Data, Deployments, Backups, Nginx, LetsEncrypt, DBFile string
}

// BootOptions customise a dedicated instance.
type BootOptions struct {
	Name string
	// Prepare runs after the directories exist and before migrations/boot.
	Prepare func(d InstanceDirs) error
	// ExtraEnv overrides harness defaults (keep to a minimum).
	ExtraEnv map[string]string
}

func (i *Instance) Booted() bool { return i.Root != "" }

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func fileURL(p string) string {
	return "file:" + filepath.ToSlash(p)
}

func bootInstance(opts BootOptions) (*Instance, error) {
	root := filepath.Join(runRoot, opts.Name)
	d := InstanceDirs{
		Data:        filepath.Join(root, "data"),
		Deployments: filepath.Join(root, "deployments"),
		Backups:     filepath.Join(root, "backups"),
		Nginx:       filepath.Join(root, "nginx"),
		LetsEncrypt: filepath.Join(root, "letsencrypt"),
	}
	d.DBFile = filepath.Join(d.Data, "smoke.db")
	for _, p := range []string{d.Data, d.Deployments, d.Backups, d.Nginx, d.LetsEncrypt} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, err
		}
	}
	inst := &Instance{Name: opts.Name, Root: root, Dirs: d, DataDir: d.Data, logPath: filepath.Join(root, "server.log")}
	inst.Env = map[string]string{
		"DATABASE_URL":                     fileURL(d.DBFile),
		"DATA_PATH":                        d.Data,
		"DEPLOYMENTS_PATH":                 d.Deployments,
		"BACKUP_PATH":                      d.Backups,
		"NGINX_CONF_PATH":                  d.Nginx,
		"LETSENCRYPT_PATH":                 d.LetsEncrypt,
		"DOCKER_HOST":                      "tcp://127.0.0.1:1",
		"JWT_SECRET":                       "",
		"INTERNAL_API_SECRET":              "",
		"CORS_ORIGIN":                      corsAllowedOrigin,
		"DOCKLIFT_FRONTEND_URL":            frontendURL,
		"DOCKLIFT_GITHUB_APP_ID":           "",
		"DOCKLIFT_GITHUB_CLIENT_ID":        "",
		"DOCKLIFT_GITHUB_CLIENT_SECRET":    "",
		"DOCKLIFT_GITHUB_PRIVATE_KEY_PATH": filepath.Join(root, "github-app.pem"),
		"DOCKER_NETWORK":                   "docklift_contract_network",
		"PORT_RANGE_START":                 fmt.Sprint(portRangeStart),
		"PORT_RANGE_END":                   fmt.Sprint(portRangeEnd),
		"CERTBOT_CONTAINER":                "docklift-contract-certbot",
		"CERTBOT_EMAIL":                    "",
		"CERTBOT_STAGING":                  "true",
		"NGINX_PROXY_CONTAINER":            "docklift-contract-nginx-proxy",
		"TERMINAL_ALLOW_NO_ORIGIN":         "",
	}
	if cfg.docker {
		// Opt-in: talk to the caller's Docker daemon for requireDocker tests.
		inst.Env["DOCKER_HOST"] = os.Getenv("DOCKER_HOST")
	}
	for k, v := range opts.ExtraEnv {
		inst.Env[k] = v
	}
	if opts.Prepare != nil {
		if err := opts.Prepare(d); err != nil {
			return nil, fmt.Errorf("prepare %s: %w", opts.Name, err)
		}
	}
	instMu.Lock()
	allInstances = append(allInstances, inst)
	instMu.Unlock()
	if err := inst.Start(); err != nil {
		return nil, err
	}
	return inst, nil
}

func linkDir(target, link string) error {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			return fmt.Errorf("mklink /J: %v: %s", err, out)
		}
		return nil
	}
	return os.Symlink(target, link)
}

func baseEnv(extra map[string]string) []string {
	strip := map[string]bool{}
	for _, k := range serverEnvKeys {
		strip[strings.ToUpper(k)] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strip[strings.ToUpper(k)] {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// Start launches the server process on a fresh port and waits for /api/health.
func (i *Instance) Start() error {
	port, err := freePort()
	if err != nil {
		return err
	}
	i.Env["PORT"] = fmt.Sprint(port)
	i.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	if cfg.target != "go" {
		return errors.New("cannot start a remote instance")
	}
	cmd := exec.Command(cfg.goBin)
	cmd.Dir = i.Root
	cmd.Env = baseEnv(i.Env)
	lf, err := os.OpenFile(i.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	fmt.Fprintf(lf, "\n===== start %s (%s) on port %d at %s =====\n", i.Name, cfg.target, port, time.Now().Format(time.RFC3339))
	cmd.Stdout = lf
	cmd.Stderr = lf
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		lf.Close()
		return err
	}
	exited := make(chan struct{})
	i.mu.Lock()
	i.cmd, i.exited, i.logFile = cmd, exited, lf
	i.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	return i.waitHealthy(90 * time.Second)
}

func (i *Instance) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	hc := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-i.exited:
			return fmt.Errorf("%s exited during startup; log:\n%s", i.Name, i.Log())
		default:
		}
		resp, err := hc.Get(i.BaseURL + "/api/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s not healthy after %s; log:\n%s", i.Name, timeout, i.Log())
}

// Stop kills the whole process tree (the server spawns docker/git children).
func (i *Instance) Stop() {
	i.mu.Lock()
	cmd, exited, lf := i.cmd, i.exited, i.logFile
	i.cmd = nil
	i.mu.Unlock()
	if cmd == nil {
		return
	}
	select {
	case <-exited:
	default:
		killTree(cmd)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
		}
	}
	if lf != nil {
		lf.Close()
	}
}

// WaitExit reports whether the server process exited on its own within d.
func (i *Instance) WaitExit(d time.Duration) bool {
	i.mu.Lock()
	exited := i.exited
	i.mu.Unlock()
	if exited == nil {
		return false
	}
	select {
	case <-exited:
		return true
	case <-time.After(d):
		return false
	}
}

// Restart starts the instance again after it stopped or exited.
func (i *Instance) Restart() error {
	i.Stop()
	return i.Start()
}

func (i *Instance) Log() string {
	if i.logPath == "" {
		return ""
	}
	b, _ := os.ReadFile(i.logPath)
	return string(b)
}

func stopAll() {
	instMu.Lock()
	list := append([]*Instance(nil), allInstances...)
	instMu.Unlock()
	for _, inst := range list {
		inst.Stop()
	}
}

func dumpLogs() {
	instMu.Lock()
	list := append([]*Instance(nil), allInstances...)
	instMu.Unlock()
	for _, inst := range list {
		fmt.Fprintf(os.Stderr, "\n----- server log: %s (%s) -----\n%s\n----- end %s -----\n", inst.Name, inst.logPath, inst.Log(), inst.Name)
	}
}

func removeAllRetry(p string) {
	for n := 0; n < 20; n++ {
		if err := os.RemoveAll(p); err == nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "contract: could not remove", p)
}

// ---------------------------------------------------------------------------
// Dedicated instances (booted lazily, torn down in TestMain)

var (
	dedicatedMu sync.Mutex
	dedicated   = map[string]*Instance{}
	dedErr      = map[string]error{}
)

// requireBoot skips when the suite runs against a remote server.
func requireBoot(t *testing.T) {
	t.Helper()
	if !mainInst.Booted() {
		t.Skip("needs a dedicated instance booted by the harness (not available with DOCKLIFT_BASE_URL)")
	}
}

// dedicatedInstance boots (once per run) an extra server for tests that must
// not mutate the shared instance.
func dedicatedInstance(t *testing.T, opts BootOptions) *Instance {
	t.Helper()
	requireBoot(t)
	dedicatedMu.Lock()
	defer dedicatedMu.Unlock()
	if inst, ok := dedicated[opts.Name]; ok {
		return inst
	}
	if err, ok := dedErr[opts.Name]; ok {
		t.Fatalf("instance %s failed to boot earlier: %v", opts.Name, err)
	}
	inst, err := bootInstance(opts)
	if err != nil {
		dedErr[opts.Name] = err
		t.Fatalf("boot %s: %v", opts.Name, err)
	}
	dedicated[opts.Name] = inst
	return inst
}

// writeSeedDB places testdata/seed.db.gz at the instance DB path.
func writeSeedDB(d InstanceDirs) error {
	raw, err := seedDBBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(d.DBFile, raw, 0o644)
}

func seedDBBytes() ([]byte, error) {
	f, err := os.Open(filepath.Join("testdata", "seed.db.gz"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

// knownSecrets is written to DATA_PATH/.secrets for instances whose tests mint
// their own JWTs (the on-disk format of the secrets file).
var knownSecrets = struct {
	JWT      string `json:"jwtSecret"`
	Internal string `json:"internalApiSecret"`
}{JWT: strings.Repeat("c0ffee", 16), Internal: strings.Repeat("5ec7e7", 16)}

func writeKnownSecrets(d InstanceDirs) error {
	b, _ := json.MarshalIndent(knownSecrets, "", "  ")
	return os.WriteFile(filepath.Join(d.Data, ".secrets"), b, 0o600)
}

// readSecrets returns the server's persisted secrets when the data dir is reachable.
func readSecrets(t *testing.T, inst *Instance) (jwtSecret, internal string) {
	t.Helper()
	if inst.DataDir == "" {
		t.Skip("needs DOCKLIFT_DATA_DIR to read .secrets")
	}
	b, err := os.ReadFile(filepath.Join(inst.DataDir, ".secrets"))
	if err != nil {
		t.Skipf("cannot read .secrets: %v", err)
	}
	var s struct {
		JWT      string `json:"jwtSecret"`
		Internal string `json:"internalApiSecret"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("parse .secrets: %v", err)
	}
	return s.JWT, s.Internal
}

// ---------------------------------------------------------------------------
// Admin bootstrap for the shared instance

func ensureAdmin(inst *Instance) (*Session, error) {
	c := inst.Anon()
	st, err := c.raw("GET", "/api/auth/status", nil, nil)
	if err != nil {
		return nil, err
	}
	var status struct {
		SetupComplete bool `json:"setupComplete"`
	}
	if err := json.Unmarshal(st.Body, &status); err != nil {
		return nil, fmt.Errorf("status: %v: %s", err, st.Body)
	}
	if status.SetupComplete {
		email, pw := os.Getenv("DOCKLIFT_ADMIN_EMAIL"), os.Getenv("DOCKLIFT_ADMIN_PASSWORD")
		if email == "" || pw == "" {
			return nil, errors.New("server is already set up: set DOCKLIFT_ADMIN_EMAIL and DOCKLIFT_ADMIN_PASSWORD")
		}
		return login(inst, email, pw)
	}
	secret := os.Getenv("DOCKLIFT_BOOTSTRAP_SECRET")
	if secret == "" {
		if inst.DataDir == "" {
			return nil, errors.New("fresh server: set DOCKLIFT_BOOTSTRAP_SECRET or DOCKLIFT_DATA_DIR")
		}
		b, err := os.ReadFile(filepath.Join(inst.DataDir, ".bootstrap-secret"))
		if err != nil {
			return nil, err
		}
		secret = strings.TrimSpace(string(b))
	}
	s := &Session{Inst: inst, Name: "Contract Admin", Email: "contract-admin@contract.test", Password: "ContractPassw0rd!"}
	resp, err := c.raw("POST", "/api/auth/register", map[string]any{"name": s.Name, "email": s.Email, "password": s.Password},
		map[string]string{"X-Bootstrap-Secret": secret})
	if err != nil {
		return nil, err
	}
	if resp.Status != 201 {
		return nil, fmt.Errorf("register: %d %s", resp.Status, resp.Body)
	}
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, err
	}
	s.Token, s.UserID = out.Token, out.User.ID
	return s, nil
}

func login(inst *Instance, email, password string) (*Session, error) {
	resp, err := inst.Anon().raw("POST", "/api/auth/login", map[string]any{"email": email, "password": password}, nil)
	if err != nil {
		return nil, err
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("login %s: %d %s", email, resp.Status, resp.Body)
	}
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, err
	}
	return &Session{Inst: inst, UserID: out.User.ID, Name: out.User.Name, Email: email, Password: password, Token: out.Token}, nil
}

func mustLogin(t *testing.T, inst *Instance, email, password string) *Session {
	t.Helper()
	s, err := login(inst, email, password)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// requireDocker marks tests that need a real Docker daemon on the server.
func requireDocker(t *testing.T) {
	t.Helper()
	if !cfg.docker {
		t.Skip("needs a Docker daemon (set DOCKLIFT_DOCKER=1)")
	}
}

// requireNoDocker marks assertions about how the server behaves when Docker
// is unreachable (the harness boots with DOCKER_HOST=tcp://127.0.0.1:1).
func requireNoDocker(t *testing.T) {
	t.Helper()
	if cfg.docker {
		t.Skip("asserts no-Docker behaviour; DOCKLIFT_DOCKER=1 is set")
	}
}

// requireNetwork marks tests whose server-side code calls the public internet.
func requireNetwork(t *testing.T) {
	t.Helper()
	if !cfg.network {
		t.Skip("server would call the public internet (set DOCKLIFT_NETWORK=1)")
	}
}

func readFileBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
