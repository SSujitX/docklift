package deploy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"docklift/internal/js"
)

// Managed database engine ids.
const (
	EnginePostgres = "postgres"
	EngineMySQL    = "mysql"
	EngineMariaDB  = "mariadb"
	EngineRedis    = "redis"
	EngineMongoDB  = "mongodb"
)

// EngineIDs lists the engines in catalog order.
var EngineIDs = []string{EnginePostgres, EngineMySQL, EngineMariaDB, EngineRedis, EngineMongoDB}

// EngineVersion is a selectable image tag; Recommended marks the default.
type EngineVersion struct {
	Tag         string `json:"tag"`
	Label       string `json:"label"`
	Recommended bool   `json:"recommended,omitempty"`
}

// Engine describes a managed database engine. The JSON form is the engine
// object the 2.x API returned (keys in the same order).
type Engine struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// ImageRepo is the Docker Hub repository without tag.
	ImageRepo string `json:"imageRepo"`
	// Versions are the fallback tags when Docker Hub is unreachable.
	Versions []EngineVersion `json:"versions"`
	// Port is the container listen port.
	Port int `json:"port"`
	// VolumeMount is the data volume path (Postgres <= 17; see
	// VolumeMountForEngine).
	VolumeMount string `json:"volumeMount"`
	// DefaultEnvKey is injected into linked apps.
	DefaultEnvKey string `json:"defaultEnvKey"`
	ServiceName   string `json:"serviceName"`
	// Image is repo:tag of the recommended version.
	Image string `json:"image"`
}

// Credentials of a managed database.
type Credentials struct {
	Username string
	Password string
	Database string
}

// LegacyManagedImages are the images of bare `[managed:<engine>]` markers
// created before version pinning; they must stay on the majors that shipped
// with the marker, never the current recommendation.
var LegacyManagedImages = map[string]string{
	EnginePostgres: "postgres:16-alpine",
	EngineMySQL:    "mysql:8.4",
	EngineMariaDB:  "mariadb:11",
	EngineRedis:    "redis:7-alpine",
	EngineMongoDB:  "mongo:7",
}

func newEngine(e Engine) Engine {
	rec := e.Versions[0]
	for _, v := range e.Versions {
		if v.Recommended {
			rec = v
			break
		}
	}
	e.Image = e.ImageRepo + ":" + rec.Tag
	return e
}

