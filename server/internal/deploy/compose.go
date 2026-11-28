package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"docklift/internal/js"
)

// ScannedService is a Dockerfile found by ScanDockerfiles. Paths are
// relative to the scanned directory with OS separators, like path.relative.
type ScannedService struct {
	Name           string
	DockerfilePath string
	ContextPath    string
	InternalPort   int
}

// scanIgnoreDirs are never descended into when scanning for Dockerfiles.
var scanIgnoreDirs = map[string]bool{
	"node_modules": true, ".git": true, ".github": true, "vendor": true, "__pycache__": true,
	".venv": true, "venv": true, ".next": true, "dist": true, "build": true, ".cache": true,
}

var exposeRe = regexp.MustCompile(`(?i)EXPOSE` + jsSpace + `+(\d+)`)

// jsSpace is JavaScript's \s character class (Go's \s is ASCII only).
const jsSpace = `[\t\n\v\f\r \x{00a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]`

// detectPortFromDockerfile reads the first EXPOSE port, else guesses from
// framework names, else 3000.
func detectPortFromDockerfile(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 3000
	}
	content := string(b)
	if m := exposeRe.FindStringSubmatch(content); m != nil {
		f, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return int(f)
		}
	}
	lower := js.Lower(content)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(lower, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("next", "react"):
		return 3000
	case has("uvicorn", "fastapi"):
		return 8000
	case has("flask"):
		return 5000
	case has("django"):
		return 8000
	}
	return 3000
}

// ScanDockerfiles finds files named exactly "Dockerfile" up to maxDepth
// directories below projectPath (2 in the 2.x default), skipping symlinks and
// scanIgnoreDirs, and names each service after its directory ("app" at the
// root, "a/b" -> "a-b"). Colliding names are deduplicated with
// DedupeScannedServices. Directory entries are visited in byte order, as
// readdir returns them.
func ScanDockerfiles(projectPath string, maxDepth int) ([]ScannedService, error) {
	var services []ScannedService
	var scan func(dir string, depth int)
	scan = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			itemPath := filepath.Join(dir, e.Name())
			info, err := os.Lstat(itemPath)
			if err != nil {
				return // 2.x abandoned the rest of the directory
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if info.IsDir() {
				if !scanIgnoreDirs[e.Name()] {
					scan(itemPath, depth+1)
				}
				continue
			}
			if e.Name() != "Dockerfile" {
				continue
			}
			rel, err1 := filepath.Rel(projectPath, itemPath)
			parent, err2 := filepath.Rel(projectPath, dir)
			if err1 != nil || err2 != nil {
				return
			}
			if parent == "." {
				parent = ""
			}
			name := "app"
			if parent != "" {
				name = strings.NewReplacer("/", "-", "\\", "-").Replace(parent)
			}
			ctxPath := parent
			if ctxPath == "" {
				ctxPath = "."
			}
			services = append(services, ScannedService{
				Name: name, DockerfilePath: rel, ContextPath: ctxPath, InternalPort: detectPortFromDockerfile(itemPath),
			})
		}
	}
	scan(projectPath, 0)
	for _, svc := range services {
		if err := CheckServiceName(svc.Name, filepath.ToSlash(svc.DockerfilePath)); err != nil {
			return nil, err
		}
	}
	return DedupeScannedServices(services)
}

// DedupeScannedServices renames a service whose name is already taken to
// <name>-<ShortPathHash(dockerfile path)> and fails when the name is still
// taken after that.
func DedupeScannedServices(services []ScannedService) ([]ScannedService, error) {
	used := map[string]bool{}
	out := make([]ScannedService, 0, len(services))
	for _, svc := range services {
		name := svc.Name
		if used[name] {
			name = svc.Name + "-" + ShortPathHash(svc.DockerfilePath)
		}
		if used[name] {
			return nil, fmt.Errorf("Duplicate service name %q after scan deduplication — adjust Dockerfile layout or paths.", name)
		}
		used[name] = true
		svc.Name = name
		out = append(out, svc)
	}
	return out, nil
}

// RuntimeVolume is a named volume mounted into a runtime service.
type RuntimeVolume struct {
	// Key is the compose volume key (StorageVolumeComposeKey).
	Key string
	// Name is the external Docker volume name.
	Name      string
	MountPath string
}

