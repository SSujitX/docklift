package logs

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

// Docker-backed tests run only with DOCKLIFT_DOCKER=1 and a reachable daemon
// (the Linux test server); they start throwaway alpine containers.

func requireDocker(t *testing.T) *client.Client {
	t.Helper()
	if os.Getenv("DOCKLIFT_DOCKER") != "1" {
		t.Skip("set DOCKLIFT_DOCKER=1 to run Docker integration tests")
	}
	c, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.Ping(t.Context(), client.PingOptions{}); err != nil {
		t.Fatalf("docker unreachable: %v", err)
	}
	return c
}

func runContainer(t *testing.T, tty bool) string {
	t.Helper()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "docklift-logs-test-" + hex.EncodeToString(b)
	args := []string{"run", "-d", "--name", name}
	if tty {
		args = append(args, "-t")
	}
	args = append(args, "alpine:3.20", "sh", "-c", "echo to-stdout; echo to-stderr >&2; sleep 120")
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	return name
}

func readEvents(t *testing.T, url string, n int) []map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	var out []map[string]any
	for len(out) < n {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("after %v: %v", out, err)
		}
		data, ok := strings.CutPrefix(strings.TrimSpace(line), "data: ")
		if !ok {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestDockerStreamDemultiplexes(t *testing.T) {
	c := requireDocker(t)
	for _, tty := range []bool{false, true} {
		name := runContainer(t, tty)
		time.Sleep(time.Second)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Stream(w, r, c, name, "200")
		}))
		ev := readEvents(t, srv.URL, 3)
		srv.Close()
		if ev[0]["type"] != "connected" || ev[0]["container"] != name {
			t.Fatalf("tty=%v first event %v", tty, ev[0])
		}
		joined := ev[1]["message"].(string) + ev[2]["message"].(string)
		if !strings.Contains(joined, "to-stdout") || !strings.Contains(joined, "to-stderr") {
			t.Fatalf("tty=%v log events %v", tty, ev[1:])
		}
		if strings.ContainsAny(ev[1]["message"].(string)[:1], "\x01\x02") {
			t.Fatalf("tty=%v multiplex header leaked: %q", tty, ev[1]["message"])
		}
		status, isRunning, err := ContainerStatus(t.Context(), c, name)
		if err != nil || status != "running" || !isRunning {
			t.Fatalf("status %q %v %v", status, isRunning, err)
		}
	}
	if status, _, err := ContainerStatus(t.Context(), c, "docklift-logs-test-missing-xyz"); err != nil || status != "not_found" {
		t.Fatalf("missing container: %q %v", status, err)
	}
}
