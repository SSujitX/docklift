package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// backendDist is the compiled TypeScript backend used as the reference
// implementation by the parity tests.
var backendDist = filepath.Join("..", "..", "..", "backend", "dist")

// runNode runs an ES module script with node and returns its stdout. The
// script finds the JSON-encoded input at process.env.PARITY_INPUT (a file
// path) and the dist directory URL at process.env.PARITY_DIST. The test is
// skipped when node or backend/dist is unavailable.
func runNode(t *testing.T, script string, input any) []byte {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available; TS parity not checked")
	}
	dist, err := filepath.Abs(backendDist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dist, "services", "compose.js")); err != nil {
		t.Skip("backend/dist not built; TS parity not checked")
	}
	in, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	inPath := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(inPath, in, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--input-type=module", "-e", script)
	cmd.Env = append(os.Environ(), "PARITY_INPUT="+inPath, "PARITY_DIST="+fileURL(dist))
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("node: %v\n%s", err, stderr)
	}
	return out
}

func decodeJSON(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode node output: %v\n%s", err, b)
	}
}

func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if p[0] != '/' {
		p = "/" + p
	}
	return "file://" + p
}

// nodePrelude loads the input and exposes load(module) for dist modules.
const nodePrelude = `
import fs from 'fs';
const input = JSON.parse(fs.readFileSync(process.env.PARITY_INPUT, 'utf8'));
const load = (m) => import(process.env.PARITY_DIST + '/' + m);
const done = (v) => { process.stdout.write(JSON.stringify(v)); process.exit(0); };
`
