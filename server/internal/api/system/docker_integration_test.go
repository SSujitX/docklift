package system

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"

	"github.com/moby/moby/client"
)

// TestDockerPurgeInUseIncludesStoppedContainers runs only with
// DOCKLIFT_DOCKER=1 and a reachable daemon (the Linux test server): the
// image of a stopped, untracked container must count as in use.
func TestDockerPurgeInUseIncludesStoppedContainers(t *testing.T) {
	if os.Getenv("DOCKLIFT_DOCKER") != "1" {
		t.Skip("set DOCKLIFT_DOCKER=1 to run Docker integration tests")
	}
	c, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "docklift-purge-test-" + hex.EncodeToString(b)
	if out, err := exec.Command("docker", "create", "--name", name, "alpine:3.20", "true").CombinedOutput(); err != nil {
		t.Fatalf("docker create: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })

	h := &handlers{docker: c}
	inUse := map[string]bool{}
	if err := h.addContainerImages(t.Context(), inUse); err != nil {
		t.Fatal(err)
	}
	info, err := c.ContainerInspect(t.Context(), name, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !inUse["alpine:3.20"] || !inUse[info.Container.Image] {
		t.Fatalf("in-use refs %v miss alpine:3.20 / %s", inUse, info.Container.Image)
	}
}
