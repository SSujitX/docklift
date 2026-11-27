package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Builders and build types.
const (
	BuildAuto       = "auto"
	BuildDockerfile = "dockerfile"
	BuildRailpack   = "railpack"
)

// BuildSettings are the project columns that drive build resolution.
type BuildSettings struct {
	BuildType      string
	BaseDirectory  string
	DockerfilePath *string
	InternalPort   *int
}

// ResolvedBuildService is one image to build.
type ResolvedBuildService struct {
	Name    string `json:"name"`
	Builder string `json:"builder"`
	// ContextPath and DockerfilePath are relative to the project root with
	// forward slashes ("." for the root); DockerfilePath is nil for Railpack.
	ContextPath    string  `json:"contextPath"`
	DockerfilePath *string `json:"dockerfilePath"`
	InternalPort   int     `json:"internalPort"`
}

// ResolvedBuild is the outcome of ResolveProjectBuild (GET
// /api/projects/:id/build/detect returns its first six fields).
type ResolvedBuild struct {
	RequestedType  string                 `json:"requestedType"`
	ResolvedType   string                 `json:"resolvedType"`
	BaseDirectory  string                 `json:"baseDirectory"`
	DockerfilePath *string                `json:"dockerfilePath"`
	Detected       string                 `json:"detected"`
	Manifests      []string               `json:"manifests"`
	Services       []ResolvedBuildService `json:"services"`
}

// buildManifests are the files whose presence is reported for Railpack.
var buildManifests = []string{
	"package.json", "bun.lock", "bun.lockb", "pnpm-lock.yaml", "yarn.lock", "package-lock.json",
	"requirements.txt", "pyproject.toml", "Pipfile", "poetry.lock", "go.mod", "Cargo.toml",
	"composer.json", "Gemfile", "pom.xml", "build.gradle", "build.gradle.kts",
}

// NormalizeBuildType maps anything but "dockerfile" and "railpack" to "auto".
func NormalizeBuildType(value any) string {
	if s, ok := value.(string); ok && (s == BuildDockerfile || s == BuildRailpack) {
		return s
	}
	return BuildAuto
}

// ErrPathEscape is the error of a build path that leaves the project.
var ErrPathEscape = errors.New("Build paths must stay inside the project directory")

// ResolveProjectPath resolves a user-controlled relative path inside
// projectPath; absolute paths and ".." segments are rejected.
func ResolveProjectPath(projectPath, relativeValue string) (string, error) {
	rel := strings.ReplaceAll(jsTrim(relativeValue), "\\", "/")
	if relativeValue == "" {
		rel = "."
	}
	root, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	if rel == "" || rel == "." {
		return root, nil
	}
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || slices.Contains(strings.Split(rel, "/"), "..") {
		return "", ErrPathEscape
	}
	resolved := filepath.Join(root, filepath.FromSlash(rel))
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", ErrPathEscape
	}
	return resolved, nil
}

func relativeFromProject(projectPath, abs string) string {
	root, err := filepath.Abs(projectPath)
	if err != nil {
		return "."
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

// assertRealPathInside rejects targets whose real path (symlinks resolved)
// leaves the project.
func assertRealPathInside(projectPath, target string) error {
	root, err := realPath(projectPath)
	if err != nil {
		return err
	}
	real, err := realPath(target)
	if err != nil {
		return err
	}
	if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
		return errors.New("Build paths and symlinks must stay inside the project directory")
	}
	return nil
}

func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// DetectManifests lists the build manifests present in basePath.
func DetectManifests(basePath string) []string {
	found := []string{}
	for _, f := range buildManifests {
		if _, err := os.Stat(filepath.Join(basePath, f)); err == nil {
			found = append(found, f)
		}
	}
	return found
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// ResolveProjectBuild decides how to build a project (resolveProjectBuild):
// a configured Dockerfile; for "auto" every repository Dockerfile under the
// base directory; otherwise (or when auto finds none) one Railpack service.
func ResolveProjectBuild(projectPath string, s BuildSettings) (*ResolvedBuild, error) {
	requested := NormalizeBuildType(s.BuildType)
	basePath, err := ResolveProjectPath(projectPath, s.BaseDirectory)
	if err != nil {
		return nil, err
	}
	if !isDir(basePath) {
		shown := s.BaseDirectory
		if shown == "" {
			shown = "."
		}
		return nil, fmt.Errorf("Base directory not found: %s", shown)
	}
	if err := assertRealPathInside(projectPath, basePath); err != nil {
		return nil, err
	}
	baseDirectory := relativeFromProject(projectPath, basePath)
	internalPort := 3000
	if s.InternalPort != nil && *s.InternalPort > 0 && *s.InternalPort <= 65535 {
		internalPort = *s.InternalPort
	}
	manifests := DetectManifests(basePath)

	if requested == BuildDockerfile {
		configured := "Dockerfile"
		if s.DockerfilePath != nil && *s.DockerfilePath != "" {
			configured = *s.DockerfilePath
		}
		dockerfile, err := ResolveProjectPath(basePath, jsTrim(configured))
		if err != nil {
			return nil, err
		}
		rel := relativeFromProject(projectPath, dockerfile)
		if !isFile(dockerfile) {
			return nil, fmt.Errorf("Dockerfile not found: %s", rel)
		}
		if err := assertRealPathInside(projectPath, dockerfile); err != nil {
			return nil, err
		}
		return &ResolvedBuild{
			RequestedType: requested, ResolvedType: BuildDockerfile, BaseDirectory: baseDirectory,
			DockerfilePath: &rel, Detected: "Configured Dockerfile", Manifests: manifests,
			Services: []ResolvedBuildService{{Name: "app", Builder: BuildDockerfile, ContextPath: baseDirectory, DockerfilePath: &rel, InternalPort: internalPort}},
		}, nil
	}

	if requested == BuildAuto {
		dockerfiles, err := ScanDockerfiles(basePath, 2)
		if err != nil {
			return nil, err
		}
		if len(dockerfiles) > 0 {
			services := make([]ResolvedBuildService, 0, len(dockerfiles))
			for _, item := range dockerfiles {
				df := relativeFromProject(projectPath, filepath.Join(basePath, item.DockerfilePath))
				port := item.InternalPort
				if port == 0 {
					port = internalPort
				}
				services = append(services, ResolvedBuildService{
					Name: item.Name, Builder: BuildDockerfile,
					ContextPath:    relativeFromProject(projectPath, filepath.Join(basePath, item.ContextPath)),
					DockerfilePath: &df, InternalPort: port,
				})
			}
			plural := "s"
			if len(services) == 1 {
				plural = ""
			}
			first := *services[0].DockerfilePath
			return &ResolvedBuild{
				RequestedType: requested, ResolvedType: BuildDockerfile, BaseDirectory: baseDirectory,
				DockerfilePath: &first, Detected: fmt.Sprintf("%d repository Dockerfile%s", len(services), plural),
				Manifests: manifests, Services: services,
			}, nil
		}
	}

	detected := "Railpack source (framework detection runs during build)"
	if len(manifests) > 0 {
		detected = "Railpack source (" + strings.Join(manifests, ", ") + ")"
	}
	return &ResolvedBuild{
		RequestedType: requested, ResolvedType: BuildRailpack, BaseDirectory: baseDirectory,
		Detected: detected, Manifests: manifests,
		Services: []ResolvedBuildService{{Name: "app", Builder: BuildRailpack, ContextPath: baseDirectory, InternalPort: internalPort}},
	}, nil
}