// RuntimeService is one service of the generated runtime compose file.
type RuntimeService struct {
	Name         string
	Image        string
	InternalPort int
	// Port is the host port, published only with PublishHostPort (nil = none).
	Port          *int
	ContainerName string
	Volumes       []RuntimeVolume
	// Command overrides the image command (e.g. redis-server --requirepass).
	Command []string
	// NoPortEnv suppresses the injected PORT=<internal port> (managed DBs).
	NoPortEnv bool
}

// RuntimeComposeOptions are the project-level compose settings.
type RuntimeComposeOptions struct {
	// ProjectID labels everything and names the project network ("unknown"
	// when empty, like the 2.x default).
	ProjectID       string
	PublishHostPort bool
	// MemLimit sets mem_limit when non-empty; CPUs sets cpus when non-nil.
	MemLimit string
	CPUs     *float64
}

// RuntimeComposePath is DEPLOYMENTS_PATH/.docklift/<projectId>/compose.yml.
func RuntimeComposePath(deploymentsPath, projectID string) string {
	return filepath.Join(RuntimeStatePath(deploymentsPath, projectID), "compose.yml")
}

// RuntimeStatePath is DEPLOYMENTS_PATH/.docklift/<projectId>, where Docklift
// keeps per-project runtime state outside the source checkout.
func RuntimeStatePath(deploymentsPath, projectID string) string {
	return filepath.Join(deploymentsPath, ".docklift", projectID)
}

// WriteRuntimeCompose renders the runtime compose file and writes it
// atomically to composePath (creating directories). Repository Dockerfiles
// and docker-compose.yml files are never touched (generateRuntimeCompose).
// The file holds env values, so it is written mode 0600.
func WriteRuntimeCompose(composePath string, services []RuntimeService, envVars []EnvVar, opts RuntimeComposeOptions) error {
	data, err := RenderRuntimeCompose(services, envVars, opts)
	if err != nil {
		return err
	}
	dir := filepath.Dir(composePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create runtime state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".compose-*.yml")
	if err != nil {
		return fmt.Errorf("write runtime compose: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write runtime compose: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write runtime compose: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("write runtime compose: %w", err)
	}
	if err := os.Rename(tmp.Name(), composePath); err != nil {
		return fmt.Errorf("write runtime compose: %w", err)
	}
	return nil
}

// RenderRuntimeCompose builds the runtime compose document. It is
// semantically identical to the js-yaml dump of 2.x: same keys and
// values, every string a YAML string. Dynamic strings (env values, names,
// images, ports) are always double-quoted so no YAML 1.1 or 1.2 parser
// reads them as numbers, booleans, null, dates or sexagesimals.
func RenderRuntimeCompose(services []RuntimeService, envVars []EnvVar, opts RuntimeComposeOptions) ([]byte, error) {
	projectID := opts.ProjectID
	if projectID == "" {
		projectID = "unknown"
	}
	svcMap := mapping()
	topVolumes := mapping()
	volumeIndex := map[string]*yaml.Node{}
	for _, svc := range services {
		// Service rows predating the scan check (rollback, redeploy of stored
		// services) must not become a reserved DNS alias either.
		if err := CheckServiceName(svc.Name, ""); err != nil {
			return nil, err
		}
		env := mapping()
		envIndex := map[string]int{}
		setEnv := func(k, v string) {
			if i, ok := envIndex[k]; ok {
				env.Content[i+1] = quoted(v)
				return
			}
			envIndex[k] = len(env.Content)
			env.Content = append(env.Content, quoted(k), quoted(v))
		}
		for _, item := range EnvForService(envVars, svc.Name) {
			if item.IsRuntime {
				setEnv(item.Key, item.Value)
			}
		}
		if !svc.NoPortEnv {
			setEnv("PORT", itoa(svc.InternalPort))
		}
		def := mapping(
			plain("image"), quoted(svc.Image),
			plain("container_name"), quoted(svc.ContainerName),
			plain("restart"), quoted("unless-stopped"),
			plain("networks"), sequence(quoted("project")),
			plain("labels"), mapping(
				quoted("com.docklift.managed"), quoted("true"),
				quoted("com.docklift.project"), quoted(projectID),
				quoted("com.docklift.service"), quoted(svc.Name),
			),
			plain("environment"), env,
			plain("security_opt"), sequence(quoted("no-new-privileges:true")),
		)
		if opts.MemLimit != "" {
			def.Content = append(def.Content, plain("mem_limit"), quoted(opts.MemLimit))
		}
		if opts.CPUs != nil {
			def.Content = append(def.Content, plain("cpus"), quoted(js.NumberString(*opts.CPUs)))
		}
		if len(svc.Command) > 0 {
			cmd := sequence()
			for _, c := range svc.Command {
				cmd.Content = append(cmd.Content, quoted(c))
			}
			def.Content = append(def.Content, plain("command"), cmd)
		}
		if opts.PublishHostPort && svc.Port != nil {
			def.Content = append(def.Content, plain("ports"), sequence(quoted(itoa(*svc.Port)+":"+itoa(svc.InternalPort))))
		}
		if len(svc.Volumes) > 0 {
			vols := sequence()
			for _, v := range svc.Volumes {
				entry := mapping(plain("name"), quoted(v.Name), plain("external"), boolean(true))
				if existing, ok := volumeIndex[v.Key]; ok {
					*existing = *entry
				} else {
					topVolumes.Content = append(topVolumes.Content, quoted(v.Key), entry)
					volumeIndex[v.Key] = entry
				}
				vols.Content = append(vols.Content, quoted(v.Key+":"+v.MountPath))
			}
			def.Content = append(def.Content, plain("volumes"), vols)
		}
		setMapping(svcMap, svc.Name, def)
	}
	doc := mapping(
		plain("services"), svcMap,
		plain("networks"), mapping(plain("project"), mapping(
			plain("name"), quoted(ProjectNetworkName(projectID)),
			plain("labels"), mapping(
				quoted("com.docklift.managed"), quoted("true"),
				quoted("com.docklift.project"), quoted(projectID),
			),
		)),
	)
	if len(topVolumes.Content) > 0 {
		doc.Content = append(doc.Content, plain("volumes"), topVolumes)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{doc}}); err != nil {
		return nil, fmt.Errorf("encode runtime compose: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode runtime compose: %w", err)
	}
	return buf.Bytes(), nil
}

