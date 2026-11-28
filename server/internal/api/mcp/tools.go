package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docklift/internal/api/openapi"
)

// args are the decoded arguments of a tool call.
type args map[string]any

func (a args) str(key string) string {
	switch v := a[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	}
	return ""
}

func (a args) has(key string) bool {
	v, ok := a[key]
	return ok && v != nil
}

func (a args) boolean(key string) bool {
	b, _ := a[key].(bool)
	return b
}

func (a args) int(key string, def int) int {
	switch v := a[key].(type) {
	case json.Number:
		if n, err := strconv.Atoi(v.String()); err == nil {
			return n
		}
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// call is a tool invocation: the router dispatcher bound to the MCP request.
type call struct {
	ctx   context.Context
	outer *http.Request
	d     *Dispatcher
}

func (c *call) do(req Request) (*Response, error) { return c.d.Do(c.ctx, c.outer, req) }

// tool is one MCP tool.
type tool struct {
	def *mcp.Tool
	run func(c *call, a args) (*mcp.CallToolResult, error)
}

// errMissing is a missing required argument.
func errMissing(names ...string) error {
	return fmt.Errorf("missing required argument: %s", strings.Join(names, ", "))
}

func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isError}
}

// render formats a router response for the agent: the status line, notes
// about cut output, then the body (pretty JSON when it is JSON; SSE events
// turned into plain lines).
func render(req Request, resp *Response) *mcp.CallToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s -> HTTP %d\n", strings.ToUpper(req.Method), req.Path, resp.Status)
	body := resp.Body
	if strings.HasPrefix(resp.ContentType, "text/event-stream") {
		body = []byte(sseText(body))
	}
	if resp.TimedOut {
		b.WriteString("[note] Stopped following the stream after the wait limit; a deploy, backup or restore keeps running on the server. Check the deployment history or call again for the result.\n")
	}
	if resp.Truncated {
		if resp.Stream {
			fmt.Fprintf(&b, "[note] Output was longer than %d KiB; only the last part is shown.\n", MaxStreamBytes>>10)
		} else {
			fmt.Fprintf(&b, "[note] Response was longer than %d KiB and was cut at the end.\n", MaxResponseBytes>>10)
		}
	}
	if strings.HasPrefix(resp.ContentType, "application/json") && !resp.Truncated {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			body = pretty.Bytes()
		}
	}
	if resp.Stream || strings.HasPrefix(resp.ContentType, "application/json") || strings.HasPrefix(resp.ContentType, "text/") {
		b.Write(body)
	} else if len(body) > 0 {
		fmt.Fprintf(&b, "(%d bytes of %s not shown)", len(body), resp.ContentType)
	}
	failed := resp.Status >= 400 || streamFailed(body)
	return textResult(b.String(), failed)
}

// streamFailed reports a progress stream that ended in an error line.
func streamFailed(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "[ERROR]") && !strings.Contains(s[strings.LastIndex(s, "[ERROR]"):], "[SUCCESS]")
}

// sseText turns `data: {"type":"log","message":"..."}` events into lines.
func sseText(raw []byte) string {
	var out strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev struct {
			Type, Message, Container string
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			out.WriteString(data + "\n")
			continue
		}
		switch ev.Type {
		case "log":
			out.WriteString(strings.TrimRight(ev.Message, "\n") + "\n")
		case "connected":
			out.WriteString("[connected to " + ev.Container + "]\n")
		default:
			out.WriteString("[" + ev.Type + "] " + ev.Message + "\n")
		}
	}
	return out.String()
}

// simple is a tool that maps its arguments to one router request.
func simple(def *mcp.Tool, build func(a args) (Request, error)) tool {
	return tool{def: def, run: func(c *call, a args) (*mcp.CallToolResult, error) {
		req, err := build(a)
		if err != nil {
			return nil, err
		}
		resp, err := c.do(req)
		if err != nil {
			return nil, err
		}
		return render(req, resp), nil
	}}
}

// schema builds an object JSON Schema from properties and required names.
func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, desc string) map[string]any { return map[string]any{"type": typ, "description": desc} }