var engines = map[string]Engine{
	EnginePostgres: newEngine(Engine{
		ID: EnginePostgres, Label: "PostgreSQL", Description: "Relational database — Prisma, Rails, Django, and most apps.",
		ImageRepo: "postgres",
		Versions: []EngineVersion{
			{Tag: "18-alpine", Label: "18 · Alpine", Recommended: true},
			{Tag: "17-alpine", Label: "17 · Alpine"},
			{Tag: "16-alpine", Label: "16 · Alpine"},
			{Tag: "15-alpine", Label: "15 · Alpine"},
			{Tag: "14-alpine", Label: "14 · Alpine"},
			{Tag: "18", Label: "18"},
			{Tag: "17", Label: "17"},
			{Tag: "16", Label: "16"},
		},
		Port: 5432, VolumeMount: "/var/lib/postgresql/data", DefaultEnvKey: "DATABASE_URL", ServiceName: "db",
	}),
	EngineMySQL: newEngine(Engine{
		ID: EngineMySQL, Label: "MySQL", Description: "Popular relational database for LAMP and many frameworks.",
		ImageRepo: "mysql",
		Versions: []EngineVersion{
			{Tag: "8.4", Label: "8.4 LTS", Recommended: true},
			{Tag: "8.0", Label: "8.0"},
			{Tag: "8.4-oracle", Label: "8.4 · Oracle Linux"},
			{Tag: "latest", Label: "latest"},
		},
		Port: 3306, VolumeMount: "/var/lib/mysql", DefaultEnvKey: "DATABASE_URL", ServiceName: "db",
	}),
	EngineMariaDB: newEngine(Engine{
		ID: EngineMariaDB, Label: "MariaDB", Description: "MySQL-compatible relational database.",
		ImageRepo: "mariadb",
		Versions: []EngineVersion{
			{Tag: "11", Label: "11", Recommended: true},
			{Tag: "11.4", Label: "11.4 LTS"},
			{Tag: "10.11", Label: "10.11 LTS"},
			{Tag: "latest", Label: "latest"},
		},
		Port: 3306, VolumeMount: "/var/lib/mysql", DefaultEnvKey: "DATABASE_URL", ServiceName: "db",
	}),
	EngineRedis: newEngine(Engine{
		ID: EngineRedis, Label: "Redis", Description: "In-memory cache, queues, and sessions.",
		ImageRepo: "redis",
		Versions: []EngineVersion{
			{Tag: "8-alpine", Label: "8 · Alpine", Recommended: true},
			{Tag: "7-alpine", Label: "7 · Alpine"},
			{Tag: "7", Label: "7"},
			{Tag: "6-alpine", Label: "6 · Alpine"},
			{Tag: "latest", Label: "latest"},
		},
		Port: 6379, VolumeMount: "/data", DefaultEnvKey: "REDIS_URL", ServiceName: "db",
	}),
	EngineMongoDB: newEngine(Engine{
		ID: EngineMongoDB, Label: "MongoDB", Description: "Document database for flexible JSON-like data.",
		ImageRepo: "mongo",
		Versions: []EngineVersion{
			{Tag: "8", Label: "8", Recommended: true},
			{Tag: "7", Label: "7"},
			{Tag: "6", Label: "6"},
			{Tag: "latest", Label: "latest"},
		},
		Port: 27017, VolumeMount: "/data/db", DefaultEnvKey: "MONGODB_URI", ServiceName: "db",
	}),
}

// GetEngine returns a copy of the engine definition.
func GetEngine(id string) (Engine, bool) {
	e, ok := engines[id]
	if !ok {
		return Engine{}, false
	}
	e.Versions = slices.Clone(e.Versions)
	return e, true
}

// ListEngines returns every engine (static versions) in catalog order.
func ListEngines() []Engine {
	out := make([]Engine, 0, len(EngineIDs))
	for _, id := range EngineIDs {
		e, _ := GetEngine(id)
		out = append(out, e)
	}
	return out
}

var leadingDigitsRe = regexp.MustCompile(`^(\d+)`)

func tagOf(imageOrTag string) string {
	if i := strings.LastIndexByte(imageOrTag, ':'); i >= 0 {
		return imageOrTag[i+1:]
	}
	return imageOrTag
}

