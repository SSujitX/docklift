// Package mcpapi serves /mcp: a Model Context Protocol server (Streamable
// HTTP transport, stateless, JSON responses) built on the official Go SDK.
// It authenticates with a Docklift API token only, and every tool call is
// one or more ordinary requests sent through the real router in process
// with the caller's token and client address, so scope, allow_dangerous,
// validation, rate limits and audit logging apply exactly as over HTTP.
package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"docklift/internal/app"
	"docklift/internal/httpx"
	"docklift/internal/version"
)

// Path is the MCP endpoint.
const Path = "/mcp"

// MaxBodyBytes bounds one JSON-RPC message.
const MaxBodyBytes = 4 << 20

// Instructions are sent to clients on initialize / server/discover.
const Instructions = `Docklift is a self-hosted Docker PaaS. These tools act on this Docklift server as the API token you connected with.
- Start with docklift_list_projects; project ids come from there.
- docklift_deploy builds and starts a project and returns the build log tail.
- For anything without a dedicated tool, find the endpoint with docklift_list_endpoints and call it with docklift_api_request.
- A read-scope token can only read. Actions that ask for the password in the dashboard (rollback, restore, reboot, purge, shell commands) need a token created with "allow dangerous actions".`

// New returns the /mcp handler. handler returns the root HTTP handler the
// tool calls are dispatched through (it is created after this one).
func New(d *app.Deps, handler func() http.Handler) http.Handler {
	disp := &Dispatcher{Handler: handler}
	list := tools()
	sdk := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "docklift", Title: "Docklift", Version: version.Version},
			&mcp.ServerOptions{Instructions: Instructions})
		for _, t := range list {
			run := t.run
			s.AddTool(t.def, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				a := args{}
				if raw := req.Params.Arguments; len(raw) > 0 && string(raw) != "null" {
					dec := json.NewDecoder(bytes.NewReader(raw))
					dec.UseNumber()
					if err := dec.Decode(&a); err != nil {
						return textResult("Invalid arguments: "+err.Error(), true), nil
					}
				}
				res, err := run(&call{ctx: ctx, outer: r, d: disp}, a)
				if err != nil {
					return textResult(err.Error(), true), nil
				}
				return res, nil
			})
		}
		return s
	}, &mcp.StreamableHTTPOptions{
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: MaxBodyBytes,
		// Origin is checked below with the dashboard's own trusted-origin
		// rule; Host-based localhost protection would refuse a panel
		// reached through a local proxy.
		DisableLocalhostProtection: true,
	})
	allow := append(append([]string(nil), d.Config.CORSOrigins...), d.Config.FrontendURL)
	authed := d.Auth.RequireAPIToken(sdk)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// DNS-rebinding protection required by the transport: a browser
		// Origin must be the dashboard itself or an allowlisted origin.
		if origin := r.Header.Get("Origin"); origin != "" && !httpx.IsTrustedOrigin(origin, r, httpx.RequestProtocol(r), allow) {
			d.Log.Warn("[SECURITY] Rejected MCP request from untrusted Origin: " + origin)
			httpx.JSON(w, http.StatusForbidden, rpcError{JSONRPC: "2.0", Error: rpcErr{Code: -32600, Message: "Origin not allowed"}})
			return
		}
		// The token is checked before the body is read.
		authed.ServeHTTP(w, r)
	})
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcError struct {
	JSONRPC string `json:"jsonrpc"`
	Error   rpcErr `json:"error"`
}
