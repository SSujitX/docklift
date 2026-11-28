package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpapi "docklift/internal/api/mcp"
	"docklift/internal/api/openapi"
)

// repoRoot is the repository root seen from server/cmd/gen-docs.
const repoRoot = "../../.."

// TestGeneratedDocsUpToDate fails when a committed generated page differs
// from what the generator writes now: the route table or the MCP tools
// changed without `go run ./cmd/gen-docs`, or the page was edited by hand.
func TestGeneratedDocsUpToDate(t *testing.T) {
	outs, err := Outputs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range outs {
		path := filepath.Join(repoRoot, filepath.FromSlash(out.Path))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v (run `go run ./cmd/gen-docs` in server/)", out.Path, err)
			continue
		}
		if bytes.Equal(got, out.Content) {
			continue
		}
		if bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), out.Content) {
			t.Errorf("%s has CRLF line endings; it must be LF (see .gitattributes, then re-checkout or run `go run ./cmd/gen-docs`)", out.Path)
			continue
		}
		t.Errorf("%s is out of date: run `go run ./cmd/gen-docs` in server/ and commit the result", out.Path)
	}
}

func TestSpliceToolSection(t *testing.T) {
	guide := "# T\r\n\r\nintro\r\n\r\n## Tool reference\r\n\r\nold\r\n\r\n### old tool\r\n\r\n## Examples\r\n\r\nkept\r\n"
	got, err := SpliceToolSection([]byte(guide), []byte("NEW\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# T\n\nintro\n\n## Tool reference\n\nNEW\n\n## Examples\n\nkept\n"
	if string(got) != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// Idempotent, and the section may be last.
	if again, _ := SpliceToolSection(got, []byte("NEW\n")); string(again) != want {
		t.Fatalf("not idempotent: %q", again)
	}
	if last, _ := SpliceToolSection([]byte("# T\n\n## Tool reference\n\nold\n"), []byte("NEW\n")); string(last) != "# T\n\n## Tool reference\n\nNEW\n" {
		t.Fatalf("last section: %q", last)
	}
	if _, err := SpliceToolSection([]byte("# T\n"), nil); err == nil {
		t.Fatal("missing heading accepted")
	}
}

func TestOutputIsLFAndComplete(t *testing.T) {
	ref := string(APIReference())
	if strings.Contains(ref, "\r") {
		t.Fatal("api reference contains CR")
	}
	anchors := map[string]bool{}
	for _, t := range openapi.Tags {
		anchors[anchor(t.Name)] = true
	}
	for _, r := range openapi.Routes {
		if a := routeAnchor(r); anchors[a] {
			t.Errorf("duplicate heading id %s (%s %s)", a, r.Method, r.Path)
		} else {
			anchors[a] = true
		}
		if !strings.Contains(ref, "### `"+r.Method+" "+r.Path+"` {#"+routeAnchor(r)+"}") {
			t.Errorf("api reference misses %s %s", r.Method, r.Path)
		}
	}
	tools := string(MCPTools())
	for _, d := range mcpapi.ToolDefs() {
		if !strings.Contains(tools, "### `"+d.Name+"`") {
			t.Errorf("tool reference misses %s", d.Name)
		}
	}
	// Spot checks of the markers against the routes the handlers guard.
	for _, tc := range []struct{ heading, marker string }{
		{"### `POST /api/system/reboot`", markDangerous},
		{"### `POST /api/tokens`", markSessionOnly},
		{"### `POST /api/deployments/{projectId}/deploy`", markStreams},
		{"### `GET /api/backup/download/{filename}`", markNoRead},
	} {
		i := strings.Index(ref, tc.heading)
		if i < 0 {
			t.Fatalf("missing %s", tc.heading)
		}
		section := ref[i:]
		if j := strings.Index(section[1:], "\n### "); j > 0 {
			section = section[:j]
		}
		if !strings.Contains(section, "**Markers:**") || !strings.Contains(section, tc.marker) {
			t.Errorf("%s is not marked %q:\n%s", tc.heading, tc.marker, section)
		}
	}
}
