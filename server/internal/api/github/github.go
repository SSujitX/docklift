// Package github serves /api/github (the port of backend/src/routes/github.ts):
// the GitHub App manifest flow, installation callbacks, connection status,
// repository/branch/tag listing and the push webhook that triggers
// auto-deploys. App credentials, JWTs, tokens and setup states live in
// internal/githubapp.
//
// /webhook, /callback, /manifest/callback and /setup are public (the router
// skips session auth for them); everything they read is treated as hostile.
package github

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	"docklift/internal/githubapp"
	"docklift/internal/httpx"
	"docklift/internal/js"
)

// Cookie carrying the setup-state nonce through the GitHub round trip.
const stateCookie = "docklift_github_state"

const manifestCallbackPath = "/api/github/manifest/callback"

var digitsRe = regexp.MustCompile(`^[0-9]+$`)

// Deployer queues a deploy of a project. EnqueueDeploy must only record the
// request and return: the deploy runs on the deploy service's own goroutine
// (one running plus one pending per project, the latest request wins), so
// the webhook can answer GitHub immediately.
type Deployer interface {
	EnqueueDeploy(ctx context.Context, projectID, trigger, commitMessage string) error
}

// Options are the optional dependencies of MountWith.
type Options struct {
	// App is the process-wide GitHub App (shared token cache). Nil builds
	// one from Deps with the production GitHub client.
	App *githubapp.App
	// Deployer receives webhook-triggered deploys. When nil, matching pushes
	// are still acknowledged and logged but nothing is deployed.
	Deployer Deployer
}

type handlers struct {
	d        *app.Deps
	app      *githubapp.App
	deployer Deployer
	cooldown *cooldown
}

// NewApp returns the GitHub App for d using the real GitHub API. Create one
// per process and share it (installation token cache).
func NewApp(d *app.Deps) *githubapp.App {
	return githubapp.New(githubapp.Config{
		AppID:          d.Config.GitHubAppID,
		PrivateKeyPath: d.Config.GitHubPrivateKeyPath,
		DataPath:       d.Config.DataPath,
	}, d.Store, githubapp.NewClient(""), d.Log)
}

// MountWith registers the /api/github routes on r, which is already scoped
// to /api/github and wrapped in the group's authentication middleware.
func MountWith(r chi.Router, d *app.Deps, o Options) {
	mount(r, newHandlers(d, o))
}

func newHandlers(d *app.Deps, o Options) *handlers {
	h := &handlers{d: d, app: o.App, deployer: o.Deployer, cooldown: newCooldown()}
	if h.app == nil {
		h.app = NewApp(d)
	}
	return h
}

func mount(r chi.Router, h *handlers) {
	r.Post("/manifest", h.manifest)
	r.Get("/manifest/callback", h.manifestCallback)
	r.Get("/app-status", h.appStatus)
	r.Post("/check-installation", h.checkInstallation)
	r.Post("/install-session", h.installSession)
	r.Get("/install", h.install)
	r.Get("/setup", h.setup)
	r.Get("/callback", h.callback)
	r.Get("/status", h.status)
	r.Get("/installations", h.installations)
	r.Get("/repos", h.repos)
	r.Get("/branches", h.refs(false))
	r.Get("/tags", h.refs(true))
	r.Post("/disconnect", h.disconnect)
	r.Post("/webhook", h.webhook)
}

// setting reads a setting with getSetting semantics; nil means null.
func (h *handlers) setting(ctx context.Context, key string) (*string, error) {
	v, ok, err := h.app.Setting(ctx, key)
	if err != nil || !ok {
		return nil, err
	}
	return &v, nil
}

