package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"docklift/internal/execx"
)

// RailpackVersion pins the Railpack BuildKit frontend.
const RailpackVersion = "0.33.0"

// railpackFrontendDigest is the OCI image index digest of
// railpack-frontend:v0.33.0 on ghcr.io (covers linux/amd64 and linux/arm64).
// Update it together with RailpackVersion.
const railpackFrontendDigest = "sha256:822711c4cb8aa7c03a6556d128186acab4aa49073431195e20617af83f742600"

// RailpackFrontend is the BUILDKIT_SYNTAX image of Railpack plans, pinned by
// digest so a re-pushed tag cannot change the frontend that runs builds.
const RailpackFrontend = "ghcr.io/railwayapp/railpack-frontend:v" + RailpackVersion + "@" + railpackFrontendDigest

// railpackPlaceholder replaces build variable values during `railpack
// prepare`, keeping secrets out of argv and the plan file.
const railpackPlaceholder = "__DOCKLIFT_BUILD_SECRET__"

var protectedBuildEnvRe = regexp.MustCompile(`(?i)^(?:PATH|HOME|PORT|NODE_OPTIONS|LD_PRELOAD|LD_LIBRARY_PATH|DOCKER_.+|BUILDKIT_.+)$`)

var (
	npmLockRe    = regexp.MustCompile(`(?is)npm ci.*can only install packages|package\.json and package-lock\.json.*in sync`)
	staleLockRe  = regexp.MustCompile(`(?is)frozen lockfile|lockfile.*outdated|lockfile.*not.*up.to.date`)
	buildTailMax = 128 * 1024

	unsafeFileCharRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// SummarizeBuildFailure turns a failed build's output into an actionable
// message (summarizeBuildFailure).
func SummarizeBuildFailure(command, output string, code int) string {
	if npmLockRe.MatchString(output) {
		return `package.json and package-lock.json are out of sync. Run "npm install" in the project, commit the updated package-lock.json, then redeploy.`
	}
	if staleLockRe.MatchString(output) {
		return "The dependency lockfile is out of sync with the project manifest. Regenerate and commit the lockfile with the project package manager, then redeploy."
	}
	return fmt.Sprintf("%s exited with code %d", command, code)
}

// CommandRunner runs an external program; execx.Run in production.
type CommandRunner func(ctx context.Context, c execx.Cmd) (execx.Result, error)

// BuildRequest is one service image build.
type BuildRequest struct {
	// ProjectPath is the source checkout; StatePath the runtime state dir
	// (Railpack plan files go there).
	ProjectPath string
	StatePath   string
	Service     ResolvedBuildService
	// ImageTag is the tag to build (DockliftImageTag).
	ImageTag string
	// EnvVars are the service's variables (EnvForService); only build args
	// are used.
	EnvVars []EnvVar
	// Log receives build output and progress lines; values of build
	// variables are never written to it by Docklift.
	Log io.Writer
	// Run executes docker/railpack (nil = execx.Run). Cancelling ctx kills
	// the build.
	Run CommandRunner
	// DockerBin and RailpackBin override the program names (default
	// "docker" and "railpack").
	DockerBin   string
	RailpackBin string
}

// BuildServiceImage builds one service image with docker build (Dockerfile;
// buildx with BuildKit secrets when secret build vars exist) or Railpack
// (prepare a plan, then buildx with the Railpack frontend), streaming the
// output to req.Log (buildServiceImage). Secret build variables are passed
// only as BuildKit secrets through the child environment, never as
// --build-arg; Railpack receives every build variable that way.
func BuildServiceImage(ctx context.Context, req BuildRequest) error {
	logw := req.Log
	if logw == nil {
		logw = io.Discard
	}
	logf := func(format string, a ...any) { _, _ = fmt.Fprintf(logw, format, a...) }
	run := req.Run
	if run == nil {
		run = execx.Run
	}
	dockerBin := orDefault(req.DockerBin, "docker")
	railpackBin := orDefault(req.RailpackBin, "railpack")
	svc := req.Service
	contextDir := filepath.Join(absOr(req.ProjectPath), filepath.FromSlash(svc.ContextPath))

	for _, v := range req.EnvVars {
		if !envKeyRe.MatchString(v.Key) {
			return fmt.Errorf("Invalid environment variable name: %s", v.Key)
		}
	}
	for _, v := range req.EnvVars {
		if v.IsBuildArg && protectedBuildEnvRe.MatchString(v.Key) {
			return fmt.Errorf("Build variable %s is reserved by DockLift and cannot be overridden", v.Key)
		}
	}

	if svc.Builder == BuildDockerfile {
		if svc.DockerfilePath == nil || *svc.DockerfilePath == "" {
			return errors.New("Resolved Dockerfile path is missing")
		}
		dockerfile := filepath.Join(absOr(req.ProjectPath), filepath.FromSlash(*svc.DockerfilePath))
		var buildArgs, secrets []EnvVar
		for _, v := range req.EnvVars {
			if !v.IsBuildArg {
				continue
			}
			if v.IsSecret {
				secrets = append(secrets, v)
			} else {
				buildArgs = append(buildArgs, v)
			}
		}
		args := []string{"build", "--progress", "plain", "-t", req.ImageTag, "-f", dockerfile}
		if len(secrets) > 0 {
			// BuildKit (buildx) is required for --secret.
			args = []string{"buildx", "build", "--load", "--progress", "plain", "-t", req.ImageTag, "-f", dockerfile}
		}
		for _, v := range buildArgs {
			args = append(args, "--build-arg", v.Key+"="+v.Value)
		}
		var extraEnv []string
		for _, v := range secrets {
			args = append(args, "--secret", "id="+v.Key+",env="+v.Key)
			extraEnv = append(extraEnv, v.Key+"="+v.Value)
			logf("🔐 Passing %s as BuildKit secret (not build-arg)\n", v.Key)
		}
		args = append(args, contextDir)
		logf("🐳 Building %s with %s\n", svc.Name, *svc.DockerfilePath)
		return runBuild(ctx, run, execx.Cmd{Name: dockerBin, Args: args, Dir: contextDir, ExtraEnv: extraEnv}, logw)
	}

	if err := os.MkdirAll(req.StatePath, 0o755); err != nil {
		return fmt.Errorf("create build state dir: %w", err)
	}
	safeName := unsafeFileCharRe.ReplaceAllString(svc.Name, "-")
	planPath := filepath.Join(req.StatePath, safeName+"-railpack-plan.json")
	infoPath := filepath.Join(req.StatePath, safeName+"-railpack-info.json")
	var buildVars []EnvVar
	for _, v := range req.EnvVars {
		if v.IsBuildArg {
			buildVars = append(buildVars, v)
		}
	}
	port := strconv.Itoa(svc.InternalPort)
	prepare := []string{"prepare", contextDir, "--plan-out", planPath, "--info-out", infoPath, "--env", "PORT=" + port}
	for _, v := range buildVars {
		prepare = append(prepare, "--env", v.Key+"="+railpackPlaceholder)
	}
	// railpack prepare runs in the backend container and follows symlinks
	// when it reads manifests (package.json, Procfile, ...); what it reads
	// ends up in the plan and the image. docker build and BuildKit send
	// symlinks as links, so only this step needs the check.
	if err := CheckProjectSymlinks(req.ProjectPath); err != nil {
		return err
	}
	logf("🛤️  No Dockerfile found → analyzing %s with Railpack\n", svc.ContextPath)
	if err := runBuild(ctx, run, execx.Cmd{Name: railpackBin, Args: prepare, Dir: contextDir}, logw); err != nil {
		return err
	}

	args := []string{
		"buildx", "build", "--load", "--progress", "plain",
		"--build-arg", "BUILDKIT_SYNTAX=" + RailpackFrontend,
		"--build-arg", "PORT=" + port,
		"--secret", "id=PORT,env=PORT",
		"-f", planPath,
		"-t", req.ImageTag,
	}
	if len(buildVars) > 0 {
		args = append(args, "--build-arg", "secrets-hash="+SecretsHash(buildVars))
	}
	extraEnv := []string{"PORT=" + port}
	for _, v := range buildVars {
		args = append(args, "--secret", "id="+v.Key+",env="+v.Key)
		extraEnv = append(extraEnv, v.Key+"="+v.Value)
	}
	args = append(args, contextDir)
	logf("📦 Railpack plan ready; building %s\n", req.ImageTag)
	return runBuild(ctx, run, execx.Cmd{Name: dockerBin, Args: args, Dir: contextDir, ExtraEnv: extraEnv}, logw)
}

// CheckProjectSymlinks rejects a project checkout holding a symlink that
// resolves outside the project directory. The whole checkout is walked, not
// only the build context: a context link to another project directory
// reaches that directory's links too. A link that resolves is judged by its
// real path; a dangling link is allowed only with a relative target that
// never climbs (".."), so it cannot point outside once its target appears.
func CheckProjectSymlinks(projectPath string) error {
	root, err := realPath(projectPath)
	if err != nil {
		return err
	}
	inside := func(p string) bool {
		return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
	}
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(p)
		if err != nil {
			return err
		}
		ok := false
		if real, err := filepath.EvalSymlinks(p); err == nil {
			ok = inside(real)
		} else {
			// Dangling: only a relative target that never climbs ("..")
			// stays inside; links it passes through are checked themselves.
			ok = !filepath.IsAbs(target) && filepath.VolumeName(target) == "" && !slices.Contains(strings.FieldsFunc(target, isPathSep), "..")
		}
		if !ok {
			rel, _ := filepath.Rel(root, p)
			return fmt.Errorf("The project contains a symlink that points outside the project directory: %s -> %s. Remove it or replace it with a regular file, then redeploy.", filepath.ToSlash(rel), target)
		}
		return nil
	})
}

