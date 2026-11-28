// Command gen-docs writes the generated pages of the docs site from the Go
// source of truth, so they cannot drift from the code:
//
//   - website/guide/api-reference.md from the route table
//     (internal/api/openapi/routes.go);
//   - the "## Tool reference" section of website/guide/mcp.md (everything
//     from that heading to the next level-2 heading) from the tool
//     definitions (internal/api/mcp/tools.go). The rest of mcp.md is written
//     by hand. (A VitePress include would be simpler, but VitePress wraps it
//     in HTML comments that render as text with markdown.html off.)
//
// Run it from server/ after changing either:
//
//	go run ./cmd/gen-docs
//
// The test in this package fails when a committed page differs from the
// output. Output always uses LF line endings; .gitattributes pins both files
// to LF so a Windows checkout (core.autocrlf=true) does not turn them into
// CRLF and break the comparison.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpapi "docklift/internal/api/mcp"
	"docklift/internal/api/openapi"
)

// Generated page paths, relative to the repository root.
const (
	APIReferencePath = "website/guide/api-reference.md"
	MCPGuidePath     = "website/guide/mcp.md"
	// toolSectionHeading starts the generated section of MCPGuidePath.
	toolSectionHeading = "## Tool reference"
)

func main() {
	root := flag.String("root", "..", "repository root (default: the parent of server/)")
	flag.Parse()
	outs, err := Outputs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-docs:", err)
		os.Exit(1)
	}
	for _, out := range outs {
		path := filepath.Join(*root, filepath.FromSlash(out.Path))
		if err := os.WriteFile(path, out.Content, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gen-docs:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", out.Path)
	}
}

// Output is one generated file.
type Output struct {
	Path    string
	Content []byte
}

// Outputs returns every generated file as it must be on disk. The MCP guide
// is read from root and only its tool section is replaced.
func Outputs(root string) ([]Output, error) {
	guide, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(MCPGuidePath)))
	if err != nil {
		return nil, err
	}
	spliced, err := SpliceToolSection(guide, MCPTools())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", MCPGuidePath, err)
	}
	return []Output{
		{APIReferencePath, APIReference()},
		{MCPGuidePath, spliced},
	}, nil
}

// SpliceToolSection replaces the body of the "## Tool reference" section of
// guide (the lines after the heading, up to the next level-2 heading or the
// end) with section. guide is read with LF or CRLF endings and returned
// with LF.
func SpliceToolSection(guide, section []byte) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(string(guide), "\r\n", "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " ") == toolSectionHeading {
			if start >= 0 {
				return nil, errors.New("more than one " + toolSectionHeading + " heading")
			}
			start = i
		}
	}
	if start < 0 {
		return nil, errors.New("no " + toolSectionHeading + " heading")
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	var b strings.Builder
	b.WriteString(strings.Join(lines[:start+1], "\n"))
	b.WriteString("\n\n")
	b.Write(bytes.TrimRight(section, "\n"))
	b.WriteString("\n")
	if end < len(lines) {
		b.WriteString("\n")
		b.WriteString(strings.Join(lines[end:], "\n"))
	}
	return append(bytes.TrimRight([]byte(b.String()), "\n"), '\n'), nil
}

// writer collects Markdown lines, always joined with LF.
type writer struct{ b bytes.Buffer }

func (w *writer) line(format string, a ...any) {
	if len(a) == 0 {
		w.b.WriteString(format)
	} else {
		fmt.Fprintf(&w.b, format, a...)
	}
	w.b.WriteByte('\n')
}

func (w *writer) blank() { w.b.WriteByte('\n') }

// bytes returns the text with exactly one trailing newline.
func (w *writer) bytes() []byte {
	return append(bytes.TrimRight(w.b.Bytes(), "\n"), '\n')
}

// cell makes text safe inside a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// oneLine joins paragraphs into one line (list items, table cells).
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// anchor is the explicit heading id of a tag.
func anchor(tag string) string { return strings.ReplaceAll(strings.ToLower(tag), " ", "-") }

