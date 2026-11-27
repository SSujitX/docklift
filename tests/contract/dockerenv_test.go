package contract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// With DOCKLIFT_DOCKER=1 the booted servers drive the caller's Docker
// daemon. Deploys attach the edge proxy (NGINX_PROXY_CONTAINER) to each
// project network and fail when it is missing, exactly as in production, so
// the harness runs a stand-in nginx-proxy configured like docker-compose.yml
// (the main instance's conf dir as conf.d, the repo's nginx.conf and
// snippets). At the end it removes every Docker object the run created.

const (
	contractProxy   = "docklift-contract-nginx-proxy"
	contractNetwork = "docklift_contract_network"
	edgeImage       = "nginx:stable-alpine"
)

func dockerCmd(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// startEdgeProxy runs the stand-in proxy for the main instance.
func startEdgeProxy(inst *Instance) error {
	if !cfg.docker || !inst.Booted() {
		return nil
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err
	}
	_, _ = dockerCmd("rm", "-f", contractProxy)
	if _, err := dockerCmd("network", "inspect", contractNetwork); err != nil {
		if out, err := dockerCmd("network", "create", contractNetwork); err != nil {
			return fmt.Errorf("create %s: %v: %s", contractNetwork, err, out)
		}
	}
	webroot := filepath.Join(inst.Root, "certbot-www")
	if err := os.MkdirAll(webroot, 0o755); err != nil {
		return err
	}
	out, err := dockerCmd("run", "-d", "--name", contractProxy, "--network", contractNetwork,
		"-v", inst.Dirs.Nginx+":/etc/nginx/conf.d:ro",
		"-v", filepath.Join(repo, "nginx-proxy", "nginx.conf")+":/etc/nginx/nginx.conf:ro",
		"-v", filepath.Join(repo, "nginx-proxy", "snippets")+":/etc/nginx/snippets:ro",
		"-v", webroot+":/var/www/certbot:ro",
		"-v", inst.Dirs.LetsEncrypt+":/etc/letsencrypt:ro",
		edgeImage)
	if err != nil {
		return fmt.Errorf("start %s: %v: %s", contractProxy, err, out)
	}
	return nil
}

var uuidDir = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// runProjectIDs are the projects of every booted instance: each has a
// directory under DEPLOYMENTS_PATH and/or a runtime compose under
// DEPLOYMENTS_PATH/.docklift.
func runProjectIDs() []string {
	instMu.Lock()
	list := append([]*Instance(nil), allInstances...)
	instMu.Unlock()
	seen := map[string]bool{}
	var ids []string
	for _, inst := range list {
		if !inst.Booted() {
			continue
		}
		for _, dir := range []string{inst.Dirs.Deployments, filepath.Join(inst.Dirs.Deployments, ".docklift")} {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.IsDir() && uuidDir.MatchString(e.Name()) && !seen[e.Name()] {
					seen[e.Name()] = true
					ids = append(ids, e.Name())
				}
			}
		}
	}
	return ids
}

// cleanupDocker removes the containers, networks, volumes and images of the
// run's projects (by their exact project label and id-derived names), then
// the stand-in proxy and the control network.
func cleanupDocker() {
	if !cfg.docker || cfg.target == "remote" {
		return
	}
	for _, id := range runProjectIDs() {
		short := id[:8]
		label := "label=com.docklift.project=" + id
		if out, _ := dockerCmd("ps", "-aq", "--filter", label); out != "" {
			_, _ = dockerCmd(append([]string{"rm", "-f", "-v"}, strings.Fields(out)...)...)
		}
		_, _ = dockerCmd("network", "disconnect", "-f", "dl-net-"+short, contractProxy)
		if out, _ := dockerCmd("network", "ls", "-q", "--filter", label); out != "" {
			_, _ = dockerCmd(append([]string{"network", "rm"}, strings.Fields(out)...)...)
		}
		_, _ = dockerCmd("network", "rm", "dl-net-"+short)
		if out, _ := dockerCmd("volume", "ls", "-q", "--filter", label); out != "" {
			_, _ = dockerCmd(append([]string{"volume", "rm", "-f"}, strings.Fields(out)...)...)
		}
		if out, _ := dockerCmd("images", "-q", "--filter", "reference=docklift-"+short+"-*"); out != "" {
			_, _ = dockerCmd(append([]string{"rmi", "-f"}, strings.Fields(out)...)...)
		}
	}
	_, _ = dockerCmd("rm", "-f", "-v", contractProxy)
	_, _ = dockerCmd("network", "rm", contractNetwork)
}
