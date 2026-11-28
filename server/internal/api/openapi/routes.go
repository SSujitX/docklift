// Package openapi is the endpoint map of the Docklift HTTP API: one Go
// table (Routes) describing every route, and the OpenAPI 3.1 document built
// from it (GET /api/openapi.json). The router test walks the chi tree and
// fails when a route is missing here or listed here without existing.
//
// Request shapes are taken from the handlers (body keys, query and path
// parameters, step-up and streaming); response schemas are deliberately loose.
package openapi

import "docklift/internal/deploy"

// Auth kinds of a route.
const (
	// AuthSession is `Authorization: Bearer <session JWT or API token>`.
	AuthSession = "session"
	// AuthPublic needs no credentials.
	AuthPublic = "public"
	// AuthSSE is a 5 minute `?token=` SSE token, or a Bearer API token.
	AuthSSE = "sse"
	// AuthSetupOrSession is X-Setup-Token before the first admin exists,
	// else a session with step-up.
	AuthSetupOrSession = "setup-or-session"
	// AuthHMAC is a GitHub webhook signature.
	AuthHMAC = "hmac"
	// AuthTerminal is the single-use terminal purpose token (WebSocket).
	AuthTerminal = "terminal"
	// AuthAPIToken is a Bearer API token only (the MCP endpoint).
	AuthAPIToken = "api-token"
)

// Body content types.
const (
	BodyJSON      = "json"
	BodyMultipart = "multipart"
	// BodyJSONOrMultipart accepts either (project create).
	BodyJSONOrMultipart = "json-or-multipart"
)

// Response kinds.
const (
	// StreamText is a chunked text/plain progress log (deploy, backup, restore).
	StreamText = "text/plain"
	// StreamSSE is a Server-Sent Events stream.
	StreamSSE = "text/event-stream"
)

// Param is a path, query or header parameter.
type Param struct {
	Name        string
	Type        string // string, integer, boolean
	Description string
	Required    bool
	Enum        []string
}

// Field is a top-level key of a request body. Type is a JSON type, "a|b"
// for unions (e.g. "string|null"), "array<string>", "file" (multipart) or
// "any".
type Field struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []string
}

// Route describes one endpoint.
type Route struct {
	Method  string
	Path    string // chi pattern, e.g. /api/projects/{id}
	Tag     string
	Summary string
	// Description adds detail to the summary.
	Description string
	// Auth is one of the Auth* constants; "" means AuthSession.
	Auth    string
	Query   []Param
	Headers []Param
	// BodyType is one of the Body* constants ("" when Body is empty: JSON).
	BodyType string
	Body     []Field
	// StepUp routes re-verify the account password (`password` in the
	// body) for sessions; an API token needs allow_dangerous instead.
	StepUp bool
	// Stream is StreamText or StreamSSE when the response is a stream.
	Stream string
	// SessionOnly routes refuse every API token.
	SessionOnly bool
	// FullScope GET routes refuse read-scope API tokens.
	FullScope bool
	// Response is the response content type when it is neither JSON nor a
	// stream (a ZIP download, an HTML redirect).
	Response string
}

// AuthKind returns r.Auth with the default applied.
func (r Route) AuthKind() string {
	if r.Auth == "" {
		return AuthSession
	}
	return r.Auth
}

// ReadOnly reports whether a read-scope API token may call the route.
func (r Route) ReadOnly() bool {
	return (r.Method == "GET" || r.Method == "HEAD") && !r.FullScope && !r.SessionOnly
}