// routeAnchor is the heading id of a route: its OpenAPI operationId with
// hyphens and lower case, because VitePress slugifies link fragments that
// way ("_" becomes "-", letters are lowered).
func routeAnchor(r openapi.Route) string {
	return strings.ToLower(strings.ReplaceAll(openapi.OperationID(r), "_", "-"))
}

// Marker labels, explained at the top of the reference.
const (
	markDangerous   = "dangerous"
	markSessionOnly = "session only"
	markStreams     = "streams"
	markNoRead      = "no read tokens"
)

// notForReadTokens reports a route a read-scope API token is refused on:
// every route behind the session guard that is not a plain GET, plus the
// GET routes marked FullScope. Public, webhook and token-only routes do not
// check the scope (the MCP endpoint checks it on each inner request), and
// session-only routes refuse every token anyway.
func notForReadTokens(r openapi.Route) bool {
	switch r.AuthKind() {
	case openapi.AuthPublic, openapi.AuthHMAC, openapi.AuthAPIToken, openapi.AuthTerminal:
		return false
	}
	return !r.SessionOnly && !r.ReadOnly()
}

func markers(r openapi.Route) []string {
	var m []string
	if r.StepUp {
		m = append(m, markDangerous)
	}
	if r.SessionOnly {
		m = append(m, markSessionOnly)
	}
	if r.Stream != "" {
		m = append(m, markStreams)
	}
	if notForReadTokens(r) {
		m = append(m, markNoRead)
	}
	return m
}

func authText(r openapi.Route) string {
	switch r.AuthKind() {
	case openapi.AuthPublic:
		return "none"
	case openapi.AuthSSE:
		return "`Authorization: Bearer` API token, or `?token=` (5 minute SSE token) for a dashboard session"
	case openapi.AuthSetupOrSession:
		return "`Authorization: Bearer` session or API token; before the first admin exists, the `X-Setup-Token` header instead"
	case openapi.AuthHMAC:
		return "GitHub webhook signature (`X-Hub-Signature-256`)"
	case openapi.AuthTerminal:
		return "single-use terminal token (`?token=` from `POST /api/auth/terminal-token`)"
	case openapi.AuthAPIToken:
		return "`Authorization: Bearer dl_...` API token only"
	}
	return "`Authorization: Bearer` session or API token"
}

func bodyKind(r openapi.Route) string {
	switch r.BodyType {
	case openapi.BodyMultipart:
		return "multipart/form-data"
	case openapi.BodyJSONOrMultipart:
		return "JSON or multipart/form-data"
	}
	return "JSON"
}

// item renders one parameter or body field as a list item.
func item(name, typ string, required bool, enum []string, desc string) string {
	s := "`" + name + "`"
	var meta []string
	if typ != "" {
		meta = append(meta, "`"+typ+"`")
	}
	if required {
		meta = append(meta, "required")
	}
	if len(meta) > 0 {
		s += " (" + strings.Join(meta, ", ") + ")"
	}
	var text []string
	if desc != "" {
		text = append(text, oneLine(desc))
	}
	if len(enum) > 0 {
		text = append(text, "One of "+codeList(enum)+".")
	}
	if len(text) > 0 {
		s += ": " + strings.Join(text, " ")
	}
	return "  - " + s
}

func codeList(values []string) string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = "`" + v + "`"
	}
	return strings.Join(out, ", ")
}

// routesByTag groups the table by tag, in Tags order, keeping table order.
func routesByTag() map[string][]openapi.Route {
	out := map[string][]openapi.Route{}
	for _, r := range openapi.Routes {
		out[r.Tag] = append(out[r.Tag], r)
	}
	return out
}

