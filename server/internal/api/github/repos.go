package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"docklift/internal/githubapp"
	"docklift/internal/httpx"
)

// maxParallelInstallations bounds concurrent per-installation repository
// fetches in GET /repos.
const maxParallelInstallations = 8

// maxTagPages is how many pages of 100 tags GET /tags reads.
const maxTagPages = 5

// repoNameRe is the characters GitHub allows in owner and repository names.
var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func jsonNumber(s string) json.Number { return json.Number(s) }

// clientMessage is the error text github.ts echoed to the client
// (error.message), without transport details.
func clientMessage(err error) string {
	if errors.Is(err, githubapp.ErrFetchFailed) {
		return githubapp.ErrFetchFailed.Error()
	}
	return err.Error()
}

// field is one key of an ordered JSON object; absent keys are left out like
// undefined properties in JSON.stringify.
type field struct {
	key     string
	value   any
	present bool
}

type orderedObject []field

// MarshalJSON writes the present fields in order.
func (o orderedObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	for _, f := range o {
		if !f.present {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		k, err := httpx.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		v, err := httpx.Marshal(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// mappedRepo is mapRepo's output plus the values GET /repos sorts, filters
// and deduplicates on.
type mappedRepo struct {
	obj       orderedObject
	idKey     string
	owner     string
	updatedAt float64 // milliseconds; NaN when unparsable
}

func jsTruthyString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && s != ""
}

// mapRepo is mapRepo of github.ts.
func mapRepo(repo map[string]any) mappedRepo {
	owner := ""
	if o, ok := repo["owner"].(map[string]any); ok {
		owner, _ = jsTruthyString(o["login"])
	}
	if owner == "" {
		if fn, ok := repo["full_name"].(string); ok {
			owner, _, _ = strings.Cut(fn, "/")
		}
	}
	get := func(k string) field {
		v, ok := repo[k]
		return field{key: k, value: v, present: ok}
	}
	obj := orderedObject{
		get("id"), get("name"), get("full_name"),
		{key: "owner", value: owner, present: true},
		get("private"), get("clone_url"), get("html_url"), get("description"),
		get("default_branch"), get("updated_at"), get("permissions"),
	}
	idKey := "undefined"
	if v, ok := repo["id"]; ok {
		b, _ := json.Marshal(v)
		idKey = string(b)
	}
	m := mappedRepo{obj: obj, idKey: idKey, owner: owner}
	if s, ok := jsTruthyString(repo["updated_at"]); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			m.updatedAt = float64(t.UnixMilli())
		} else {
			m.updatedAt = math.NaN()
		}
	}
	return m
}

type failedInstall struct {
	ID    any    `json:"id"`
	Login string `json:"login"`
	Error string `json:"error"`
}

func objects(repos []mappedRepo) []orderedObject {
	out := make([]orderedObject, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.obj)
	}
	return out
}

func filterOwner(repos []mappedRepo, owner string) []mappedRepo {
	if owner == "" {
		return repos
	}
	out := repos[:0]
	for _, r := range repos {
		if strings.ToLower(r.owner) == owner {
			out = append(out, r)
		}
	}
	return out
}

// installationRepos fetches every repository of one installation.
func (h *handlers) installationRepos(ctx context.Context, installationID string) ([]map[string]any, error) {
	token, err := h.app.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return h.app.API().ListInstallationRepositories(ctx, token)
}

