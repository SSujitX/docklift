// Package deploy is Docklift's deployment service layer: Docker naming
// rules, build resolution (Dockerfile vs Railpack) and image builds, runtime
// compose generation, per-project deploy locks and cancellation, boot
// recovery, project status sync, keep-2 image retention, host port
// allocation, env variable scoping, and managed databases (engines,
// connection URLs, link env sync, Docker Hub tags).
//
// It is the port of backend/src/services/{compose,buildResolver,buildRunner}.ts
// and backend/src/lib/{naming,deploymentState,deploymentRecovery,
// projectStatusSync,imageCleanup,portAllocation,envVariables,databaseEngines,
// databaseLinks,dockerHubTags}.ts. Names, tags and compose output match the
// TypeScript rules byte for byte, because existing installs have Docker
// objects named by them.
package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

const (
	slugMax    = 24
	serviceMax = 20
)

var (
	nonSlugRe   = regexp.MustCompile(`[^a-z0-9]+`)
	edgeDashRe  = regexp.MustCompile(`^-+|-+$`)
	trailDashRe = regexp.MustCompile(`-+$`)
)

// DockerSlug sanitizes a project or service label for Docker names
// (dockerSlug): lowercase, runs of other characters become "-", no leading
// or trailing "-", at most maxLen characters; "app" when nothing is left.
func DockerSlug(input string, maxLen int) string {
	if input == "" {
		input = "app"
	}
	slug := nonSlugRe.ReplaceAllString(jsLower(input), "-")
	slug = edgeDashRe.ReplaceAllString(slug, "")
	if len(slug) > maxLen {
		slug = slug[:maxLen] // ASCII only at this point
	}
	slug = trailDashRe.ReplaceAllString(slug, "")
	if slug == "" {
		return "app"
	}
	return slug
}

// Slug is DockerSlug with the default 24-character limit.
func Slug(input string) string { return DockerSlug(input, slugMax) }

// ShortProjectID is the project id without dashes, first 8 characters.
func ShortProjectID(projectID string) string {
	return jsSlice(strings.ReplaceAll(projectID, "-", ""), 8)
}

// ComposeProjectName is the `docker compose -p` name: dl-<slug>-<id8>.
func ComposeProjectName(projectName, projectID string) string {
	return "dl-" + Slug(projectName) + "-" + ShortProjectID(projectID)
}

// ProjectNetworkName is the isolated bridge network of a project:
// dl-net-<id8>.
func ProjectNetworkName(projectID string) string {
	return "dl-net-" + ShortProjectID(projectID)
}

// ServiceContainerName is the explicit container_name of a service:
// dl_<slug>_<id8>_<service slug with "_">, at most 63 characters.
func ServiceContainerName(projectName, projectID, serviceName string) string {
	svc := strings.ReplaceAll(DockerSlug(serviceName, serviceMax), "-", "_")
	return jsSlice("dl_"+Slug(projectName)+"_"+ShortProjectID(projectID)+"_"+svc, 63)
}

// ComposeProjectAliases are every compose project name this project may
// have used: the current one and the legacy bare UUID (deduplicated).
func ComposeProjectAliases(projectName, projectID string) []string {
	current := ComposeProjectName(projectName, projectID)
	if current == projectID {
		return []string{current}
	}
	return []string{current, projectID}
}

// ShortPathHash is the first 4 hex characters of sha256(path), a stable
// suffix used to dedupe names.
func ShortPathHash(relativePath string) string {
	sum := sha256.Sum256([]byte(relativePath))
	return hex.EncodeToString(sum[:])[:4]
}

// StorageVolumeComposeKey is the compose top-level volume key of a service
// mount: storage_<service slug>_<index>_<hash of the Docker volume name>.
func StorageVolumeComposeKey(serviceName string, mountIndex int, volumeDockerName string) string {
	return "storage_" + DockerSlug(serviceName, serviceMax) + "_" + itoa(mountIndex) + "_" + ShortPathHash(volumeDockerName)
}

// PersistentVolumeName is the Docker volume name of a persistent storage
// entry: dl-<id8>-<label slug, 28>-<hash of the label>. Hashing the original
// label keeps "a-b" and "a_b" apart (routes/projects.ts, routes/databases.ts).
func PersistentVolumeName(projectID, label string) string {
	return "dl-" + ShortProjectID(projectID) + "-" + DockerSlug(label, 28) + "-" + ShortPathHash(label)
}

// ManagedDatabaseVolumeLabel marks the data volume of a managed database.
const ManagedDatabaseVolumeLabel = "com.docklift.role=managed-database"

// VolumeLabels returns the labels of a Docklift-created Docker volume, in the
// order the TS routes passed them: the compose project label,
// com.docklift.project, then extra (e.g. ManagedDatabaseVolumeLabel).
func VolumeLabels(projectName, projectID string, extra ...string) []string {
	return append([]string{
		"com.docker.compose.project=" + ComposeProjectName(projectName, projectID),
		"com.docklift.project=" + projectID,
	}, extra...)
}

func itoa(n int) string { return jsNumberString(float64(n)) }