// APIReference renders website/guide/api-reference.md.
func APIReference() []byte {
	var w writer
	groups := routesByTag()
	w.line("---")
	w.line("outline: 2")
	w.line("---")
	w.blank()
	w.line("# Full API Reference")
	w.blank()
	w.line("::: info Generated page")
	w.line("This page is generated from the route table in `server/internal/api/openapi/routes.go`. Do not edit it by hand: change the table, then run `go run ./cmd/gen-docs` in `server/`. A test fails when this page and the table differ.")
	w.line(":::")
	w.blank()
	w.line("Every HTTP route of the Docklift server: **%d routes** in %d groups. The running server serves the same data as an OpenAPI 3.1 document at `GET /api/openapi.json`. Authentication, token scopes, errors and worked `curl` examples are in the [API guide](/guide/api).", len(openapi.Routes), len(openapi.Tags))
	w.blank()
	w.line("Paths are relative to the panel URL (for example `https://panel.example.com`). Path parameters are written `{name}` and are always required. Bodies are JSON unless noted. Errors are JSON `{\"error\": \"...\"}`.")
	w.blank()
	w.line("## Markers")
	w.blank()
	w.line("| Marker | Meaning |")
	w.line("|---|---|")
	w.line("| **%s** | Step-up action. A dashboard session must send the account `password` in the body; an API token must have been created with **allow dangerous actions** (else 403). |", markDangerous)
	w.line("| **%s** | Refuses every API token with 403. Use a dashboard session. |", markSessionOnly)
	w.line("| **%s** | The response is a stream: a `text/plain` progress log until the operation ends, or Server-Sent Events (`text/event-stream`) that follow until the client disconnects. Use `curl -N`. |", markStreams)
	w.line("| **%s** | A read-scope API token gets 403. Every write method behind the login, plus the GET routes marked so. |", markNoRead)
	w.blank()
	w.line("## Groups")
	w.blank()
	w.line("| Group | Routes | Description |")
	w.line("|---|---|---|")
	for _, t := range openapi.Tags {
		w.line("| [%s](#%s) | %d | %s |", t.Name, anchor(t.Name), len(groups[t.Name]), cell(t.Description))
	}
	for _, t := range openapi.Tags {
		routes := groups[t.Name]
		if len(routes) == 0 {
			continue
		}
		w.blank()
		w.line("## %s {#%s}", t.Name, anchor(t.Name))
		w.blank()
		w.line("%s", t.Description)
		w.blank()
		w.line("| Method | Path | Summary | Markers |")
		w.line("|---|---|---|---|")
		for _, r := range routes {
			w.line("| `%s` | [`%s`](#%s) | %s | %s |", r.Method, r.Path, routeAnchor(r), cell(r.Summary), cell(strings.Join(markers(r), ", ")))
		}
		for _, r := range routes {
			route(&w, r)
		}
	}
	return w.bytes()
}

func route(w *writer, r openapi.Route) {
	w.blank()
	w.line("### `%s %s` {#%s}", r.Method, r.Path, routeAnchor(r))
	w.blank()
	w.line("%s.", strings.TrimSuffix(r.Summary, "."))
	if r.Description != "" {
		w.blank()
		w.line("%s", oneLine(r.Description))
	}
	w.blank()
	w.line("- **Auth:** %s", authText(r))
	if m := markers(r); len(m) > 0 {
		w.line("- **Markers:** %s", strings.Join(m, ", "))
	}
	if names := openapi.PathParams(r.Path); len(names) > 0 {
		w.line("- **Path parameters:** %s", codeList(names))
	}
	if len(r.Query) > 0 {
		w.line("- **Query parameters:**")
		for _, q := range r.Query {
			w.line("%s", item(q.Name, q.Type, q.Required, q.Enum, q.Description))
		}
	}
	if len(r.Headers) > 0 {
		w.line("- **Headers:**")
		for _, h := range r.Headers {
			w.line("%s", item(h.Name, h.Type, h.Required, h.Enum, h.Description))
		}
	}
	if fields := openapi.BodyFields(r); len(fields) > 0 {
		w.line("- **Body** (%s):", bodyKind(r))
		for _, f := range fields {
			w.line("%s", item(f.Name, f.Type, f.Required, f.Enum, f.Description))
		}
	}
	switch {
	case r.Stream != "":
		w.line("- **Response:** streamed `%s`", r.Stream)
	case r.Response != "":
		w.line("- **Response:** `%s`", r.Response)
	}
}

