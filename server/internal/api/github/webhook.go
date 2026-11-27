package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"docklift/internal/githubapp"
	"docklift/internal/gitops"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// deployCooldown is DEPLOY_COOLDOWN_MS: a project is not re-triggered by
// pushes within this window.
const deployCooldown = 10 * time.Second

// cooldown is the recentDeploys map of github.ts.
type cooldown struct {
	mu   sync.Mutex
	last map[string]time.Time
	now  func() time.Time
}

func newCooldown() *cooldown {
	return &cooldown{last: map[string]time.Time{}, now: time.Now}
}

// take reports whether projectID may deploy now and, if so, starts its
// cooldown. Entries older than the window are dropped as it goes.
func (c *cooldown) take(projectID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for id, at := range c.last {
		if now.Sub(at) >= deployCooldown {
			delete(c.last, id)
		}
	}
	if at, ok := c.last[projectID]; ok && now.Sub(at) < deployCooldown {
		return false
	}
	c.last[projectID] = now
	return true
}

// validSignature compares X-Hub-Signature-256 with the HMAC-SHA256 of the raw
// body in constant time; a different length is simply a mismatch.
func validSignature(payload []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	digest := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(digest))
}

// headerValue is a request header as Node exposes it (repeats joined).
func headerValue(r *http.Request, name string) (string, bool) {
	vs := r.Header.Values(name)
	if len(vs) == 0 {
		return "", false
	}
	return strings.Join(vs, ", "), true
}

// utf16Prefix is String#substring(0, n).
func utf16Prefix(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	return string(utf16.Decode(u[:n]))
}

type messageBody struct {
	Message string `json:"message"`
}

// POST /webhook (public) — push events of every repository the App can see.
// The signature is checked before any lookup; matching projects with
// auto-deploy on are queued for a deploy and GitHub gets its answer at once.
func (h *handlers) webhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	event, ok := headerValue(r, "X-GitHub-Event")
	if !ok {
		event = "undefined"
	}
	if event != "push" {
		httpx.JSON(w, http.StatusOK, messageBody{"Ignoring " + event + " event"})
		return
	}

	// Verify the signature first, before any database query or processing.
	secret, ok, err := h.app.Setting(ctx, githubapp.KeyWebhookSecret)
	if err != nil {
		h.fail(w, "Webhook error", "Webhook processing failed", err)
		return
	}
	// Fail closed: never accept unsigned webhooks.
	if !ok {
		h.d.Log.Warn("[Webhook] Rejected — webhook secret not configured")
		httpx.Error(w, http.StatusUnauthorized, "Webhook secret not configured")
		return
	}
	signature, _ := headerValue(r, "X-Hub-Signature-256")
	if signature == "" {
		h.d.Log.Warn("[Webhook] Missing signature header")
		httpx.Error(w, http.StatusUnauthorized, "Missing webhook signature")
		return
	}
	body := httpx.BodyFrom(r)
	// The HMAC covers the exact bytes GitHub sent. Only JSON bodies keep
	// them; any other body cannot carry a valid signature.
	if body.Raw == nil || !validSignature(body.Raw, signature, secret) {
		h.d.Log.Warn("[Webhook] Signature verification failed")
		httpx.Error(w, http.StatusUnauthorized, "Invalid signature")
		return
	}

	payload := body.Value()
	if deleted, _ := payload.Get("deleted").Bool(); deleted || !payload.Get("head_commit").Defined() || payload.Get("head_commit").IsNull() {
		httpx.JSON(w, http.StatusOK, messageBody{"Ignoring branch deletion or empty push"})
		return
	}

	repository := payload.Get("repository")
	repoValue := repository.Get("clone_url")
	if !repoValue.Truthy() {
		repoValue = repository.Get("html_url")
	}
	rawRef, _ := payload.Get("ref").String()
	pushedRef, pushedKind := rawRef, "other"
	switch {
	case strings.HasPrefix(rawRef, "refs/heads/"):
		pushedRef, pushedKind = strings.TrimPrefix(rawRef, "refs/heads/"), "branch"
	case strings.HasPrefix(rawRef, "refs/tags/"):
		pushedRef, pushedKind = strings.TrimPrefix(rawRef, "refs/tags/"), "tag"
	}
	if !repoValue.Truthy() {
		httpx.JSON(w, http.StatusOK, messageBody{"No repository URL in payload"})
		return
	}
	repoURL, isString := repoValue.String()
	if !isString {
		// repoUrl.replace on a non-string threw in github.ts.
		h.fail(w, "Webhook error", "Webhook processing failed", errNonStringRepoURL)
		return
	}
	normalized := strings.TrimSuffix(repoURL, ".git")

	github, autoDeploy := "github", true
	projects, err := h.d.Store.ListProjects(ctx, store.ProjectFilter{
		SourceType:  &github,
		AutoDeploy:  &autoDeploy,
		GithubURLIn: []string{repoURL, normalized, normalized + ".git"},
	}, store.OrderNone)
	if err != nil {
		h.fail(w, "Webhook error", "Webhook processing failed", err)
		return
	}
	if len(projects) == 0 {
		h.logUnmatched(ctx, repoURL)
		httpx.JSON(w, http.StatusOK, messageBody{"No matching projects with auto-deploy enabled"})
		return
	}

	headCommit := payload.Get("head_commit")
	commitMessage := "No message"
	if m := headCommit.Get("message"); m.Truthy() {
		if s, ok := m.String(); ok {
			commitMessage = s
		}
	}
	pusher := "Unknown"
	if p, ok := payload.Get("pusher").Get("name").String(); ok && p != "" {
		pusher = p
	}

	triggered, skipped := []string{}, []string{}
	for _, p := range projects {
		// Match the stored pin (branch or tag name) against the normalised ref.
		if branch := deref(p.GithubBranch); branch != "" && branch != pushedRef {
			shown := pushedRef
			if shown == "" {
				shown = rawRef
			}
			skipped = append(skipped, p.Name+" (ref mismatch: "+shown+" != "+branch+")")
			continue
		}
		if pushedRef == "" || pushedKind == "other" {
			shown := rawRef
			if shown == "" {
				shown = "none"
			}
			skipped = append(skipped, p.Name+" (unsupported ref: "+shown+")")
			continue
		}
		if !h.cooldown.take(p.ID) {
			skipped = append(skipped, p.Name+" (cooldown)")
			continue
		}
		h.d.Log.Info("[Auto-Deploy] "+p.Name+" by "+pusher, "project", p.ID)
		h.enqueue(p, commitMessage)
		triggered = append(triggered, p.Name)
	}

	commit := "unknown"
	if id, ok := headCommit.Get("id").String(); ok {
		if short := utf16Prefix(id, 7); short != "" {
			commit = short
		}
	}
	message := "No deployments triggered"
	if len(triggered) > 0 {
		message = "Auto-deploy triggered"
	}
	httpx.JSON(w, http.StatusOK, struct {
		Message   string   `json:"message"`
		Triggered []string `json:"triggered"`
		Skipped   []string `json:"skipped"`
		Branch    string   `json:"branch"`
		Commit    string   `json:"commit"`
	}{message, triggered, skipped, pushedRef, commit})
}

