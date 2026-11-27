package databases

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"docklift/internal/deploy"
	"docklift/internal/execx"
	"docklift/internal/store"
)

// Docker-backed tests run only with DOCKLIFT_DOCKER=1 and a reachable daemon
// (the Linux test server). They pull the official database images, start the
// database container the way a deploy would (data volume, credential env,
// Redis command), connect with the engine's own client over the injected
// connection URL from a container on the app network, and remove every
// container, volume and network they created.

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

func dockerCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// clientCheck is the command run in a throwaway container of the database
// image on the app network: it must authenticate with the connection URL
// (or its parts) and succeed.
func clientCheck(engine, url, user, password, database, host string) []string {
	switch engine {
	case deploy.EnginePostgres:
		return []string{"psql", url, "-tAc", "select 1"}
	case deploy.EngineMySQL:
		return []string{"mysql", "-h", host, "-u", user, "-p" + password, database, "-e", "select 1"}
	case deploy.EngineMariaDB:
		return []string{"mariadb", "-h", host, "-u", user, "-p" + password, database, "-e", "select 1"}
	case deploy.EngineRedis:
		return []string{"sh", "-c", `[ "$(redis-cli -u "$1" ping)" = PONG ]`, "check", url}
	case deploy.EngineMongoDB:
		return []string{"mongosh", url, "--quiet", "--eval", "db.runCommand({ping: 1}).ok"}
	}
	return nil
}

func TestDockerManagedDatabases(t *testing.T) {
	c := requireDocker(t)
	for _, engine := range deploy.EngineIDs {
		t.Run(engine, func(t *testing.T) {
			e := setup(t, nil)
			e.d.DockerX.Engine = c
			e.d.DockerX.Run = execx.Run
			ctx := context.Background()

			rec, raw := e.do(t, http.MethodPost, "/", map[string]any{"name": "it " + engine, "engine": engine})
			if rec.Code != 201 {
				t.Fatalf("create: %d %v", rec.Code, raw)
			}
			body := obj(t, raw)
			id := body["id"].(string)
			svc := obj(t, body["services"].([]any)[0])
			vol := obj(t, body["persistent_volumes"].([]any)[0])
			containerName, volumeName := svc["container_name"].(string), vol["name"].(string)
			image := body["image"].(string)
			t.Cleanup(func() {
				_ = exec.Command("docker", "rm", "-f", containerName).Run()
				_ = exec.Command("docker", "volume", "rm", "-f", volumeName).Run()
			})
			labels := dockerCLI(t, "volume", "inspect", "-f", "{{json .Labels}}", volumeName)
			for _, want := range []string{`"com.docklift.project":"` + id + `"`, `"com.docklift.role":"managed-database"`} {
				if !strings.Contains(labels, want) {
					t.Fatalf("volume labels %s lack %s", labels, want)
				}
			}

			// Start the container as the deploy runtime would: data volume,
			// credential env, no published port.
			rows, err := e.d.Store.ListEnvVariables(ctx, store.EnvFilter{ProjectID: &id}, store.OrderNone)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "-d", "--name", containerName, "-v", volumeName + ":" + vol["mount_path"].(string)}
			envMap := map[string]string{}
			for _, r := range rows {
				args = append(args, "-e", r.Key+"="+r.Value)
				envMap[r.Key] = r.Value
			}
			args = append(args, image)
			eng, _ := deploy.GetEngine(engine)
			creds, ok := deploy.CredentialsFromEnv(eng, envMap)
			if !ok {
				t.Fatal("credentials missing from env")
			}
			args = append(args, deploy.EngineCommand(eng, creds)...)
			dockerCLI(t, args...)
			if ports := dockerCLI(t, "port", containerName); ports != "" {
				t.Fatalf("managed database published host ports: %s", ports)
			}
			if _, err := e.d.Store.UpdateProject(ctx, id, store.ProjectUpdate{Status: store.Some(store.Ptr("running"))}); err != nil {
				t.Fatal(err)
			}

			// Link to an app whose project network exists.
			appP := seedApp(t, e.d, "it-app-"+engine, "app")
			network := deploy.ProjectNetworkName(appP.ID)
			dockerCLI(t, "network", "create", network)
			t.Cleanup(func() { _ = exec.Command("docker", "network", "rm", network).Run() })
			rec, raw = e.do(t, http.MethodPost, "/"+id+"/links", map[string]any{"app_project_id": appP.ID})
			link := obj(t, raw)
			if rec.Code != 201 || link["network_attached"] != true {
				t.Fatalf("link: %d %v", rec.Code, link)
			}
			networks := dockerCLI(t, "inspect", "-f", "{{json .NetworkSettings.Networks}}", containerName)
			if !strings.Contains(networks, `"`+network+`"`) {
				t.Fatalf("database not on app network: %s", networks)
			}
			injected, err := e.d.Store.FindEnvVariable(ctx, store.EnvFilter{ProjectID: &appP.ID, ServiceName: store.Ptr(""), Key: &eng.DefaultEnvKey})
			if err != nil {
				t.Fatal(err)
			}

			// Connect from the app network with the injected URL.
			check := clientCheck(engine, injected.Value, creds.Username, creds.Password, creds.Database, containerName)
			deadline := time.Now().Add(3 * time.Minute)
			for {
				out, err := exec.Command("docker", slices.Concat([]string{"run", "--rm", "--network", network, image}, check)...).CombinedOutput()
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("could not connect with the injected URL: %v\n%s", err, out)
				}
				time.Sleep(3 * time.Second)
			}

			// Unlink removes the env and detaches the database.
			rec, raw = e.do(t, http.MethodDelete, "/"+id+"/links/"+obj(t, link["link"])["id"].(string), nil)
			if rec.Code != 200 {
				t.Fatalf("unlink: %d %v", rec.Code, raw)
			}
			networks = dockerCLI(t, "inspect", "-f", "{{json .NetworkSettings.Networks}}", containerName)
			if strings.Contains(networks, `"`+network+`"`) {
				t.Fatalf("database still on app network after unlink: %s", networks)
			}
		})
	}
}