// MajorFromImageTag parses the leading major of a tag or image
// ("18-alpine" -> 18; "latest" or no digits -> ok=false).
func MajorFromImageTag(imageOrTag string) (int, bool) {
	tag := tagOf(imageOrTag)
	if tag == "" || tag == "latest" {
		return 0, false
	}
	m := leadingDigitsRe.FindStringSubmatch(tag)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

// VolumeMountForEngine is the data volume path for the selected image.
// Postgres 18+ (and "latest") mount /var/lib/postgresql, as the official
// image declares; older majors keep /var/lib/postgresql/data. imageOrTag ""
// means the engine's recommended image.
func VolumeMountForEngine(e Engine, imageOrTag string) string {
	if e.ID != EnginePostgres {
		return e.VolumeMount
	}
	ref := imageOrTag
	if ref == "" {
		ref = e.Image
	}
	tag := tagOf(ref)
	if tag == "latest" {
		return "/var/lib/postgresql"
	}
	if major, ok := MajorFromImageTag(tag); ok && major >= 18 {
		return "/var/lib/postgresql"
	}
	return "/var/lib/postgresql/data"
}

// GenerateDatabasePassword returns 24 random bytes, base64url without padding.
func GenerateDatabasePassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate database password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DefaultDatabaseCredentials are the credentials of a new managed database:
// user "docklift" and a database named after the project (slug of 16 with
// "_" instead of "-").
func DefaultDatabaseCredentials(projectName, password string) Credentials {
	base := strings.ReplaceAll(DockerSlug(projectName, 16), "-", "_")
	if base == "" {
		base = "app"
	}
	return Credentials{Username: "docklift", Password: password, Database: base}
}

// EnvPair is one ordered environment entry.
type EnvPair struct{ Key, Value string }

// EngineRuntimeEnv is the environment of the database container itself.
func EngineRuntimeEnv(e Engine, c Credentials) []EnvPair {
	switch e.ID {
	case EnginePostgres:
		return []EnvPair{{"POSTGRES_USER", c.Username}, {"POSTGRES_PASSWORD", c.Password}, {"POSTGRES_DB", c.Database}}
	case EngineMySQL, EngineMariaDB:
		return []EnvPair{{"MYSQL_USER", c.Username}, {"MYSQL_PASSWORD", c.Password}, {"MYSQL_DATABASE", c.Database}, {"MYSQL_ROOT_PASSWORD", c.Password}}
	case EngineRedis:
		return []EnvPair{{"REDIS_PASSWORD", c.Password}}
	case EngineMongoDB:
		return []EnvPair{{"MONGO_INITDB_ROOT_USERNAME", c.Username}, {"MONGO_INITDB_ROOT_PASSWORD", c.Password}, {"MONGO_INITDB_DATABASE", c.Database}}
	}
	return nil
}

// BuildConnectionURL is the URL apps on a shared Docker network use; host
// is the database container name (Docker DNS).
func BuildConnectionURL(e Engine, host string, c Credentials) (string, error) {
	user, pass, dbName := js.EncodeURIComponent(c.Username), js.EncodeURIComponent(c.Password), js.EncodeURIComponent(c.Database)
	port := strconv.Itoa(e.Port)
	switch e.ID {
	case EnginePostgres:
		return "postgresql://" + user + ":" + pass + "@" + host + ":" + port + "/" + dbName, nil
	case EngineMySQL, EngineMariaDB:
		return "mysql://" + user + ":" + pass + "@" + host + ":" + port + "/" + dbName, nil
	case EngineRedis:
		return "redis://:" + pass + "@" + host + ":" + port, nil
	case EngineMongoDB:
		return "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/" + dbName + "?authSource=admin", nil
	}
	return "", fmt.Errorf("Unsupported engine: %s", e.ID)
}

// EngineCommand is the compose command override (Redis requirepass), or nil.
func EngineCommand(e Engine, c Credentials) []string {
	if e.ID == EngineRedis {
		return []string{"redis-server", "--requirepass", c.Password, "--appendonly", "yes"}
	}
	return nil
}

// CredentialsFromEnv reads the credentials back from the database project's
// env (credentialsFromEnvMap); ok=false when a required value is empty.
func CredentialsFromEnv(e Engine, env map[string]string) (Credentials, bool) {
	all := func(vals ...string) bool {
		for _, v := range vals {
			if v == "" {
				return false
			}
		}
		return true
	}
	switch e.ID {
	case EnginePostgres:
		c := Credentials{env["POSTGRES_USER"], env["POSTGRES_PASSWORD"], env["POSTGRES_DB"]}
		return c, all(c.Username, c.Password, c.Database)
	case EngineMySQL, EngineMariaDB:
		c := Credentials{env["MYSQL_USER"], env["MYSQL_PASSWORD"], env["MYSQL_DATABASE"]}
		return c, all(c.Username, c.Password, c.Database)
	case EngineRedis:
		c := Credentials{Password: env["REDIS_PASSWORD"], Database: "0"}
		return c, all(c.Password)
	case EngineMongoDB:
		c := Credentials{env["MONGO_INITDB_ROOT_USERNAME"], env["MONGO_INITDB_ROOT_PASSWORD"], env["MONGO_INITDB_DATABASE"]}
		if c.Database == "" {
			c.Database = "admin"
		}
		return c, all(c.Username, c.Password)
	}
	return Credentials{}, false
}

// ManagedCredentialEnvKeys are the credential env keys that official images
// honour only on first volume init (env edits are blocked for them).
func ManagedCredentialEnvKeys(engineID string) []string {
	switch engineID {
	case EnginePostgres:
		return []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"}
	case EngineMySQL, EngineMariaDB:
		return []string{"MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MYSQL_ROOT_PASSWORD"}
	case EngineMongoDB:
		return []string{"MONGO_INITDB_ROOT_USERNAME", "MONGO_INITDB_ROOT_PASSWORD", "MONGO_INITDB_DATABASE"}
	case EngineRedis:
		return []string{"REDIS_PASSWORD"}
	}
	return []string{}
}

// ResolvedImage is a validated engine image selection.
type ResolvedImage struct {
	Image string
	Tag   string
}

// ResolveEngineImage turns an optional tag (bare or "<repo>:<tag>") into a
// full image (resolveEngineImage). Tags from allowedTags (default: the
// engine's versions) or matching the engine's Docker Hub tag pattern are
// accepted, so creation works while the live list is briefly stale. The
// error text is the API's 400 message.
func ResolveEngineImage(e Engine, tag *string, allowedTags []string) (ResolvedImage, error) {
	catalog := allowedTags
	if len(catalog) == 0 {
		for _, v := range e.Versions {
			catalog = append(catalog, v.Tag)
		}
	}
	recommended := ""
	for _, v := range e.Versions {
		if v.Recommended {
			recommended = v.Tag
			break
		}
	}
	if recommended == "" && len(catalog) > 0 {
		recommended = catalog[0]
	}
	if recommended == "" && len(e.Versions) > 0 {
		recommended = e.Versions[0].Tag
	}
	if recommended == "" {
		return ResolvedImage{}, fmt.Errorf("No versions configured for %s", e.ID)
	}
	if tag == nil || *tag == "" {
		return ResolvedImage{Image: e.ImageRepo + ":" + recommended, Tag: recommended}, nil
	}
	bare := js.Trim(*tag)
	bare = strings.TrimPrefix(bare, e.ImageRepo+":")
	if !slices.Contains(catalog, bare) && !MatchesEngineTagPattern(e.ID, bare) {
		sample := strings.Join(catalog[:min(12, len(catalog))], ", ")
		more := ""
		if len(catalog) > 12 {
			more = "…"
		}
		return ResolvedImage{}, fmt.Errorf("Invalid version for %s. Try one of: %s%s", e.Label, sample, more)
	}
	return ResolvedImage{Image: e.ImageRepo + ":" + bare, Tag: bare}, nil
}

// ManagedServiceMarker is the dockerfile_path of a managed database
// service: [managed:<engine>|<image>].
func ManagedServiceMarker(engineID, image string) string {
	return "[managed:" + engineID + "|" + image + "]"
}

var managedMarkerRe = regexp.MustCompile(`^\[managed:([^\]|]+)(?:\|([^\]]+))?\]$`)

// ParseManagedServiceMarker parses [managed:<engine>] or
// [managed:<engine>|<image>]; image is "" for the bare form.
func ParseManagedServiceMarker(dockerfilePath string) (engineID, image string, ok bool) {
	m := managedMarkerRe.FindStringSubmatch(dockerfilePath)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// ImageForManagedService prefers the image pinned on the service marker; a
// bare marker (pre-pinning) keeps its legacy image, never a newer major.
func ImageForManagedService(e Engine, dockerfilePath string) string {
	if _, image, ok := ParseManagedServiceMarker(dockerfilePath); ok && image != "" {
		return image
	}
	if img, ok := LegacyManagedImages[e.ID]; ok {
		return img
	}
	return e.Image
}

// ConnectionURLFor is BuildConnectionURL for a known engine; it never fails
// for engines from the catalog.
func ConnectionURLFor(e Engine, host string, c Credentials) string {
	u, _ := BuildConnectionURL(e, host, c)
	return u
}