// apiRequestTool is the generic tool whose scope depends on the endpoint.
const apiRequestTool = "docklift_api_request"

// toolScope is the API token scope a tool needs.
func toolScope(t *mcp.Tool) string {
	switch {
	case t.Name == apiRequestTool:
		return "read for `GET`, full otherwise; **dangerous** endpoints need allow dangerous actions"
	case t.Annotations != nil && t.Annotations.ReadOnlyHint:
		return "read"
	}
	return "full"
}

// shortScope is toolScope for the summary table.
func shortScope(t *mcp.Tool) string {
	if t.Name == apiRequestTool {
		return "depends on the endpoint"
	}
	return toolScope(t)
}

// schemaProps returns the properties and required names of a tool's input
// schema (the map tools.go builds).
func schemaProps(t *mcp.Tool) (map[string]map[string]any, []string) {
	s, _ := t.InputSchema.(map[string]any)
	props := map[string]map[string]any{}
	if raw, ok := s["properties"].(map[string]any); ok {
		for k, v := range raw {
			p, _ := v.(map[string]any)
			props[k] = p
		}
	}
	req, _ := s["required"].([]string)
	return props, req
}

// argOrder is the required arguments in schema order, then the rest by name.
func argOrder(props map[string]map[string]any, required []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range required {
		if _, ok := props[r]; ok {
			out = append(out, r)
			seen[r] = true
		}
	}
	var rest []string
	for k := range props {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// MCPTools renders the body of the tool section of website/guide/mcp.md.
func MCPTools() []byte {
	var w writer
	defs := mcpapi.ToolDefs()
	found := false
	for _, t := range defs {
		found = found || t.Name == apiRequestTool
	}
	if !found {
		panic("gen-docs: tool " + apiRequestTool + " no longer exists; update toolScope")
	}
	w.line("::: info Generated section")
	w.line("This section is generated from `server/internal/api/mcp/tools.go` (%d tools). Do not edit it by hand: run `go run ./cmd/gen-docs` in `server/`. The rest of this page is written by hand.", len(defs))
	w.line(":::")
	w.blank()
	w.line("Every tool call becomes one or more ordinary API requests (see [How it is enforced](#how-it-is-enforced)), so **Scope** is the API token scope those requests need. Tools that follow streamed output accept `waitSeconds` (default 600, max 1800).")
	w.blank()
	w.line("| Tool | Scope | What it does |")
	w.line("|---|---|---|")
	for _, t := range defs {
		w.line("| [`%s`](#%s) | %s | %s |", t.Name, strings.ReplaceAll(t.Name, "_", "-"), cell(shortScope(t)), cell(oneLine(t.Description)))
	}
	for _, t := range defs {
		w.blank()
		w.line("### `%s` {#%s}", t.Name, strings.ReplaceAll(t.Name, "_", "-"))
		w.blank()
		w.line("%s", oneLine(t.Description))
		w.blank()
		w.line("- **Scope:** %s", toolScope(t))
		if t.Annotations != nil && t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint {
			w.line("- **Destructive:** yes (clients may ask you to confirm)")
		}
		props, required := schemaProps(t)
		if len(props) == 0 {
			w.line("- **Arguments:** none")
			continue
		}
		req := map[string]bool{}
		for _, r := range required {
			req[r] = true
		}
		w.line("- **Arguments:**")
		for _, name := range argOrder(props, required) {
			p := props[name]
			typ, _ := p["type"].(string)
			desc, _ := p["description"].(string)
			var enum []string
			if e, ok := p["enum"].([]string); ok {
				enum = e
			}
			w.line("%s", item(name, typ, req[name], enum, desc))
		}
	}
	return w.bytes()
}