type webhookError string

func (e webhookError) Error() string { return string(e) }

const errNonStringRepoURL = webhookError("repository URL in payload is not a string")

// enqueue hands the deploy to the deploy service. The queue call only records
// the request (on the server's background context, never the request's), so
// the webhook never waits for a build.
func (h *handlers) enqueue(p *store.Project, commitMessage string) {
	if h.deployer == nil {
		h.d.Log.Error("[Auto-Deploy] no deploy service is wired; push not deployed", "project", p.ID)
		return
	}
	if err := h.deployer.EnqueueDeploy(h.d.Background, p.ID, "webhook", commitMessage); err != nil {
		h.d.Log.Error("[Auto-Deploy] Failed to trigger deploy", "project", p.ID, "name", p.Name, "error", err)
	}
}

// logUnmatched logs the GitHub projects when a push matched none, to help
// spot URL mismatches. URLs are logged without credentials.
func (h *handlers) logUnmatched(ctx context.Context, repoURL string) {
	github := "github"
	all, err := h.d.Store.ListProjects(ctx, store.ProjectFilter{SourceType: &github}, store.OrderNone)
	if err != nil {
		h.d.Log.Warn("[Webhook] list GitHub projects", "error", err)
		return
	}
	type summary struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		GithubURL    string `json:"github_url"`
		GithubBranch string `json:"github_branch"`
		AutoDeploy   bool   `json:"auto_deploy"`
	}
	list := make([]summary, 0, len(all))
	for _, p := range all {
		list = append(list, summary{p.ID, p.Name, gitops.RedactURL(deref(p.GithubURL)), deref(p.GithubBranch), p.AutoDeploy != nil && *p.AutoDeploy})
	}
	h.d.Log.Info("[Webhook] No projects found for "+gitops.RedactURL(repoURL), "githubProjects", list)
}