// Tags in document order, with their descriptions.
var Tags = []struct{ Name, Description string }{
	{"Health", "Liveness."},
	{"Auth", "Setup, login, the account and short-lived purpose tokens."},
	{"API tokens", "Bearer tokens for scripts and AI agents. Dashboard session only."},
	{"Projects", "Applications: create, configure, environment variables, persistent storage."},
	{"Deployments", "Deploy, redeploy, stop, restart, cancel, rollback, history, service domains."},
	{"Databases", "Managed databases and links to apps."},
	{"Domains", "Panel (dashboard) domains, SSL and the ACME email."},
	{"Files", "The project file editor."},
	{"Logs", "Container logs."},
	{"Ports", "The host port pool."},
	{"GitHub", "GitHub App setup, repositories and the push webhook."},
	{"System", "Host metrics, logs, version and host maintenance."},
	{"Backup", "Backups and restore."},
	{"Meta", "This document, the WebSocket terminal and the MCP endpoint."},
}

var (
	password = Field{Name: "password", Type: "string", Description: "Account password (step-up). Not needed with an API token created with allow_dangerous."}
	envFlags = []Field{
		{Name: "service_name", Type: "string|null", Description: "Service the variable applies to; empty or absent means all services."},
		{Name: "is_build_arg", Type: "boolean|null", Description: "Pass to the image build."},
		{Name: "is_runtime", Type: "boolean|null", Description: "Set in the running container (default true)."},
	}
	projectBuild = []Field{
		{Name: "build_type", Type: "string", Description: "auto (default), dockerfile or railpack; anything else means auto.", Enum: []string{"auto", "dockerfile", "railpack"}},
		{Name: "base_directory", Type: "string", Description: "Subdirectory to build from (default \".\")."},
		{Name: "dockerfile_path", Type: "string|null", Description: "Explicit Dockerfile path inside the project."},
		{Name: "internal_port", Type: "integer|string", Description: "Port the app listens on (default 3000, clamped to 1-65535)."},
	}
	tail     = Param{Name: "tail", Type: "integer", Description: "Number of past lines (default 200; negative means all)."}
	sseToken = Param{Name: "token", Type: "string", Description: "SSE token from POST /api/auth/sse-token (not needed with a Bearer API token)."}
	repo     = []Param{
		{Name: "repo", Type: "string", Required: true, Description: "owner/name"},
		{Name: "type", Type: "string", Description: "Repository visibility hint used to pick the installation token."},
	}
	bootstrapHeader = Param{Name: "X-Bootstrap-Secret", Type: "string", Description: "Bootstrap secret from the backend logs or data/.bootstrap-secret."}
)