// setMapping sets key in a mapping node, replacing an existing entry in place
// (a later service with the same name overwrote the earlier one in 2.x).
func setMapping(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, quoted(key), value)
}

func mapping(kv ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: kv}
}

func sequence(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

// plain is a fixed structural key (never ambiguous).
func plain(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// quoted is a double-quoted string scalar. Invalid UTF-8 becomes U+FFFD per
// byte, as a JavaScript string read from SQLite would hold it.
func quoted(s string) *yaml.Node {
	if !utf8.ValidString(s) {
		var b strings.Builder
		for _, r := range s {
			b.WriteRune(r)
		}
		s = b.String()
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle}
}

func boolean(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
}

// argRe matches ARG instructions at the start of a line (JS \s semantics).
var argRe = regexp.MustCompile(`(?m)^` + jsSpace + `*ARG` + jsSpace + `+([a-zA-Z0-9_]+)`)

// jsLineBreaks are the extra line terminators JavaScript's multiline ^
// honours besides \n.
var jsLineBreaks = strings.NewReplacer("\r", "\n", string(rune(0x2028)), "\n", string(rune(0x2029)), "\n")

// ValidateDockerBuildArgs returns the build args that the Dockerfile does
// not declare with an ARG instruction (validateDockerBuildArgs). A missing or
// unreadable Dockerfile yields none.
func ValidateDockerBuildArgs(dockerfilePath string, buildArgs []string) []string {
	b, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return nil
	}
	declared := map[string]bool{}
	for _, m := range argRe.FindAllStringSubmatch(jsLineBreaks.Replace(string(b)), -1) {
		declared[m[1]] = true
	}
	var missing []string
	for _, a := range buildArgs {
		if !declared[a] {
			missing = append(missing, a)
		}
	}
	return missing
}

var secretTypeRe = regexp.MustCompile(`(?i)type` + jsSpace + `*=` + jsSpace + `*secret`)

// DockerfileMountsSecret reports whether a line of the Dockerfile mounts the
// BuildKit secret secretID (RUN --mount=type=secret,id=<id>), matching
// case-insensitively like the 2.x check. Unlike the 2.x regex the id must end
// at a delimiter (NPM_TOKEN does not match id=NPM_TOKEN_2), "id" must be its
// own option (not uid=...), and type/id may come in either order.
func DockerfileMountsSecret(dockerfilePath, secretID string) bool {
	b, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return false
	}
	idRe, err := regexp.Compile(`(?i)(?:^|[^A-Za-z0-9_])id` + jsSpace + `*=` + jsSpace + `*` + regexp.QuoteMeta(secretID) + `(?:$|[,"'\]\t\v\f\r \x{00a0}\x{feff}])`)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if secretTypeRe.MatchString(line) && idRe.MatchString(line) {
			return true
		}
	}
	return false
}