func (h *handlers) saveSettings(ctx context.Context, kv ...string) error {
	for i := 0; i+1 < len(kv); i += 2 {
		if err := h.app.SaveSetting(ctx, kv[i], kv[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func installURLFor(slug *string) *string {
	if slug == nil {
		return nil
	}
	u := "https://github.com/apps/" + githubapp.PathSegment(*slug) + "/installations/new"
	return &u
}

func (h *handlers) setStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	secure := ""
	if requestProto(r) == "https" {
		secure = "; Secure"
	}
	w.Header().Add("Set-Cookie", stateCookie+"="+js.EncodeURIComponent(state)+"; Path=/; HttpOnly; SameSite=Lax; Max-Age=3600"+secure)
}

func clearStateCookie(w http.ResponseWriter) {
	w.Header().Add("Set-Cookie", stateCookie+"=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0")
}

// requireState checks the CSRF nonce of a GitHub return (manifest callback
// and /setup): the `state` query parameter GitHub echoes back must be
// present, equal (constant time) to the HttpOnly cookie set in the browser
// that started the flow, and name a valid unexpired state. It returns that
// state. TS (requireGithubSetupState) accepted the query OR the cookie, so a
// link carrying an attacker's own state, or the victim's cookie alone,
// passed: a login-CSRF that could bind an attacker's App or installation.
func (h *handlers) requireState(r *http.Request) (string, bool, error) {
	fromQuery, _ := queryString(r, "state")
	fromCookie, err := readCookie(r, stateCookie)
	if err != nil {
		return "", false, err
	}
	if fromQuery == "" || fromCookie == "" || subtle.ConstantTimeCompare([]byte(fromQuery), []byte(fromCookie)) != 1 {
		return "", false, nil
	}
	ok, err := h.app.States().Verify(r.Context(), fromQuery)
	if err != nil || !ok {
		return "", false, err
	}
	return fromQuery, true, nil
}

// withState appends the state nonce to a GitHub install URL; GitHub passes
// it back to the setup URL.
func withState(installURL, state string) string {
	return installURL + "?state=" + js.EncodeURIComponent(state)
}

func (h *handlers) frontendError(code string) string {
	return h.d.Config.FrontendURL + "/settings?github_error=" + code
}

// sanitizeAppName lowercases like String#toLowerCase, turns every UTF-16
// code unit outside [a-z0-9-] into '-' and keeps 34 characters.
func sanitizeAppName(name string) string {
	var b strings.Builder
	for _, c := range name {
		if c == 'İ' {
			// "İ".toLowerCase() is "i" plus a combining dot above.
			b.WriteString("i-")
			continue
		}
		lc := unicode.ToLower(c)
		switch {
		case (lc >= 'a' && lc <= 'z') || (lc >= '0' && lc <= '9') || lc == '-':
			b.WriteRune(lc)
		case lc > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 34 {
		out = out[:34]
	}
	return out
}

type manifestHook struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

type manifestPermissions struct {
	Contents string `json:"contents"`
	Metadata string `json:"metadata"`
}

type appManifest struct {
	Name               string              `json:"name"`
	URL                string              `json:"url"`
	RedirectURL        string              `json:"redirect_url"`
	CallbackURLs       []string            `json:"callback_urls"`
	SetupURL           string              `json:"setup_url"`
	HookAttributes     manifestHook        `json:"hook_attributes"`
	DefaultEvents      []string            `json:"default_events"`
	Public             bool                `json:"public"`
	DefaultPermissions manifestPermissions `json:"default_permissions"`
}

// POST /manifest — build the App manifest and the GitHub form action.
func (h *handlers) manifest(w http.ResponseWriter, r *http.Request) {
	body := httpx.BodyFrom(r)
	appName, ok := body.Get("appName").String()
	if !ok || appName == "" {
		httpx.Error(w, http.StatusBadRequest, "appName is required")
		return
	}
	sanitized := sanitizeAppName(appName)
	serverURL := h.publicBaseURL(r)
	safeReturn := ""
	if rv := body.Get("returnUrl"); rv.Truthy() {
		s, isString := rv.String()
		safeReturn = h.sanitizeReturnURL(s, isString, r)
	}
	state, err := h.app.States().Create(safeReturn)
	if err != nil {
		h.fail(w, "Manifest generation error", "Failed to generate manifest", err)
		return
	}
	h.setStateCookie(w, r, state)

	// GitHub rejects redirect_url values with a query string; the CSRF nonce
	// goes on the form action (?state=) and in the HttpOnly cookie.
	m := appManifest{
		Name:           "docklift-" + sanitized,
		URL:            "https://github.com/SSujitX/docklift",
		RedirectURL:    serverURL + manifestCallbackPath,
		CallbackURLs:   []string{serverURL + manifestCallbackPath},
		SetupURL:       serverURL + "/api/github/setup",
		HookAttributes: manifestHook{URL: serverURL + "/api/github/webhook", Active: true},
		DefaultEvents:  []string{"push"},
		// Public, as in TS: the App is created on the user's personal account,
		// and only a public App can be installed on other accounts (the
		// "Add Account" flow for organisations). Anyone may therefore install
		// it, so installations are only bound through a matching setup state
		// (/setup) or the saved id / App owner (check-installation).
		Public:             true,
		DefaultPermissions: manifestPermissions{Contents: "read", Metadata: "read"},
	}
	encoded, err := httpx.Marshal(m)
	if err != nil {
		h.fail(w, "Manifest generation error", "Failed to generate manifest", err)
		return
	}
	if err := h.app.SaveSetting(r.Context(), githubapp.KeyPendingAppName, sanitized); err != nil {
		h.fail(w, "Manifest generation error", "Failed to generate manifest", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Manifest  string `json:"manifest"`
		Action    string `json:"action"`
		ServerURL string `json:"serverUrl"`
	}{string(encoded), "https://github.com/settings/apps/new?state=" + js.EncodeURIComponent(state), serverURL})
}

// GET /manifest/callback (public) — exchange the manifest code for the new
// App's credentials, then send the browser to install it.
func (h *handlers) manifestCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	failed := func(err error) {
		h.d.Log.Error("Manifest callback error", "error", err)
		redirect(w, r, h.frontendError("callback_failed"))
	}
	state, ok, err := h.requireState(r)
	if err != nil {
		failed(err)
		return
	}
	if !ok {
		redirect(w, r, h.frontendError("invalid_state"))
		return
	}
	code := queryCast(r, "code")
	if code == "" {
		if inst := queryCast(r, "installation_id"); inst != "" {
			redirect(w, r, "/api/github/setup?installation_id="+js.EncodeURIComponent(inst)+"&state="+js.EncodeURIComponent(state))
			return
		}
		redirect(w, r, h.frontendError("no_code"))
		return
	}

	conv, err := h.app.API().ConvertManifest(ctx, code)
	if err != nil {
		if status := githubapp.StatusOf(err); status != 0 {
			h.d.Log.Error("GitHub manifest conversion failed", "status", status)
			redirect(w, r, h.frontendError("conversion_failed"))
			return
		}
		failed(err)
		return
	}
	if conv.ID == "" || conv.Slug == "" || conv.PEM == "" {
		failed(errors.New("GitHub manifest conversion returned an incomplete App"))
		return
	}
	login, avatar := "", ""
	if conv.Owner != nil {
		login, avatar = deref(conv.Owner.Login), deref(conv.Owner.AvatarURL)
	}
	// Persisted only after the state was verified.
	if err := h.saveSettings(ctx,
		githubapp.KeyAppID, conv.ID.String(),
		githubapp.KeyAppSlug, conv.Slug,
		githubapp.KeyAppName, conv.Name,
		githubapp.KeyClientID, conv.ClientID,
		githubapp.KeyClientSecret, conv.ClientSecret,
		githubapp.KeyPrivateKey, conv.PEM,
		githubapp.KeyWebhookSecret, conv.WebhookSecret,
		githubapp.KeyUsername, login,
		githubapp.KeyAvatarURL, avatar,
	); err != nil {
		failed(err)
		return
	}
	h.app.ForgetTokens()

	states := h.app.States()
	preserved := states.ReturnURL(state)
	if err := states.Clear(ctx, state); err != nil {
		failed(err)
		return
	}
	// Fresh one-time nonce for the install -> /setup return.
	installState, err := states.Create(preserved)
	if err != nil {
		failed(err)
		return
	}
	h.setStateCookie(w, r, installState)
	h.d.Log.Info("GitHub App created successfully", "name", conv.Name, "id", conv.ID.String())
	redirect(w, r, withState(*installURLFor(&conv.Slug), installState))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// orDefault is `value || def`.
func orDefault(s *string, def string) string {
	if s == nil || *s == "" {
		return def
	}
	return *s
}

// GET /app-status — whether an App is configured and installed.
func (h *handlers) appStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var vals [6]*string
	for i, k := range []string{githubapp.KeyAppID, githubapp.KeyAppName, githubapp.KeyAppSlug, githubapp.KeyInstallationID, githubapp.KeyUsername, githubapp.KeyAvatarURL} {
		v, err := h.setting(ctx, k)
		if err != nil {
			h.fail(w, "app status", "Failed to get app status", err)
			return
		}
		vals[i] = v
	}
	httpx.JSON(w, http.StatusOK, struct {
		Configured     bool    `json:"configured"`
		Installed      bool    `json:"installed"`
		AppID          *string `json:"appId"`
		AppName        *string `json:"appName"`
		AppSlug        *string `json:"appSlug"`
		InstallationID *string `json:"installationId"`
		Username       *string `json:"username"`
		AvatarURL      *string `json:"avatarUrl"`
		InstallURL     *string `json:"installUrl"`
	}{vals[0] != nil, vals[3] != nil, vals[0], vals[1], vals[2], vals[3], vals[4], vals[5], installURLFor(vals[2])})
}

type notFoundBody struct {
	Found   bool   `json:"found"`
	Message string `json:"message"`
}

type installationSummary struct {
	ID    any    `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

// idValue is the installation id as JSON (undefined ids become null).
func idValue(id string) any {
	if id == "" {
		return nil
	}
	return jsonNumber(id)
}

// POST /check-installation — find the App's installation without waiting for
// the /setup redirect (localhost installs).
func (h *handlers) checkInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	unexpected := notFoundBody{Message: "Could not check installation. Please try installing the app."}
	appID, err := h.setting(ctx, githubapp.KeyAppID)
	if err != nil {
		httpx.JSON(w, http.StatusOK, unexpected)
		return
	}
	if appID == nil {
		httpx.Error(w, http.StatusBadRequest, "GitHub App not configured")
		return
	}
	appJWT, err := h.app.JWT(ctx)
	if err != nil {
		httpx.JSON(w, http.StatusOK, notFoundBody{Message: "GitHub App credentials not ready. Please complete app creation."})
		return
	}
	installations, err := h.app.API().ListInstallations(ctx, appJWT)
	if err != nil {
		switch githubapp.StatusOf(err) {
		case http.StatusNotFound:
			httpx.JSON(w, http.StatusOK, notFoundBody{Message: `App not installed yet. Click "Install GitHub App" to continue.`})
		case http.StatusUnauthorized:
			httpx.JSON(w, http.StatusOK, notFoundBody{Message: "Authentication failed. Please recreate the GitHub App."})
		default:
			h.d.Log.Error("GitHub API error listing installations", "error", err)
			httpx.JSON(w, http.StatusOK, notFoundBody{Message: "Could not verify installation status."})
		}
		return
	}
	if len(installations) == 0 {
		httpx.JSON(w, http.StatusOK, notFoundBody{Message: "No installations found. Please install the app on GitHub first."})
		return
	}
	// Keep the saved installation when it is still present, else take the
	// installation on the account that owns the App. Never fall back to
	// installations[0] as TS did: the App is public, so anyone can install it
	// on their own account, and that installation must not become ours.
	existing, err := h.setting(ctx, githubapp.KeyInstallationID)
	if err != nil {
		httpx.JSON(w, http.StatusOK, unexpected)
		return
	}
	owner, err := h.setting(ctx, githubapp.KeyUsername)
	if err != nil {
		httpx.JSON(w, http.StatusOK, unexpected)
		return
	}
	chosen, matched := matchInstallation(installations, deref(existing), deref(owner))
	if !matched {
		httpx.JSON(w, http.StatusOK, notFoundBody{Message: msgUnmatchedInstallation})
		return
	}
	if chosen.ID == "" {
		httpx.JSON(w, http.StatusOK, unexpected)
		return
	}
	var login, avatar, typ *string
	if chosen.Account != nil {
		login, avatar = chosen.Account.Login, chosen.Account.AvatarURL
	}
	if err := h.saveSettings(ctx,
		githubapp.KeyInstallationID, chosen.ID.String(),
		githubapp.KeyUsername, orDefault(login, "unknown"),
		githubapp.KeyAvatarURL, orDefault(avatar, ""),
	); err != nil {
		httpx.JSON(w, http.StatusOK, unexpected)
		return
	}
	h.d.Log.Info("Found GitHub App installation", "installation", chosen.ID.String(), "account", deref(login), "total", len(installations))
	list := make([]installationSummary, 0, len(installations))
	for _, inst := range installations {
		login, typ = nil, nil
		if inst.Account != nil {
			login, typ = inst.Account.Login, inst.Account.Type
		}
		list = append(list, installationSummary{ID: idValue(inst.ID.String()), Login: orDefault(login, "Unknown"), Type: orDefault(typ, "User")})
	}
	var username *string
	if chosen.Account != nil {
		username = chosen.Account.Login
	}
	httpx.JSON(w, http.StatusOK, struct {
		Found          bool                  `json:"found"`
		InstallationID any                   `json:"installationId"`
		Username       *string               `json:"username,omitempty"`
		Installations  []installationSummary `json:"installations"`
	}{true, idValue(chosen.ID.String()), username, list})
}

// msgUnmatchedInstallation is check-installation's answer when no listed
// installation is the saved one or on the App owner's account.
const msgUnmatchedInstallation = `Installation could not be matched to this Docklift. Click "Install GitHub App" to finish setup.`

// matchInstallation picks the saved installation (by id) or, when none is
// saved or it is gone, the one on the App owner's account (GitHub logins
// compare case-insensitively). Nothing else is ever chosen.
func matchInstallation(list []githubapp.Installation, savedID, ownerLogin string) (githubapp.Installation, bool) {
	if savedID != "" {
		for _, inst := range list {
			if inst.ID.String() == savedID {
				return inst, true
			}
		}
	}
	if ownerLogin != "" && ownerLogin != "unknown" {
		for _, inst := range list {
			if inst.Account != nil && inst.Account.Login != nil && strings.EqualFold(*inst.Account.Login, ownerLogin) {
				return inst, true
			}
		}
	}
	return githubapp.Installation{}, false
}

// beginInstallSession is beginGithubInstallSession: a fresh nonce cookie
// remembering the sanitised return URL, and the App's install URL carrying
// the same nonce as ?state= (GitHub hands it back to /setup).
func (h *handlers) beginInstallSession(w http.ResponseWriter, r *http.Request, returnURL string, isString bool) (string, int, error) {
	ctx := r.Context()
	appID := h.d.Config.GitHubAppID
	if appID == "" {
		v, err := h.setting(ctx, githubapp.KeyAppID)
		if err != nil {
			return "", 0, err
		}
		appID = deref(v)
	}
	if appID == "" {
		return "", http.StatusBadRequest, nil
	}
	safeReturn := h.sanitizeReturnURL(returnURL, isString, r)
	state, err := h.app.States().Create(safeReturn)
	if err != nil {
		return "", 0, err
	}
	h.setStateCookie(w, r, state)
	slug, err := h.setting(ctx, githubapp.KeyAppSlug)
	if err != nil {
		return "", 0, err
	}
	s := orDefault(slug, "docklift-app")
	return withState(*installURLFor(&s), state), 0, nil
}

const msgNotConfigured = "GitHub App not configured. Please create one first."

// POST /install-session — set the nonce cookie and return the install URL.
func (h *handlers) installSession(w http.ResponseWriter, r *http.Request) {
	body := httpx.BodyFrom(r)
	rv := body.Get("returnUrl")
	if !rv.Defined() || rv.IsNull() {
		rv = body.Get("return_url")
	}
	s, isString := rv.String()
	installURL, status, err := h.beginInstallSession(w, r, s, isString)
	switch {
	case err != nil:
		h.fail(w, "install session", "Failed to initiate GitHub install", err)
	case status != 0:
		httpx.Error(w, status, msgNotConfigured)
	default:
		httpx.JSON(w, http.StatusOK, struct {
			InstallURL string `json:"installUrl"`
		}{installURL})
	}
}

// GET /install — legacy redirect variant of /install-session.
func (h *handlers) install(w http.ResponseWriter, r *http.Request) {
	s, isString := queryString(r, "return_url")
	installURL, status, err := h.beginInstallSession(w, r, s, isString)
	switch {
	case err != nil:
		h.fail(w, "install", "Failed to initiate GitHub install", err)
	case status != 0:
		httpx.Error(w, status, msgNotConfigured)
	default:
		redirect(w, r, installURL)
	}
}

// GET /setup (public) — GitHub's post-installation redirect. The
// installation must belong to this App before anything is stored.
func (h *handlers) setup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	returnBase := h.d.Config.FrontendURL + "/settings"
	failed := func(err error) {
		h.d.Log.Error("GitHub setup failed", "error", err)
		redirect(w, r, returnBase+"?github_error=setup_failed")
	}
	installationID := queryCast(r, "installation_id")
	if !digitsRe.MatchString(installationID) {
		redirect(w, r, returnBase+"?github_error=invalid_installation")
		return
	}
	state, ok, err := h.requireState(r)
	if err != nil {
		failed(err)
		return
	}
	if !ok {
		redirect(w, r, returnBase+"?github_error=invalid_state")
		return
	}
	appJWT, err := h.app.JWT(ctx)
	if err != nil {
		failed(err)
		return
	}
	inst, err := h.app.API().GetInstallation(ctx, appJWT, installationID)
	if err != nil {
		if status := githubapp.StatusOf(err); status != 0 {
			h.d.Log.Warn("[GITHUB] Installation verify failed", "installation", installationID, "status", status)
			redirect(w, r, returnBase+"?github_error=installation_verify_failed")
			return
		}
		failed(err)
		return
	}
	var login, avatar *string
	if inst.Account != nil {
		login, avatar = inst.Account.Login, inst.Account.AvatarURL
	}
	if err := h.saveSettings(ctx,
		githubapp.KeyInstallationID, installationID,
		githubapp.KeyUsername, orDefault(login, "unknown"),
		githubapp.KeyAvatarURL, orDefault(avatar, ""),
	); err != nil {
		failed(err)
		return
	}
	states := h.app.States()
	returnURL := states.ReturnURL(state)
	if returnURL == "" {
		returnURL = returnBase
	}
	if err := states.Clear(ctx, state); err != nil {
		failed(err)
		return
	}
	clearStateCookie(w)
	// Drop the legacy global return URL if present.
	_ = h.app.DeleteSetting(ctx, "github_return_url")
	sep := "?"
	if strings.Contains(returnURL, "?") {
		sep = "&"
	}
	redirect(w, r, returnURL+sep+"github=connected")
}

// GET /callback (public) — the legacy OAuth callback no longer mutates
// anything; installation redirects are forwarded to /setup.
func (h *handlers) callback(w http.ResponseWriter, r *http.Request) {
	installationID := queryCast(r, "installation_id")
	if digitsRe.MatchString(installationID) {
		qs := "installation_id=" + formEncode(installationID)
		if state, ok := queryString(r, "state"); ok && state != "" {
			qs += "&state=" + formEncode(state)
		}
		redirect(w, r, "/api/github/setup?"+qs)
		return
	}
	redirect(w, r, h.frontendError("legacy_oauth_disabled"))
}

// GET /status — installation status for the settings page.
func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var vals [5]*string
	for i, k := range []string{githubapp.KeyInstallationID, githubapp.KeyUsername, githubapp.KeyAvatarURL, githubapp.KeyAppSlug, githubapp.KeyAppName} {
		v, err := h.setting(ctx, k)
		if err != nil {
			h.fail(w, "github status", "Failed to get GitHub status", err)
			return
		}
		vals[i] = v
	}
	installationID, username, avatar, slug, name := vals[0], vals[1], vals[2], vals[3], vals[4]
	if installationID == nil {
		httpx.JSON(w, http.StatusOK, struct {
			Connected bool    `json:"connected"`
			Username  *string `json:"username"`
			AppName   *string `json:"app_name"`
			AppSlug   *string `json:"app_slug"`
		}{false, nil, name, slug})
		return
	}
	_, hasKey, err := h.app.PrivateKey(ctx)
	if err != nil {
		h.fail(w, "github status", "Failed to get GitHub status", err)
		return
	}
	if !hasKey {
		httpx.JSON(w, http.StatusOK, struct {
			Connected      bool    `json:"connected"`
			Username       *string `json:"username"`
			AvatarURL      *string `json:"avatar_url"`
			InstallationID *string `json:"installation_id"`
			AppName        *string `json:"app_name"`
			AppSlug        *string `json:"app_slug"`
			Warning        string  `json:"warning"`
		}{true, username, avatar, installationID, name, slug, "Private key not found"})
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Connected      bool    `json:"connected"`
		Username       *string `json:"username"`
		AvatarURL      *string `json:"avatar_url"`
		InstallationID *string `json:"installation_id"`
		AppName        *string `json:"app_name"`
		AppSlug        *string `json:"app_slug"`
		InstallURL     *string `json:"installUrl"`
	}{true, username, avatar, installationID, name, slug, installURLFor(slug)})
}

// GET /installations — every account the App is installed on.
func (h *handlers) installations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	appJWT, err := h.app.JWT(ctx)
	if err != nil {
		h.fail(w, "Failed to fetch installations", "Failed to fetch installations", err)
		return
	}
	installations, err := h.app.API().ListInstallations(ctx, appJWT)
	if err != nil {
		h.fail(w, "Failed to fetch installations", "Failed to fetch installations", err)
		return
	}
	slug, err := h.setting(ctx, githubapp.KeyAppSlug)
	if err != nil {
		h.fail(w, "Failed to fetch installations", "Failed to fetch installations", err)
		return
	}
	type item struct {
		ID        any    `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		Type      string `json:"type"`
	}
	list := make([]item, 0, len(installations))
	for _, inst := range installations {
		var login, avatar, typ *string
		if inst.Account != nil {
			login, avatar, typ = inst.Account.Login, inst.Account.AvatarURL, inst.Account.Type
		}
		list = append(list, item{idValue(inst.ID.String()), orDefault(login, "Unknown"), orDefault(avatar, ""), orDefault(typ, "User")})
	}
	httpx.JSON(w, http.StatusOK, struct {
		Installations []item  `json:"installations"`
		InstallURL    *string `json:"installUrl"`
	}{list, installURLFor(slug)})
}

// POST /disconnect — forget the installation (the App itself stays).
func (h *handlers) disconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	for _, k := range []string{githubapp.KeyInstallationID, githubapp.KeyUsername, githubapp.KeyAvatarURL} {
		if err := h.app.DeleteSetting(ctx, k); err != nil {
			h.fail(w, "disconnect", "Failed to disconnect GitHub", err)
			return
		}
	}
	h.app.ForgetTokens()
	httpx.JSON(w, http.StatusOK, struct {
		Message string `json:"message"`
	}{"GitHub disconnected"})
}

// fail logs err and answers 500 {"error": msg}.
func (h *handlers) fail(w http.ResponseWriter, logMsg, msg string, err error) {
	h.d.Log.Error(logMsg, "error", err)
	httpx.Error(w, http.StatusInternalServerError, msg)
}
