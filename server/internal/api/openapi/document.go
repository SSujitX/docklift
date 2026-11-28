package openapi

import (
	"net/http"
	"regexp"
	"strings"

	"docklift/internal/httpx"
)

// Version is the OpenAPI version of the document.
const Version = "3.1.0"

var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

// PathParams lists the {name} segments of a chi pattern.
func PathParams(path string) []string {
	var out []string
	for _, m := range pathParamRe.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

// OperationID is a stable identifier derived from method and path.
func OperationID(r Route) string {
	p := strings.NewReplacer("{", "", "}", "", ".", "_", "-", "_").Replace(strings.TrimPrefix(r.Path, "/"))
	return strings.ToLower(r.Method) + "_" + strings.ReplaceAll(p, "/", "_")
}

// schemaOf turns a Field/Param type into a JSON Schema.
func schemaOf(typ string, enum []string) map[string]any {
	s := map[string]any{}
	switch {
	case typ == "" || typ == "any":
	case typ == "file":
		s["type"], s["format"] = "string", "binary"
	case strings.HasPrefix(typ, "array<") && strings.HasSuffix(typ, ">"):
		s["type"] = "array"
		s["items"] = schemaOf(strings.TrimSuffix(strings.TrimPrefix(typ, "array<"), ">"), nil)
	case strings.Contains(typ, "|"):
		s["type"] = strings.Split(typ, "|")
	default:
		s["type"] = typ
	}
	if len(enum) > 0 {
		s["enum"] = enum
	}
	return s
}

// BodyFields returns the body keys of r, with `password` added on step-up
// routes that do not list it.
func BodyFields(r Route) []Field {
	fields := append([]Field(nil), r.Body...)
	if r.StepUp {
		for _, f := range fields {
			if f.Name == "password" {
				return fields
			}
		}
		fields = append(fields, StepUpField())
	}
	return fields
}

// BodySchema is the JSON Schema of r's request body (nil without a body).
func BodySchema(r Route) map[string]any {
	fields := BodyFields(r)
	if len(fields) == 0 {
		return nil
	}
	props := map[string]any{}
	var required []string
	for _, f := range fields {
		s := schemaOf(f.Type, f.Enum)
		if f.Description != "" {
			s["description"] = f.Description
		}
		props[f.Name] = s
		if f.Required {
			required = append(required, f.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func param(in string, p Param) map[string]any {
	s := schemaOf(p.Type, p.Enum)
	out := map[string]any{"name": p.Name, "in": in, "schema": s}
	if p.Required || in == "path" {
		out["required"] = true
	}
	if p.Description != "" {
		out["description"] = p.Description
	}
	return out
}

func security(r Route) []map[string][]string {
	switch r.AuthKind() {
	case AuthPublic, AuthHMAC:
		return []map[string][]string{}
	case AuthSSE:
		return []map[string][]string{{"bearerAuth": {}}, {"sseToken": {}}}
	case AuthSetupOrSession:
		return []map[string][]string{{"bearerAuth": {}}, {"setupToken": {}}}
	case AuthTerminal:
		return []map[string][]string{{"terminalToken": {}}}
	case AuthAPIToken:
		return []map[string][]string{{"apiToken": {}}}
	}
	return []map[string][]string{{"bearerAuth": {}}}
}

func description(r Route) string {
	var parts []string
	if r.Description != "" {
		parts = append(parts, r.Description)
	}
	if r.StepUp {
		parts = append(parts, "Step-up: a dashboard session must send the account `password` in the body; an API token must have been created with allow_dangerous (else 403).")
	}
	if r.SessionOnly {
		parts = append(parts, "Not available with an API token (403).")
	}
	if r.FullScope {
		parts = append(parts, "Needs an API token with scope full (read tokens get 403).")
	}
	switch r.Stream {
	case StreamText:
		parts = append(parts, "The response is a text/plain log streamed until the operation ends.")
	case StreamSSE:
		parts = append(parts, "The response is a Server-Sent Events stream that follows until the client disconnects.")
	}
	return strings.Join(parts, "\n\n")
}

func operation(r Route) map[string]any {
	op := map[string]any{
		"operationId": OperationID(r),
		"tags":        []string{r.Tag},
		"summary":     r.Summary,
		"security":    security(r),
	}
	if d := description(r); d != "" {
		op["description"] = d
	}
	var params []map[string]any
	for _, name := range PathParams(r.Path) {
		params = append(params, param("path", Param{Name: name, Type: "string"}))
	}
	for _, q := range r.Query {
		params = append(params, param("query", q))
	}
	for _, h := range r.Headers {
		params = append(params, param("header", h))
	}
	if len(params) > 0 {
		op["parameters"] = params
	}
	if schema := BodySchema(r); schema != nil {
		content := map[string]any{}
		switch r.BodyType {
		case BodyMultipart:
			content["multipart/form-data"] = map[string]any{"schema": schema}
		case BodyJSONOrMultipart:
			content["application/json"] = map[string]any{"schema": schema}
			content["multipart/form-data"] = map[string]any{"schema": schema}
		default:
			content["application/json"] = map[string]any{"schema": schema}
		}
		op["requestBody"] = map[string]any{"required": anyRequired(BodyFields(r)), "content": content}
	}
	ok := map[string]any{"description": "Success"}
	switch {
	case r.Stream != "":
		ok["content"] = map[string]any{r.Stream: map[string]any{"schema": map[string]any{"type": "string"}}}
	case r.Response != "":
		ok["content"] = map[string]any{r.Response: map[string]any{"schema": map[string]any{"type": "string"}}}
	default:
		ok["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{}}}
	}
	responses := map[string]any{"200": ok, "default": map[string]any{"$ref": "#/components/responses/Error"}}
	op["responses"] = responses
	op["x-docklift-auth"] = r.AuthKind()
	op["x-docklift-read-scope"] = r.ReadOnly()
	if r.StepUp {
		op["x-docklift-step-up"] = true
	}
	if r.Stream != "" {
		op["x-docklift-stream"] = r.Stream
	}
	if r.SessionOnly {
		op["x-docklift-session-only"] = true
	}
	return op
}

func anyRequired(fields []Field) bool {
	for _, f := range fields {
		if f.Required {
			return true
		}
	}
	return false
}

// Document is the OpenAPI 3.1 document of Routes.
func Document(version string) map[string]any {
	paths := map[string]any{}
	for _, r := range Routes {
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		item[strings.ToLower(r.Method)] = operation(r)
	}
	tags := make([]map[string]string, 0, len(Tags))
	for _, t := range Tags {
		tags = append(tags, map[string]string{"name": t.Name, "description": t.Description})
	}
	return map[string]any{
		"openapi": Version,
		"info": map[string]any{
			"title":   "Docklift API",
			"version": version,
			"description": "Every route of the Docklift server. Authenticate with `Authorization: Bearer <token>`: " +
				"a dashboard session JWT or an API token (`dl_...`, created under API & MCP in the dashboard or with " +
				"`docklift api-token create`). Read-scope tokens may only call GET routes. " +
				"Errors are JSON `{\"error\": \"...\"}`.",
		},
		"servers": []map[string]string{{"url": "/"}},
		"tags":    tags,
		"paths":   paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer",
					"description": "Session JWT (POST /api/auth/login) or API token (dl_...)."},
				"apiToken": map[string]any{"type": "http", "scheme": "bearer", "description": "API token (dl_...) only."},
				"sseToken": map[string]any{"type": "apiKey", "in": "query", "name": "token",
					"description": "5 minute token from POST /api/auth/sse-token."},
				"setupToken": map[string]any{"type": "apiKey", "in": "header", "name": "X-Setup-Token",
					"description": "Fresh-install restore token."},
				"terminalToken": map[string]any{"type": "apiKey", "in": "query", "name": "token",
					"description": "Single-use token from POST /api/auth/terminal-token."},
			},
			"responses": map[string]any{
				"Error": map[string]any{
					"description": "Error",
					"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"error": map[string]any{"type": "string"}},
					}}},
				},
			},
		},
	}
}

// Handler serves the document (GET /api/openapi.json). version names the
// running build.
func Handler(version func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, http.StatusOK, Document(version()))
	}
}

// Find returns the route matching method and a concrete path (path
// parameters match one segment), or false.
func Find(method, path string) (Route, bool) {
	path = strings.TrimSuffix(path, "/")
	segs := strings.Split(path, "/")
	best, bestParams := Route{}, -1
	for _, r := range Routes {
		if r.Method != method {
			continue
		}
		pat := strings.Split(r.Path, "/")
		if len(pat) != len(segs) {
			continue
		}
		match, params := true, 0
		for i := range pat {
			if strings.HasPrefix(pat[i], "{") {
				params++
				if segs[i] == "" {
					match = false
					break
				}
				continue
			}
			if pat[i] != segs[i] {
				match = false
				break
			}
		}
		// The most specific pattern wins, as in the router.
		if match && (bestParams < 0 || params < bestParams) {
			best, bestParams = r, params
		}
	}
	return best, bestParams >= 0
}