func enumProp(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

var (
	yes = true
	no  = false
)

func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &no}
}

func writes(title string, destructive bool) *mcp.ToolAnnotations {
	d := destructive
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &d, OpenWorldHint: &no}
}

var projectIDProp = prop("string", "Project id (from docklift_list_projects).")

func pid(a args) (string, error) {
	id := a.str("projectId")
	if id == "" {
		return "", errMissing("projectId")
	}
	if !safeSegment(id) {
		return "", errors.New("projectId is not a valid id")
	}
	return id, nil
}

// safeSegment reports a value that can be one path segment as is.
func safeSegment(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\?#% \t\r\n")
}

// safeAPIPath reports a clean /api/ path: no empty, dot or dot-dot segments.
func safeAPIPath(p string) bool {
	if !strings.HasPrefix(p, "/api/") || strings.ContainsAny(p, "\\?#\r\n") {
		return false
	}
	for _, seg := range strings.Split(strings.TrimSuffix(p[1:], "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func waitArg(a args) time.Duration {
	if s := a.int("waitSeconds", 0); s > 0 {
		return time.Duration(s) * time.Second
	}
	return 0
}

var waitProp = prop("integer", "How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.")

// tools is the curated tool set plus the generic escape hatches.
func tools() []tool {
	return []tool{
		simple(&mcp.Tool{Name: "docklift_list_projects", Description: "List every project (apps and managed databases) with status, source, domains and services.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("List projects")},
			func(args) (Request, error) { return Request{Method: "GET", Path: "/api/projects"}, nil }),

		simple(&mcp.Tool{Name: "docklift_get_project", Description: "Get one project with its services, build settings, status and domains.",
			InputSchema: schema(map[string]any{"projectId": projectIDProp}, "projectId"), Annotations: readOnly("Get project")},
			func(a args) (Request, error) {
				id, err := pid(a)
				return Request{Method: "GET", Path: "/api/projects/" + id}, err
			}),

		simple(&mcp.Tool{Name: "docklift_create_project",
			Description: "Create a project from a GitHub repository (https://github.com/owner/repo). The repository is cloned now; call docklift_deploy afterwards to build and start it. Private repositories need the GitHub App installed in Docklift.",
			InputSchema: schema(map[string]any{
				"name":            prop("string", "Project name (max 120 characters)."),
				"github_url":      prop("string", "https://github.com/owner/repo"),
				"github_branch":   prop("string", "Branch or tag (default: the repository's default branch)."),
				"description":     prop("string", "Optional description."),
				"domain":          prop("string", "Optional comma-separated domains for the first service."),
				"build_type":      enumProp("How to build (default auto: Dockerfile/compose if present, else Railpack).", "auto", "dockerfile", "railpack"),
				"base_directory":  prop("string", "Subdirectory to build from (monorepos), default \".\"."),
				"dockerfile_path": prop("string", "Explicit Dockerfile path inside the project."),
				"internal_port":   prop("integer", "Port the app listens on inside the container (default 3000)."),
			}, "name", "github_url"), Annotations: writes("Create project", false)},
			func(a args) (Request, error) {
				if a.str("name") == "" || a.str("github_url") == "" {
					return Request{}, errMissing("name", "github_url")
				}
				body := map[string]any{"source_type": "github"}
				for _, k := range []string{"name", "github_url", "github_branch", "description", "domain", "build_type", "base_directory", "dockerfile_path"} {
					if v := a.str(k); v != "" {
						body[k] = v
					}
				}
				if a.has("internal_port") {
					body["internal_port"] = a.int("internal_port", 3000)
				}
				return Request{Method: "POST", Path: "/api/projects", Body: body}, nil
			}),

		simple(&mcp.Tool{Name: "docklift_delete_project", Description: "Delete a project for good: its containers, images, volumes records, files and history. This cannot be undone.",
			InputSchema: schema(map[string]any{"projectId": projectIDProp}, "projectId"), Annotations: writes("Delete project", true)},
			func(a args) (Request, error) {
				id, err := pid(a)
				return Request{Method: "DELETE", Path: "/api/projects/" + id}, err
			}),

		simple(&mcp.Tool{Name: "docklift_deploy",
			Description: "Build and start a project (pull/clone source, build the image, start containers) and return the build output. Set redeploy to rebuild the current source. Output is followed until the deploy ends or waitSeconds pass; only the last 64 KiB is returned.",
			InputSchema: schema(map[string]any{
				"projectId":      projectIDProp,
				"redeploy":       prop("boolean", "Use the redeploy route instead of deploy."),
				"commit_message": prop("string", "Optional note stored on the deployment."),
				"waitSeconds":    waitProp,
			}, "projectId"), Annotations: writes("Deploy", false)},
			func(a args) (Request, error) {
				id, err := pid(a)
				route := "/deploy"
				if a.boolean("redeploy") {
					route = "/redeploy"
				}
				body := map[string]any{"trigger": "mcp"}
				if m := a.str("commit_message"); m != "" {
					body["commit_message"] = m
				}
				return Request{Method: "POST", Path: "/api/deployments/" + id + route, Body: body, Wait: waitArg(a)}, err
			}),

		lifecycle("docklift_stop", "stop", "Stop every container of a project (returns the log)."),
		lifecycle("docklift_restart", "restart", "Restart every container of a project (returns the log)."),

		simple(&mcp.Tool{Name: "docklift_project_logs", Description: "The last 100 log lines of every running service of a project.",
			InputSchema: schema(map[string]any{"projectId": projectIDProp}, "projectId"), Annotations: readOnly("Project logs")},
			func(a args) (Request, error) {
				id, err := pid(a)
				return Request{Method: "GET", Path: "/api/deployments/" + id + "/logs"}, err
			}),

		simple(&mcp.Tool{Name: "docklift_container_logs",
			Description: "Read one container's logs: the last `tail` lines, then follows new lines for `seconds` (default 3, max 60). Container names come from docklift_get_project (services[].container_name).",
			InputSchema: schema(map[string]any{
				"projectId":     projectIDProp,
				"containerName": prop("string", "Container name of a service of the project."),
				"tail":          prop("integer", "Past lines to include (default 200)."),
				"seconds":       prop("integer", "How long to follow new output (default 3, max 60)."),
			}, "projectId", "containerName"), Annotations: readOnly("Container logs")},
			func(a args) (Request, error) {
				id, err := pid(a)
				name := a.str("containerName")
				if err != nil || name == "" {
					return Request{}, errMissing("projectId", "containerName")
				}
				if !safeSegment(name) {
					return Request{}, errors.New("containerName is not a valid container name")
				}
				q := url.Values{"tail": {strconv.Itoa(a.int("tail", 200))}}
				secs := min(max(a.int("seconds", 3), 1), 60)
				return Request{Method: "GET", Path: "/api/logs/" + id + "/stream/" + name, Query: q, Wait: time.Duration(secs) * time.Second}, nil
			}),

		simple(&mcp.Tool{Name: "docklift_list_env", Description: "List a project's environment variables (values included).",
			InputSchema: schema(map[string]any{"projectId": projectIDProp, "service": prop("string", "Only variables of this service scope (\"\" = shared).")}, "projectId"),
			Annotations: readOnly("List env vars")},
			func(a args) (Request, error) {
				id, err := pid(a)
				var q url.Values
				if a.has("service") {
					q = url.Values{"service": {a.str("service")}}
				}
				return Request{Method: "GET", Path: "/api/projects/" + id + "/env", Query: q}, err
			}),

		{def: &mcp.Tool{Name: "docklift_set_env",
			Description: "Create or replace an environment variable of a project (replacing deletes the old one with the same key and scope first). Redeploy for running containers to see it.",
			InputSchema: schema(map[string]any{
				"projectId":    projectIDProp,
				"key":          prop("string", "Variable name: letters, digits and _, not starting with a digit."),
				"value":        prop("string", "Value."),
				"service_name": prop("string", "Service scope; empty (default) means every service."),
				"is_build_arg": prop("boolean", "Also pass to the image build (default false)."),
				"is_runtime":   prop("boolean", "Set in the running container (default true)."),
				"is_secret":    prop("boolean", "Pass a build variable as a BuildKit secret instead of a build arg."),
			}, "projectId", "key", "value"), Annotations: writes("Set env var", false)},
			run: setEnv},

		{def: &mcp.Tool{Name: "docklift_delete_env", Description: "Delete an environment variable of a project by key (and service scope).",
			InputSchema: schema(map[string]any{
				"projectId":    projectIDProp,
				"key":          prop("string", "Variable name."),
				"service_name": prop("string", "Service scope; empty (default) means the shared scope."),
			}, "projectId", "key"), Annotations: writes("Delete env var", true)},
			run: deleteEnv},

		simple(&mcp.Tool{Name: "docklift_list_databases", Description: "List managed databases (Postgres, MySQL, MariaDB, MongoDB, Redis).",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("List databases")},
			func(args) (Request, error) { return Request{Method: "GET", Path: "/api/databases"}, nil }),

		simple(&mcp.Tool{Name: "docklift_create_database", Description: "Create and start a managed database. Use docklift_api_request POST /api/databases/{id}/links to inject its URL into an app.",
			InputSchema: schema(map[string]any{
				"name":    prop("string", "Name (max 120 characters)."),
				"engine":  enumProp("Engine.", "postgres", "mysql", "mariadb", "mongodb", "redis"),
				"version": prop("string", "Image tag (default: the recommended version, see GET /api/databases/engines)."),
			}, "name", "engine"), Annotations: writes("Create database", false)},
			func(a args) (Request, error) {
				if a.str("name") == "" || a.str("engine") == "" {
					return Request{}, errMissing("name", "engine")
				}
				body := map[string]any{"name": a.str("name"), "engine": a.str("engine")}
				if v := a.str("version"); v != "" {
					body["version"] = v
				}
				return Request{Method: "POST", Path: "/api/databases", Body: body}, nil
			}),

		simple(&mcp.Tool{Name: "docklift_list_domains", Description: "List the services of a project with their domains (comma-separated) and ids.",
			InputSchema: schema(map[string]any{"projectId": projectIDProp}, "projectId"), Annotations: readOnly("List domains")},
			func(a args) (Request, error) {
				id, err := pid(a)
				return Request{Method: "GET", Path: "/api/deployments/" + id + "/services"}, err
			}),

		domainTool("docklift_add_domain", "Add a domain to a service of a project. Writes the nginx config and requests an HTTPS certificate (point DNS at the server first).", true),
		domainTool("docklift_remove_domain", "Remove a domain from a service of a project.", false),

		simple(&mcp.Tool{Name: "docklift_system_stats", Description: "Host metrics: CPU, memory, disk, network, uptime, Docker.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("System stats")},
			func(args) (Request, error) { return Request{Method: "GET", Path: "/api/system/stats"}, nil }),

		simple(&mcp.Tool{Name: "docklift_list_deployments", Description: "Deployment history of a project, newest first, with status, trigger, commit and logs.",
			InputSchema: schema(map[string]any{
				"projectId": projectIDProp,
				"limit":     prop("integer", "Page size (default 10)."),
				"offset":    prop("integer", "Rows to skip."),
				"status":    prop("string", "Only this status (e.g. success, failed, building)."),
			}, "projectId"), Annotations: readOnly("Deployment history")},
			func(a args) (Request, error) {
				id, err := pid(a)
				q := url.Values{"meta": {"1"}, "limit": {strconv.Itoa(a.int("limit", 10))}, "offset": {strconv.Itoa(a.int("offset", 0))}}
				if s := a.str("status"); s != "" {
					q.Set("status", s)
				}
				return Request{Method: "GET", Path: "/api/deployments/" + id, Query: q}, err
			}),

		simple(&mcp.Tool{Name: "docklift_create_backup", Description: "Create a backup of the database, project files and settings, and return the log.",
			InputSchema: schema(map[string]any{"name": prop("string", "Optional label for the file name."), "waitSeconds": waitProp}),
			Annotations: writes("Create backup", false)},
			func(a args) (Request, error) {
				body := map[string]any{}
				if n := a.str("name"); n != "" {
					body["name"] = n
				}
				return Request{Method: "POST", Path: "/api/backup/create", Body: body, Wait: waitArg(a)}, nil
			}),

		simple(&mcp.Tool{Name: "docklift_list_backups", Description: "List backups on the server.",
			InputSchema: schema(map[string]any{}), Annotations: readOnly("List backups")},
			func(args) (Request, error) { return Request{Method: "GET", Path: "/api/backup"}, nil }),

		{def: &mcp.Tool{Name: "docklift_list_endpoints",
			Description: "List every Docklift HTTP endpoint (method, path, summary, body fields, flags) for use with docklift_api_request. Filter by tag or a search word. The full OpenAPI document is GET /api/openapi.json.",
			InputSchema: schema(map[string]any{
				"tag":    prop("string", "Only this tag (Projects, Deployments, Databases, Domains, Files, Logs, Ports, GitHub, System, Backup, Auth, API tokens, Health, Meta)."),
				"search": prop("string", "Only endpoints whose path or summary contains this text."),
			}), Annotations: readOnly("List endpoints")},
			run: listEndpoints},

		{def: &mcp.Tool{Name: "docklift_api_request",
			Description: "Call any Docklift API endpoint as this token (see docklift_list_endpoints). Read-scope tokens may only use GET. Endpoints marked step-up need a token created with allow_dangerous; token management, password, terminal and the domain-only switch are never available. Streamed responses are followed up to waitSeconds and the tail is returned.",
			InputSchema: schema(map[string]any{
				"method":      enumProp("HTTP method.", "GET", "POST", "PUT", "PATCH", "DELETE"),
				"path":        prop("string", "Path starting with /api/, e.g. /api/projects/abc/env."),
				"query":       map[string]any{"type": "object", "description": "Query parameters.", "additionalProperties": map[string]any{"type": "string"}},
				"body":        map[string]any{"type": "object", "description": "JSON body."},
				"waitSeconds": waitProp,
			}, "method", "path"), Annotations: &mcp.ToolAnnotations{Title: "API request", DestructiveHint: &yes, OpenWorldHint: &no}},
			run: apiRequest},
	}
}

// ToolDefs returns the definition of every tool, in the order they are
// defined here. The docs generator (cmd/gen-docs) builds the website's tool
// reference from it.
func ToolDefs() []*mcp.Tool {
	list := tools()
	defs := make([]*mcp.Tool, 0, len(list))
	for _, t := range list {
		defs = append(defs, t.def)
	}
	return defs
}

func lifecycle(name, route, desc string) tool {
	return simple(&mcp.Tool{Name: name, Description: desc,
		InputSchema: schema(map[string]any{"projectId": projectIDProp, "waitSeconds": waitProp}, "projectId"), Annotations: writes(name, false)},
		func(a args) (Request, error) {
			id, err := pid(a)
			return Request{Method: "POST", Path: "/api/deployments/" + id + "/" + route, Wait: waitArg(a)}, err
		})
}

// envRow is the part of an env variable row the tools read.
type envRow struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	ServiceName string `json:"service_name"`
}

func findEnv(c *call, id, key, scope string) (*envRow, *mcp.CallToolResult, error) {
	req := Request{Method: "GET", Path: "/api/projects/" + id + "/env", Query: url.Values{"service": {scope}}}
	resp, err := c.do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.Status != http.StatusOK {
		return nil, render(req, resp), nil
	}
	var rows []envRow
	if err := resp.JSON(&rows); err != nil {
		return nil, nil, fmt.Errorf("read env list: %w", err)
	}
	for i := range rows {
		if rows[i].Key == key && rows[i].ServiceName == scope {
			return &rows[i], nil, nil
		}
	}
	return nil, nil, nil
}

func setEnv(c *call, a args) (*mcp.CallToolResult, error) {
	id, err := pid(a)
	key, scope := a.str("key"), a.str("service_name")
	if err != nil || key == "" || !a.has("value") {
		return nil, errMissing("projectId", "key", "value")
	}
	existing, refused, err := findEnv(c, id, key, scope)
	if err != nil || refused != nil {
		return refused, err
	}
	body := map[string]any{"key": key, "value": fmt.Sprint(a["value"]), "service_name": scope}
	for _, k := range []string{"is_build_arg", "is_runtime", "is_secret"} {
		if b, ok := a[k].(bool); ok {
			body[k] = b
		}
	}
	if existing != nil {
		del := Request{Method: "DELETE", Path: "/api/projects/" + id + "/env/" + existing.ID}
		resp, err := c.do(del)
		if err != nil {
			return nil, err
		}
		if resp.Status != http.StatusOK {
			return render(del, resp), nil
		}
	}
	req := Request{Method: "POST", Path: "/api/projects/" + id + "/env", Body: body}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	out := render(req, resp)
	if existing != nil && !out.IsError {
		out.Content = append([]mcp.Content{&mcp.TextContent{Text: "Replaced the existing variable " + key + "."}}, out.Content...)
	}
	return out, nil
}

func deleteEnv(c *call, a args) (*mcp.CallToolResult, error) {
	id, err := pid(a)
	key, scope := a.str("key"), a.str("service_name")
	if err != nil || key == "" {
		return nil, errMissing("projectId", "key")
	}
	existing, refused, err := findEnv(c, id, key, scope)
	if err != nil || refused != nil {
		return refused, err
	}
	if existing == nil {
		return textResult(fmt.Sprintf("No variable %s in scope %q.", key, scope), true), nil
	}
	req := Request{Method: "DELETE", Path: "/api/projects/" + id + "/env/" + existing.ID}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return render(req, resp), nil
}

// serviceRow is the part of a service row the domain tools read.
type serviceRow struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Domain *string `json:"domain"`
}

func domainTool(name, desc string, add bool) tool {
	title := "Remove domain"
	if add {
		title = "Add domain"
	}
	return tool{def: &mcp.Tool{Name: name, Description: desc,
		InputSchema: schema(map[string]any{
			"projectId": projectIDProp,
			"domain":    prop("string", "Hostname, e.g. app.example.com."),
			"service":   prop("string", "Service id or name; optional when the project has one service."),
		}, "projectId", "domain"), Annotations: writes(title, !add)},
		run: func(c *call, a args) (*mcp.CallToolResult, error) {
			id, err := pid(a)
			domain := strings.ToLower(a.str("domain"))
			if err != nil || domain == "" {
				return nil, errMissing("projectId", "domain")
			}
			list := Request{Method: "GET", Path: "/api/deployments/" + id + "/services"}
			resp, err := c.do(list)
			if err != nil {
				return nil, err
			}
			if resp.Status != http.StatusOK {
				return render(list, resp), nil
			}
			var services []serviceRow
			if err := resp.JSON(&services); err != nil {
				return nil, fmt.Errorf("read services: %w", err)
			}
			svc, msg := pickService(services, a.str("service"))
			if svc == nil {
				return textResult(msg, true), nil
			}
			current := []string{}
			if svc.Domain != nil {
				for _, d := range strings.Split(*svc.Domain, ",") {
					if d = strings.TrimSpace(d); d != "" {
						current = append(current, d)
					}
				}
			}
			next := []string{}
			found := false
			for _, d := range current {
				if strings.EqualFold(d, domain) {
					found = true
					if !add {
						continue
					}
				}
				next = append(next, d)
			}
			switch {
			case add && found:
				return textResult(domain+" is already a domain of service "+svc.Name+".", false), nil
			case add:
				next = append(next, domain)
			case !found:
				return textResult(domain+" is not a domain of service "+svc.Name+".", true), nil
			}
			put := Request{Method: "PUT", Path: "/api/deployments/" + id + "/services/" + svc.ID, Body: map[string]any{"domain": strings.Join(next, ",")}}
			resp, err = c.do(put)
			if err != nil {
				return nil, err
			}
			return render(put, resp), nil
		}}
}

func pickService(services []serviceRow, want string) (*serviceRow, string) {
	names := make([]string, 0, len(services))
	for i := range services {
		s := &services[i]
		names = append(names, s.Name+" ("+s.ID+")")
		if want != "" && (s.ID == want || s.Name == want) {
			return s, ""
		}
	}
	switch {
	case want != "":
		return nil, "No service " + want + ". Services: " + strings.Join(names, ", ")
	case len(services) == 1:
		return &services[0], ""
	case len(services) == 0:
		return nil, "The project has no services yet. Deploy it first."
	}
	return nil, "The project has several services; pass `service`: " + strings.Join(names, ", ")
}

func listEndpoints(_ *call, a args) (*mcp.CallToolResult, error) {
	tag, search := strings.ToLower(a.str("tag")), strings.ToLower(a.str("search"))
	var b strings.Builder
	n := 0
	routes := append([]openapi.Route(nil), openapi.Routes...)
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].Tag < routes[j].Tag })
	last := ""
	for _, r := range routes {
		if tag != "" && strings.ToLower(r.Tag) != tag {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Path+" "+r.Summary), search) {
			continue
		}
		if r.Tag != last {
			fmt.Fprintf(&b, "\n## %s\n", r.Tag)
			last = r.Tag
		}
		n++
		fmt.Fprintf(&b, "%s %s - %s", r.Method, r.Path, r.Summary)
		var flags []string
		if r.StepUp {
			flags = append(flags, "step-up: needs allow_dangerous")
		}
		if r.SessionOnly {
			flags = append(flags, "not available to API tokens")
		}
		if r.FullScope {
			flags = append(flags, "full scope")
		}
		if r.Stream != "" {
			flags = append(flags, "streams "+r.Stream)
		}
		if a := r.AuthKind(); a != openapi.AuthSession {
			flags = append(flags, "auth: "+a)
		}
		if len(flags) > 0 {
			b.WriteString(" [" + strings.Join(flags, "; ") + "]")
		}
		b.WriteString("\n")
		if len(r.Query) > 0 {
			var qs []string
			for _, q := range r.Query {
				qs = append(qs, paramText(q.Name, q.Type, q.Required))
			}
			b.WriteString("    query: " + strings.Join(qs, ", ") + "\n")
		}
		if fields := openapi.BodyFields(r); len(fields) > 0 {
			var fs []string
			for _, f := range fields {
				fs = append(fs, paramText(f.Name, f.Type, f.Required))
			}
			kind := "body"
			if r.BodyType == openapi.BodyMultipart {
				kind = "multipart body (not available through docklift_api_request)"
			}
			b.WriteString("    " + kind + ": " + strings.Join(fs, ", ") + "\n")
		}
	}
	if n == 0 {
		return textResult("No endpoint matches.", false), nil
	}
	return textResult(fmt.Sprintf("%d endpoints.%s", n, b.String()), false), nil
}

func paramText(name, typ string, required bool) string {
	s := name + " (" + typ
	if required {
		s += ", required"
	}
	return s + ")"
}

func apiRequest(c *call, a args) (*mcp.CallToolResult, error) {
	method := strings.ToUpper(a.str("method"))
	path := a.str("path")
	if method == "" || path == "" {
		return nil, errMissing("method", "path")
	}
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, errors.New("method must be GET, POST, PUT, PATCH or DELETE")
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "" || u.Host != "" || u.RawPath != "" || !safeAPIPath(u.Path) {
		return nil, errors.New("path must be a clean server path starting with /api/ (no .., no encoded slashes)")
	}
	q := u.Query()
	if raw, ok := a["query"].(map[string]any); ok {
		for k, v := range raw {
			q.Set(k, fmt.Sprint(v))
		}
	}
	req := Request{Method: method, Path: u.Path, Query: q, Wait: waitArg(a)}
	if b, ok := a["body"]; ok && b != nil {
		if method == "GET" {
			return nil, errors.New("a GET request has no body")
		}
		req.Body = b
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return render(req, resp), nil
}