// Routes is every HTTP route of the server.
var Routes = []Route{
	{Method: "GET", Path: "/api/health", Tag: "Health", Summary: "Health check", Auth: AuthPublic},

	// Auth
	{Method: "GET", Path: "/api/auth/status", Tag: "Auth", Summary: "Setup status (whether an admin exists)", Auth: AuthPublic},
	{Method: "POST", Path: "/api/auth/register", Tag: "Auth", Summary: "Create the first admin", Auth: AuthPublic,
		Description: "Only while no user exists. Needs the bootstrap secret (header or body).",
		Headers:     []Param{bootstrapHeader},
		Body: []Field{
			{Name: "name", Type: "string", Required: true},
			{Name: "email", Type: "string", Required: true},
			{Name: "password", Type: "string", Required: true, Description: "At least 8 characters."},
			{Name: "bootstrapSecret", Type: "string", Description: "Alternative to the X-Bootstrap-Secret header."},
		}},
	{Method: "POST", Path: "/api/auth/login", Tag: "Auth", Summary: "Log in and get a 7 day session token", Auth: AuthPublic,
		Body: []Field{{Name: "email", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true}}},
	{Method: "GET", Path: "/api/auth/me", Tag: "Auth", Summary: "The signed-in user"},
	{Method: "PATCH", Path: "/api/auth/profile", Tag: "Auth", Summary: "Change name and email", SessionOnly: true,
		Body: []Field{{Name: "name", Type: "string", Required: true}, {Name: "email", Type: "string", Required: true}}},
	{Method: "POST", Path: "/api/auth/change-password", Tag: "Auth", Summary: "Change the account password", SessionOnly: true,
		Description: "Ends every older session and revokes every API token created before the change.",
		Body: []Field{
			{Name: "currentPassword", Type: "string", Required: true},
			{Name: "newPassword", Type: "string", Required: true, Description: "At least 8 characters."},
		}},
	{Method: "GET", Path: "/api/auth/setup-token", Tag: "Auth", Summary: "Setup token for a fresh-install restore", Auth: AuthPublic,
		Description: "Refused once a user exists.", Headers: []Param{bootstrapHeader}},
	{Method: "POST", Path: "/api/auth/sse-token", Tag: "Auth", Summary: "5 minute token for log streams (EventSource)", SessionOnly: true},
	{Method: "POST", Path: "/api/auth/terminal-token", Tag: "Auth", Summary: "5 minute single-use token for the WebSocket terminal", SessionOnly: true},

	// API tokens
	{Method: "GET", Path: "/api/tokens", Tag: "API tokens", Summary: "List API tokens", SessionOnly: true},
	{Method: "POST", Path: "/api/tokens", Tag: "API tokens", Summary: "Create an API token (returned once)", SessionOnly: true, StepUp: true,
		Body: []Field{
			{Name: "name", Type: "string", Required: true, Description: "1-100 characters."},
			{Name: "scope", Type: "string", Enum: []string{"read", "full"}, Description: "read (default): GET only. full: every method."},
			{Name: "allowDangerous", Type: "boolean", Description: "Allow actions that ask for the password in the dashboard. Needs scope full."},
			{Name: "expiresInDays", Type: "integer|null", Description: "1-3650; null or absent for no expiry."},
		}},
	{Method: "DELETE", Path: "/api/tokens/{id}", Tag: "API tokens", Summary: "Revoke an API token", SessionOnly: true, StepUp: true},

	// Backup
	{Method: "GET", Path: "/api/backup", Tag: "Backup", Summary: "List backups"},
	{Method: "GET", Path: "/api/backup/uploads", Tag: "Backup", Summary: "List uploaded backup files"},
	{Method: "DELETE", Path: "/api/backup/uploads/{filename}", Tag: "Backup", Summary: "Delete an uploaded backup file"},
	{Method: "POST", Path: "/api/backup/create", Tag: "Backup", Summary: "Create a backup (streamed log)", Stream: StreamText,
		Body: []Field{{Name: "name", Type: "string", Description: "Label for the file name (default docklift)."}}},
	{Method: "POST", Path: "/api/backup/restore/{filename}", Tag: "Backup", Summary: "Restore a server-created backup (streamed log)", StepUp: true, Stream: StreamText,
		Description: "Replaces the database and project state; the server restarts after a committed restore."},
	{Method: "DELETE", Path: "/api/backup/{filename}", Tag: "Backup", Summary: "Delete a backup"},
	{Method: "GET", Path: "/api/backup/download/{filename}", Tag: "Backup", Summary: "Download a backup ZIP", FullScope: true, Response: "application/zip"},
	{Method: "POST", Path: "/api/backup/upload", Tag: "Backup", Summary: "Upload a backup ZIP without restoring it", BodyType: BodyMultipart,
		Body: []Field{{Name: "backup", Type: "file", Required: true, Description: "The backup ZIP (max 500 MiB)."}}},
	{Method: "POST", Path: "/api/backup/restore-upload", Tag: "Backup", Summary: "Upload and restore a backup in one request (streamed log)",
		Auth: AuthSetupOrSession, StepUp: true, Stream: StreamText, BodyType: BodyMultipart,
		Description: "Before the first admin exists it is authorised by the X-Setup-Token header instead.",
		Headers:     []Param{{Name: "X-Setup-Token", Type: "string", Description: "Fresh install only."}},
		Body: []Field{
			{Name: "backup", Type: "file", Required: true, Description: "The backup ZIP (max 500 MiB)."},
			{Name: "password", Type: "string", Description: password.Description},
		}},
	{Method: "POST", Path: "/api/backup/restore-from-upload/{filename}", Tag: "Backup", Summary: "Restore a previously uploaded backup (streamed log)", StepUp: true, Stream: StreamText},
	{Method: "POST", Path: "/api/backup/clear-critical-restore", Tag: "Backup", Summary: "Clear the restore-critical seal", StepUp: true},
	{Method: "GET", Path: "/api/backup/critical-status", Tag: "Backup", Summary: "Restore-critical seal status"},

	// Databases
	{Method: "GET", Path: "/api/databases/engines", Tag: "Databases", Summary: "Supported database engines and versions"},
	{Method: "GET", Path: "/api/databases", Tag: "Databases", Summary: "List managed databases"},
	{Method: "GET", Path: "/api/databases/links/by-app/{appProjectId}", Tag: "Databases", Summary: "Database links of an app project"},
	{Method: "POST", Path: "/api/databases", Tag: "Databases", Summary: "Create (and start) a managed database",
		Body: []Field{
			{Name: "name", Type: "string", Required: true, Description: "Max 120 characters."},
			{Name: "engine", Type: "string", Required: true, Description: "An engine id from GET /api/databases/engines.", Enum: deploy.EngineIDs},
			{Name: "version", Type: "string", Description: "Image tag; default is the recommended version. `tag` and `image` are accepted as aliases."},
		}},
	{Method: "GET", Path: "/api/databases/{id}/connection", Tag: "Databases", Summary: "Connection details and credentials"},
	{Method: "GET", Path: "/api/databases/{id}/links", Tag: "Databases", Summary: "Apps linked to a database"},
	{Method: "POST", Path: "/api/databases/{id}/links", Tag: "Databases", Summary: "Link a database to an app (injects a connection URL env var)",
		Body: []Field{
			{Name: "app_project_id", Type: "string", Required: true},
			{Name: "service_name", Type: "string", Description: "Service of the app that gets the variable; empty means every service."},
			{Name: "env_key", Type: "string", Description: "Variable name (default from the engine, e.g. DATABASE_URL)."},
			{Name: "overwrite", Type: "boolean", Description: "Replace an existing variable with that key."},
		}},
	{Method: "DELETE", Path: "/api/databases/{id}/links/{linkId}", Tag: "Databases", Summary: "Remove a database link"},

	// Deployments
	{Method: "GET", Path: "/api/deployments/{projectId}", Tag: "Deployments", Summary: "Deployment history, newest first",
		Query: []Param{
			{Name: "limit", Type: "integer", Description: "Page size (default 10)."},
			{Name: "offset", Type: "integer"},
			{Name: "status", Type: "string", Description: "Filter by status."},
			{Name: "meta", Type: "string", Description: "1 or true: answer {items, total} instead of an array.", Enum: []string{"1", "true"}},
		}},
	{Method: "GET", Path: "/api/deployments/{projectId}/services", Tag: "Deployments", Summary: "Services of a project"},
	{Method: "GET", Path: "/api/deployments/{projectId}/services/{serviceId}/ssl", Tag: "Deployments", Summary: "SSL status of a service's domains"},
	{Method: "POST", Path: "/api/deployments/{projectId}/services/{serviceId}/dns-check", Tag: "Deployments", Summary: "Check DNS of a service's domains",
		Body: []Field{{Name: "domains", Type: "array<string>", Description: "Domains to check (max 10); default the service's domains."}}},
	{Method: "POST", Path: "/api/deployments/{projectId}/services/{serviceId}/ssl/retry", Tag: "Deployments", Summary: "Retry HTTPS certificates for a service"},
	{Method: "PUT", Path: "/api/deployments/{projectId}/services/{serviceId}", Tag: "Deployments", Summary: "Set a service's domains",
		Description: "Writes the nginx vhost and requests certificates. An empty string or null removes every domain.",
		Body:        []Field{{Name: "domain", Type: "string|null", Description: "Comma-separated domain list."}}},
	{Method: "POST", Path: "/api/deployments/{projectId}/deploy", Tag: "Deployments", Summary: "Deploy (streamed build log)", Stream: StreamText,
		Description: "The deploy keeps running if the client disconnects.",
		Body:        []Field{{Name: "trigger", Type: "string", Description: "Label stored on the deployment (default manual)."}, {Name: "commit_message", Type: "string"}}},
	{Method: "POST", Path: "/api/deployments/{projectId}/redeploy", Tag: "Deployments", Summary: "Redeploy (streamed build log)", Stream: StreamText,
		Body: []Field{{Name: "trigger", Type: "string"}, {Name: "commit_message", Type: "string"}}},
	{Method: "POST", Path: "/api/deployments/{projectId}/stop", Tag: "Deployments", Summary: "Stop the project's containers (streamed log)", Stream: StreamText},
	{Method: "POST", Path: "/api/deployments/{projectId}/restart", Tag: "Deployments", Summary: "Restart the project's containers (streamed log)", Stream: StreamText},
	{Method: "POST", Path: "/api/deployments/{projectId}/rollback", Tag: "Deployments", Summary: "Roll back to an earlier deployment's images (streamed log)", StepUp: true, Stream: StreamText,
		Body: []Field{{Name: "deploymentId", Type: "string", Required: true}}},
	{Method: "POST", Path: "/api/deployments/{projectId}/cancel", Tag: "Deployments", Summary: "Cancel the running build (streamed log)", Stream: StreamText},
	{Method: "GET", Path: "/api/deployments/{projectId}/logs", Tag: "Deployments", Summary: "Last 100 log lines of every service"},
	{Method: "GET", Path: "/api/deployments/{projectId}/stats", Tag: "Deployments", Summary: "CPU and memory of every service"},

	// Domains
	{Method: "GET", Path: "/api/domains/ssl/email", Tag: "Domains", Summary: "ACME account email"},
	{Method: "PUT", Path: "/api/domains/ssl/email", Tag: "Domains", Summary: "Set the ACME account email",
		Body: []Field{{Name: "email", Type: "string|null"}}},
	{Method: "GET", Path: "/api/domains", Tag: "Domains", Summary: "Panel domains with SSL status and access state"},
	{Method: "POST", Path: "/api/domains", Tag: "Domains", Summary: "Add a panel domain",
		Body: []Field{
			{Name: "domain", Type: "string", Required: true},
			{Name: "port", Type: "integer|string", Required: true, Description: "Target port 1-65535 (the dashboard port for the panel)."},
		}},
	{Method: "POST", Path: "/api/domains/dns-check", Tag: "Domains", Summary: "Check DNS of hostnames",
		Body: []Field{{Name: "domains", Type: "array<string>", Required: true}}},
	{Method: "GET", Path: "/api/domains/{domain}/ssl", Tag: "Domains", Summary: "SSL status of a panel domain"},
	{Method: "POST", Path: "/api/domains/{domain}/ssl/retry", Tag: "Domains", Summary: "Retry HTTPS for a panel domain"},
	{Method: "PUT", Path: "/api/domains/{domain}/access", Tag: "Domains", Summary: "Only allow dashboard access through this domain", SessionOnly: true,
		Description: "Turning it on needs the step-up password and a request through the proxy over HTTPS for that domain.",
		Body:        []Field{{Name: "domainOnly", Type: "boolean", Required: true}, {Name: "password", Type: "string", Description: "Required when domainOnly is true."}}},
	{Method: "DELETE", Path: "/api/domains/{domain}", Tag: "Domains", Summary: "Remove a panel domain"},

	// Files
	{Method: "GET", Path: "/api/files/{projectId}", Tag: "Files", Summary: "File tree of a project"},
	{Method: "GET", Path: "/api/files/{projectId}/content", Tag: "Files", Summary: "Read a file",
		Query: []Param{{Name: "path", Type: "string", Required: true, Description: "Path relative to the project."}}},
	{Method: "PUT", Path: "/api/files/{projectId}/content", Tag: "Files", Summary: "Write a file",
		Query: []Param{{Name: "path", Type: "string", Required: true, Description: "Path relative to the project."}},
		Body:  []Field{{Name: "content", Type: "string", Required: true, Description: "Whole new content (max 10 MiB)."}}},

	// GitHub
	{Method: "POST", Path: "/api/github/manifest", Tag: "GitHub", Summary: "Start creating the GitHub App (manifest flow)",
		Body: []Field{{Name: "appName", Type: "string", Required: true}, {Name: "returnUrl", Type: "string", Description: "Dashboard URL to return to."}}},
	{Method: "GET", Path: "/api/github/manifest/callback", Tag: "GitHub", Summary: "GitHub App manifest callback (browser redirect)", Auth: AuthPublic, Response: "text/html",
		Query: []Param{{Name: "code", Type: "string"}, {Name: "state", Type: "string"}, {Name: "installation_id", Type: "string"}}},
	{Method: "GET", Path: "/api/github/app-status", Tag: "GitHub", Summary: "Whether the GitHub App is configured"},
	{Method: "POST", Path: "/api/github/check-installation", Tag: "GitHub", Summary: "Look up the App installation"},
	{Method: "POST", Path: "/api/github/install-session", Tag: "GitHub", Summary: "Start an install session (returns the install URL)",
		Body: []Field{{Name: "returnUrl", Type: "string", Description: "Alias: return_url."}}},
	{Method: "GET", Path: "/api/github/install", Tag: "GitHub", Summary: "Redirect to the App install page", Response: "text/html",
		Query: []Param{{Name: "return_url", Type: "string"}}},
	{Method: "GET", Path: "/api/github/setup", Tag: "GitHub", Summary: "GitHub App setup callback (browser redirect)", Auth: AuthPublic, Response: "text/html",
		Query: []Param{{Name: "installation_id", Type: "string"}, {Name: "state", Type: "string"}}},
	{Method: "GET", Path: "/api/github/callback", Tag: "GitHub", Summary: "Legacy GitHub callback (browser redirect)", Auth: AuthPublic, Response: "text/html",
		Query: []Param{{Name: "installation_id", Type: "string"}, {Name: "state", Type: "string"}}},
	{Method: "GET", Path: "/api/github/status", Tag: "GitHub", Summary: "GitHub connection status"},
	{Method: "GET", Path: "/api/github/installations", Tag: "GitHub", Summary: "App installations"},
	{Method: "GET", Path: "/api/github/repos", Tag: "GitHub", Summary: "Repositories the App can access",
		Query: []Param{{Name: "owner", Type: "string", Description: "Only this account."}}},
	{Method: "GET", Path: "/api/github/branches", Tag: "GitHub", Summary: "Branches of a repository", Query: repo},
	{Method: "GET", Path: "/api/github/tags", Tag: "GitHub", Summary: "Tags of a repository", Query: repo},
	{Method: "POST", Path: "/api/github/disconnect", Tag: "GitHub", Summary: "Forget the App installation"},
	{Method: "POST", Path: "/api/github/webhook", Tag: "GitHub", Summary: "GitHub push webhook", Auth: AuthHMAC,
		Description: "Signed by GitHub (X-Hub-Signature-256 over the raw body). Queues auto-deploys.",
		Headers:     []Param{{Name: "X-Hub-Signature-256", Type: "string", Required: true}, {Name: "X-GitHub-Event", Type: "string", Required: true}},
		Body:        []Field{{Name: "ref", Type: "string"}, {Name: "repository", Type: "object"}, {Name: "head_commit", Type: "object|null"}, {Name: "pusher", Type: "object"}, {Name: "deleted", Type: "boolean"}}},

	// Logs
	{Method: "GET", Path: "/api/logs/{projectId}/containers", Tag: "Logs", Summary: "Containers of a project and their state"},
	{Method: "GET", Path: "/api/logs/{projectId}/stream/{containerName}", Tag: "Logs", Summary: "Follow a container's logs (SSE)", Auth: AuthSSE, Stream: StreamSSE,
		Query: []Param{sseToken, tail}},

	// Ports
	{Method: "GET", Path: "/api/ports", Tag: "Ports", Summary: "Host port pool and Docker port mappings"},
	{Method: "DELETE", Path: "/api/ports/{port}", Tag: "Ports", Summary: "Release a port from the pool"},

	// Projects
	{Method: "GET", Path: "/api/projects", Tag: "Projects", Summary: "List projects"},
	{Method: "POST", Path: "/api/projects", Tag: "Projects", Summary: "Create a project from a GitHub repository or a ZIP upload", BodyType: BodyJSONOrMultipart,
		Description: "A GitHub repository is cloned now; a ZIP (multipart field `files`) is extracted now. Deploy it afterwards.",
		Body: append([]Field{
			{Name: "name", Type: "string", Required: true, Description: "Max 120 characters."},
			{Name: "source_type", Type: "string", Enum: []string{"github", "upload"}, Description: "Default: upload when a file is sent, else github when github_url is set."},
			{Name: "github_url", Type: "string", Description: "https://github.com/owner/repo"},
			{Name: "github_branch", Type: "string", Description: "Branch or tag."},
			{Name: "description", Type: "string"},
			{Name: "domain", Type: "string", Description: "Comma-separated domains."},
			{Name: "project_type", Type: "string", Description: "Default app."},
			{Name: "files", Type: "file", Description: "ZIP of the source (multipart only, max 100 MiB)."},
		}, projectBuild...)},
	{Method: "GET", Path: "/api/projects/{id}", Tag: "Projects", Summary: "Get a project with its services"},
	{Method: "PATCH", Path: "/api/projects/{id}", Tag: "Projects", Summary: "Update project settings",
		Body: append([]Field{
			{Name: "name", Type: "string"},
			{Name: "description", Type: "string|null"},
			{Name: "github_url", Type: "string|null"},
			{Name: "domain", Type: "string|null"},
			{Name: "project_type", Type: "string"},
			{Name: "publish_host_port", Type: "boolean", Description: "Publish host ports from the pool (opt-in)."},
		}, projectBuild...)},
	{Method: "DELETE", Path: "/api/projects/{id}", Tag: "Projects", Summary: "Delete a project, its containers, images and files"},
	{Method: "GET", Path: "/api/projects/{id}/build/detect", Tag: "Projects", Summary: "Detect how the project would be built"},
	{Method: "GET", Path: "/api/projects/{id}/storage", Tag: "Projects", Summary: "Persistent volumes"},
	{Method: "POST", Path: "/api/projects/{id}/storage", Tag: "Projects", Summary: "Add a persistent volume",
		Body: []Field{
			{Name: "name", Type: "string", Required: true, Description: "1-32 letters, numbers, _ or -."},
			{Name: "service_name", Type: "string", Required: true},
			{Name: "mount_path", Type: "string", Required: true, Description: "Absolute path in the container."},
		}},
	{Method: "DELETE", Path: "/api/projects/{id}/storage/{storageId}", Tag: "Projects", Summary: "Delete a persistent volume and its data",
		Query: []Param{{Name: "removeVolume", Type: "string", Required: true, Enum: []string{"true"}, Description: "Confirms permanent data deletion."}}},
	{Method: "GET", Path: "/api/projects/{id}/env", Tag: "Projects", Summary: "List environment variables",
		Query: []Param{{Name: "service", Type: "string", Description: "Only this service scope."}}},
	{Method: "POST", Path: "/api/projects/{id}/env", Tag: "Projects", Summary: "Create an environment variable",
		Body: append([]Field{
			{Name: "key", Type: "string", Required: true, Description: "Letters, digits and _, not starting with a digit."},
			{Name: "value", Type: "string"},
			{Name: "is_secret", Type: "boolean", Description: "Pass a build variable as a BuildKit secret."},
		}, envFlags...)},
	{Method: "POST", Path: "/api/projects/{id}/env/bulk", Tag: "Projects", Summary: "Import KEY=VALUE lines",
		Body: append([]Field{{Name: "content", Type: "string", Required: true, Description: ".env text."}}, envFlags...)},
	{Method: "DELETE", Path: "/api/projects/{id}/env/{envId}", Tag: "Projects", Summary: "Delete an environment variable"},
	{Method: "PATCH", Path: "/api/projects/{id}/auto-deploy", Tag: "Projects", Summary: "Turn auto-deploy on push on or off (GitHub projects)",
		Body: []Field{{Name: "enabled", Type: "boolean|null"}}},
	{Method: "GET", Path: "/api/projects/{id}/auto-deploy", Tag: "Projects", Summary: "Auto-deploy settings and webhook URL"},

	// System
	{Method: "GET", Path: "/api/system/stats", Tag: "System", Summary: "Host metrics"},
	{Method: "GET", Path: "/api/system/logs/{service}", Tag: "System", Summary: "Follow a Docklift container's logs (SSE)", Auth: AuthSSE, Stream: StreamSSE,
		Query: []Param{sseToken, tail}},
	{Method: "GET", Path: "/api/system/quick", Tag: "System", Summary: "CPU and memory percent"},
	{Method: "GET", Path: "/api/system/ip", Tag: "System", Summary: "Server IP addresses"},
	{Method: "POST", Path: "/api/system/purge", Tag: "System", Summary: "Remove unused Docklift images and build cache", StepUp: true},
	{Method: "POST", Path: "/api/system/reboot", Tag: "System", Summary: "Reboot the host", StepUp: true},
	{Method: "POST", Path: "/api/system/reset", Tag: "System", Summary: "Restart the Docklift containers", StepUp: true},
	{Method: "POST", Path: "/api/system/execute", Tag: "System", Summary: "Run a shell command on the host (30 s, 1 MiB output)", StepUp: true,
		Body: []Field{{Name: "command", Type: "string", Required: true}}},
	{Method: "GET", Path: "/api/system/version", Tag: "System", Summary: "Version and update status",
		Query: []Param{{Name: "refresh", Type: "string", Enum: []string{"1", "true"}, Description: "Check for a new release now."}}},
	{Method: "POST", Path: "/api/system/update-system", Tag: "System", Summary: "Update the host OS packages", StepUp: true},
	{Method: "POST", Path: "/api/system/upgrade", Tag: "System", Summary: "Upgrade Docklift", StepUp: true},

	// Meta
	{Method: "GET", Path: "/api/openapi.json", Tag: "Meta", Summary: "This OpenAPI document"},
	{Method: "GET", Path: "/ws/terminal", Tag: "Meta", Summary: "WebSocket host terminal (upgrade)", Auth: AuthTerminal, SessionOnly: true,
		Description: "Needs a trusted Origin and ?token= from POST /api/auth/terminal-token, then the account password inside the socket.",
		Query:       []Param{{Name: "token", Type: "string", Required: true}}},
	{Method: "POST", Path: "/mcp", Tag: "Meta", Summary: "MCP endpoint (Streamable HTTP, stateless JSON-RPC)", Auth: AuthAPIToken,
		Description: "Model Context Protocol for AI agents. Bearer API token only. See https://docklift.dev/guide/mcp",
		Headers:     []Param{{Name: "MCP-Protocol-Version", Type: "string"}, {Name: "Accept", Type: "string", Required: true, Description: "application/json, text/event-stream"}},
		Body:        []Field{{Name: "jsonrpc", Type: "string", Required: true}, {Name: "id", Type: "integer|string"}, {Name: "method", Type: "string", Required: true}, {Name: "params", Type: "object"}}},
	{Method: "GET", Path: "/mcp", Tag: "Meta", Summary: "MCP endpoint: no server stream (405)", Auth: AuthAPIToken},
	{Method: "DELETE", Path: "/mcp", Tag: "Meta", Summary: "MCP endpoint: no sessions (405)", Auth: AuthAPIToken},
}

// StepUpField is the password body field step-up routes accept.
func StepUpField() Field { return password }
