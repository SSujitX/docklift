// Package middlewarebypass patches Next.js middleware host allowlists so a
// deployed app answers on localhost, the server IP and its domains (the port
// of backend/src/lib/middlewareBypass.ts, used by the deploy pipeline).
package middlewarebypass

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// allowedHostsRe matches `const allowedHosts = [ ... ]`.
var allowedHostsRe = regexp.MustCompile(`const\s+allowedHosts\s*=\s*\[([\s\S]*?)\]`)

// quotedRe extracts quoted strings from the array body.
var quotedRe = regexp.MustCompile(`["']([^"']+)["']`)

// skipDirs are never searched for middleware files.
var skipDirs = []string{"node_modules", ".git", ".next", "dist", "build", ".cache"}

// maxDepth bounds the directory search below the project root.
const maxDepth = 3

// Options describes the deployment whose middleware is patched.
type Options struct {
	ProjectPath string
	Port        int
	Domains     []string
	// ServerIP overrides the detected first non-loopback IPv4 address.
	ServerIP string
}

// Result reports what Patch found and did.
type Result struct {
	Found         bool
	Patched       bool
	FilePath      string
	OriginalHosts []string
	AddedHosts    []string
	Error         string
}

// ServerIP returns the first non-loopback IPv4 address, or "localhost".
func ServerIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "localhost"
}

// findMiddlewareFiles lists middleware.ts/middleware.js files up to maxDepth
// directories below dir; unreadable directories are skipped.
func findMiddlewareFiles(dir string, depth int) []string {
	if depth > maxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		switch {
		case e.IsDir():
			if !slices.Contains(skipDirs, e.Name()) {
				out = append(out, findMiddlewareFiles(full, depth+1)...)
			}
		case e.Name() == "middleware.ts" || e.Name() == "middleware.js":
			out = append(out, full)
		}
	}
	return out
}

func parseHosts(arrayBody string) []string {
	var hosts []string
	for _, m := range quotedRe.FindAllStringSubmatch(arrayBody, -1) {
		hosts = append(hosts, m[1])
	}
	return hosts
}

// Patch adds the local hosts and domains to the first middleware file that
// declares `const allowedHosts = [...]`, writing a `.bak` copy first.
func Patch(opts Options) Result {
	serverIP := opts.ServerIP
	if serverIP == "" {
		serverIP = ServerIP()
	}
	files := findMiddlewareFiles(opts.ProjectPath, 0)
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return Result{Found: true, FilePath: path, Error: err.Error()}
		}
		loc := allowedHostsRe.FindSubmatchIndex(content)
		if loc == nil {
			continue
		}
		original := parseHosts(string(content[loc[2]:loc[3]]))
		port := strconv.Itoa(opts.Port)
		candidates := []string{"localhost", "localhost:" + port, serverIP, serverIP + ":" + port}
		for _, d := range opts.Domains {
			if d = strings.TrimSpace(d); d != "" {
				candidates = append(candidates, d)
			}
		}
		added := []string{}
		for _, h := range candidates {
			if !slices.Contains(original, h) {
				added = append(added, h)
			}
		}
		if len(added) == 0 {
			return Result{Found: true, FilePath: path, OriginalHosts: original, AddedHosts: added}
		}
		if err := os.WriteFile(path+".bak", content, 0o644); err != nil {
			return Result{Found: true, FilePath: path, Error: err.Error()}
		}
		quoted := make([]string, 0, len(original)+len(added))
		for _, h := range append(append([]string(nil), original...), added...) {
			quoted = append(quoted, `"`+h+`"`)
		}
		patched := string(content[:loc[0]]) + "const allowedHosts = [" + strings.Join(quoted, ", ") + "]" + string(content[loc[1]:])
		if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
			return Result{Found: true, FilePath: path, Error: err.Error()}
		}
		return Result{Found: true, Patched: true, FilePath: path, OriginalHosts: original, AddedHosts: added}
	}
	return Result{}
}

// Log writes the outcome to a deployment log stream (nothing when no
// middleware was found).
func Log(r Result, writeLog func(string)) {
	if !r.Found {
		return
	}
	writeLog("\n🔐 Middleware Host Check Detected\n")
	writeLog(fmt.Sprintf("   File: %s\n", r.FilePath))
	if r.Error != "" {
		writeLog(fmt.Sprintf("   ⚠️  Error patching: %s\n", r.Error))
		return
	}
	if !r.Patched {
		writeLog("   ✓ Already has required hosts\n")
		return
	}
	writeLog(fmt.Sprintf("   📋 Original hosts: %s\n", strings.Join(r.OriginalHosts, ", ")))
	writeLog(fmt.Sprintf("   ➕ Added hosts: %s\n", strings.Join(r.AddedHosts, ", ")))
	writeLog("   ✅ Middleware patched for local access\n")
	writeLog(fmt.Sprintf("   💾 Backup created: %s.bak\n", r.FilePath))
}