// GET /repos — repositories of every installation (user and organisations),
// deduplicated, newest activity first; ?owner= filters by owner login.
func (h *handlers) repos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	appJWT, err := h.app.JWT(ctx)
	if err != nil {
		h.fail(w, "repos", "Failed to fetch repositories", err)
		return
	}
	ownerFilter := ""
	if o, ok := queryString(r, "owner"); ok {
		ownerFilter = strings.ToLower(strings.TrimSpace(o))
	}

	installations, err := h.app.API().ListInstallations(ctx, appJWT)
	if err != nil {
		// Fall back to the saved installation, flagged so the UI does not look multi-account.
		h.d.Log.Error("Failed to list all installations for /repos, falling back", "error", err)
		installationID, serr := h.setting(ctx, githubapp.KeyInstallationID)
		if serr != nil {
			h.fail(w, "repos", "Failed to fetch repositories", serr)
			return
		}
		if installationID == nil {
			httpx.Error(w, http.StatusUnauthorized, "GitHub not connected")
			return
		}
		raw, ferr := h.installationRepos(ctx, *installationID)
		if ferr != nil {
			httpx.JSON(w, http.StatusBadGateway, struct {
				Error   string `json:"error"`
				Details string `json:"details"`
			}{"Failed to fetch repositories (installations list and saved install both failed)", clientMessage(ferr)})
			return
		}
		mapped := make([]mappedRepo, 0, len(raw))
		for _, repo := range raw {
			mapped = append(mapped, mapRepo(repo))
		}
		httpx.JSON(w, http.StatusOK, struct {
			Repositories        []orderedObject `json:"repositories"`
			FailedInstallations []failedInstall `json:"failedInstallations"`
			FallbackSingle      bool            `json:"fallbackSingle"`
		}{objects(filterOwner(mapped, ownerFilter)), []failedInstall{}, true})
		return
	}

	type result struct {
		raw []map[string]any
		err error
	}
	results := make([]result, len(installations))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallelInstallations)
	for i, inst := range installations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			raw, err := h.installationRepos(ctx, inst.ID.String())
			results[i] = result{raw, err}
		}()
	}
	wg.Wait()

	failed := []failedInstall{}
	seen := map[string]bool{}
	var repos []mappedRepo
	for i, res := range results {
		inst := installations[i]
		login := "Unknown"
		if inst.Account != nil {
			login = orDefault(inst.Account.Login, "Unknown")
		}
		if res.err != nil {
			msg := clientMessage(res.err)
			h.d.Log.Error("Failed to fetch repos for installation", "installation", inst.ID.String(), "account", login, "error", msg)
			failed = append(failed, failedInstall{ID: idValue(inst.ID.String()), Login: login, Error: msg})
			continue
		}
		for _, repo := range res.raw {
			m := mapRepo(repo)
			if !seen[m.idKey] {
				seen[m.idKey] = true
				repos = append(repos, m)
			}
		}
	}
	if len(repos) == 0 && len(failed) > 0 {
		httpx.JSON(w, http.StatusBadGateway, struct {
			Error               string          `json:"error"`
			FailedInstallations []failedInstall `json:"failedInstallations"`
		}{"Failed to fetch repositories from all GitHub installations", failed})
		return
	}
	repos = filterOwner(repos, ownerFilter)
	// Array#sort with (b - a): NaN comparisons leave the pair in place.
	sort.SliceStable(repos, func(i, j int) bool { return repos[i].updatedAt > repos[j].updatedAt })
	httpx.JSON(w, http.StatusOK, struct {
		Repositories        []orderedObject `json:"repositories"`
		FailedInstallations []failedInstall `json:"failedInstallations"`
		FallbackSingle      bool            `json:"fallbackSingle"`
	}{objects(repos), failed, false})
}

// repoToken is githubRepoApiHeaders: an installation token for owner/repo,
// or "" for public access. A private repository without a resolvable
// installation is refused (ok false).
func (h *handlers) repoToken(ctx context.Context, owner, repo, typ string) (token string, ok bool) {
	id, err := h.app.InstallationIDForRepo(ctx, owner, repo)
	if err == nil {
		token, err = h.app.InstallationToken(ctx, id)
	}
	if err != nil {
		h.d.Log.Warn("Could not resolve installation for repository", "repo", owner+"/"+repo, "error", clientMessage(err))
		return "", typ != "private"
	}
	return token, true
}

// refs serves GET /branches and GET /tags (?repo=owner/name&type=private|public).
func (h *handlers) refs(tags bool) http.HandlerFunc {
	what := "branches"
	if tags {
		what = "tags"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		repo, ok := queryString(r, "repo")
		if !ok || repo == "" {
			httpx.Error(w, http.StatusBadRequest, "Repo parameter is required (owner/name)")
			return
		}
		parts := strings.Split(repo, "/")
		owner, name := parts[0], ""
		if len(parts) > 1 {
			name = parts[1]
		}
		if owner == "" || name == "" {
			httpx.Error(w, http.StatusBadRequest, "Invalid repo format. Expected owner/name")
			return
		}
		typ, _ := queryString(r, "type")
		token, allowed := h.repoToken(ctx, owner, name, typ)
		if !allowed {
			httpx.Error(w, http.StatusForbidden, "Access denied: Could not verify GitHub App installation for this repository.")
			return
		}
		// Anything that is not a plain owner/name cannot be a repository:
		// answer as GitHub would instead of sending it into an API path.
		if len(parts) != 2 || !validRepoSegment(owner) || !validRepoSegment(name) {
			httpx.Error(w, http.StatusNotFound, "Repository not found")
			return
		}
		var names []any
		var err error
		if tags {
			names = []any{}
			for page := 1; page <= maxTagPages; page++ {
				var batch []any
				batch, err = h.app.API().ListTags(ctx, token, owner, name, page)
				if err != nil {
					break
				}
				names = append(names, batch...)
				if len(batch) < 100 {
					break
				}
			}
		} else {
			names, err = h.app.API().ListBranches(ctx, token, owner, name)
		}
		if err != nil {
			switch githubapp.StatusOf(err) {
			case http.StatusNotFound:
				httpx.Error(w, http.StatusNotFound, "Repository not found")
			case http.StatusForbidden:
				httpx.Error(w, http.StatusForbidden, "Rate limit exceeded or access denied")
			default:
				h.d.Log.Error("Failed to fetch "+what, "error", err)
				httpx.Error(w, http.StatusInternalServerError, clientMessage(err))
			}
			return
		}
		httpx.JSON(w, http.StatusOK, names)
	}
}

func validRepoSegment(s string) bool {
	return s != "." && s != ".." && repoNameRe.MatchString(s)
}