func isPathSep(r rune) bool { return r == '/' || r == filepath.Separator }

// SecretsHash is the sha256 (hex) of the sorted "KEY=VALUE" lines of the
// build variables; Railpack uses it to invalidate cached layers when a
// secret changes without exposing the value.
func SecretsHash(vars []EnvVar) string {
	lines := make([]string, len(vars))
	for i, v := range vars {
		lines[i] = v.Key + "=" + v.Value
	}
	slices.SortFunc(lines, compareUTF16)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// compareUTF16 orders strings like Array.prototype.sort's default
// comparison (UTF-16 code units).
func compareUTF16(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

// runBuild streams a build command to logw, keeping the last 128 KiB to
// summarize a failure.
func runBuild(ctx context.Context, run CommandRunner, c execx.Cmd, logw io.Writer) error {
	tail := &execx.TailBuffer{Max: buildTailMax}
	out := io.MultiWriter(tail, logw)
	shared := &serialWriter{w: out}
	c.Stdout, c.Stderr = shared, shared
	res, err := run(ctx, c)
	if err == nil {
		return nil
	}
	var ee *execx.ExitError
	if errors.As(err, &ee) {
		return errors.New(SummarizeBuildFailure(c.Name, tail.String(), res.ExitCode))
	}
	return err
}

// serialWriter is one comparable writer for stdout and stderr, so exec
// serializes the writes of both streams.
type serialWriter struct{ w io.Writer }

func (s *serialWriter) Write(p []byte) (int, error) { return s.w.Write(p) }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func absOr(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
