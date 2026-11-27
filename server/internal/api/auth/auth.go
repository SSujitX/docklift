// Package auth serves /api/auth: setup status, first-admin registration,
// login, profile, password change, the restore setup token and short-lived
// SSE/terminal tokens (the port of backend/src/routes/auth.ts).
package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	authsvc "docklift/internal/auth"
	"docklift/internal/db"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

const msgBootstrapRequired = "Bootstrap secret required. Copy it from the backend logs (or data/.bootstrap-secret on the host)."

type handlers struct {
	d *app.Deps
}

// Mount registers the /api/auth routes on r (already scoped to /api/auth,
// with the rate limiters applied by the router).
func Mount(r chi.Router, d *app.Deps) {
	h := &handlers{d: d}
	r.Get("/status", h.status)
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Get("/setup-token", h.setupToken)
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireSession)
		r.Get("/me", h.me)
		r.Patch("/profile", h.profile)
		r.Post("/change-password", h.changePassword)
		r.Post("/sse-token", h.purposeToken(authsvc.PurposeSSE, "Failed to generate SSE token"))
		r.Post("/terminal-token", h.purposeToken(authsvc.PurposeTerminal, "Failed to generate terminal token"))
	})
}

type publicUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func toPublicUser(u *store.User) publicUser {
	return publicUser{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role}
}

type statusBody struct {
	SetupComplete     bool `json:"setupComplete"`
	UserCount         int  `json:"userCount"`
	BootstrapRequired bool `json:"bootstrapRequired"`
}

type needsRestoreBody struct {
	SetupComplete     bool `json:"setupComplete"`
	UserCount         int  `json:"userCount"`
	NeedsRestore      bool `json:"needsRestore"`
	BootstrapRequired bool `json:"bootstrapRequired"`
}

// GET /status — whether setup is complete; ensures a bootstrap secret exists
// on fresh installs without exposing it.
func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	n, err := h.d.Store.CountUsers(r.Context())
	if err == nil && n == 0 {
		_, err = h.d.Auth.Bootstrap().EnsureSecret()
	}
	if errors.Is(err, store.ErrMissingTable) {
		if _, ensureErr := h.d.Auth.Bootstrap().EnsureSecret(); ensureErr != nil {
			h.fail(w, "Internal server error", ensureErr)
			return
		}
		httpx.JSON(w, http.StatusOK, needsRestoreBody{UserCount: 0, NeedsRestore: true, BootstrapRequired: true})
		return
	}
	if err != nil {
		h.fail(w, "Internal server error", err)
		return
	}
	httpx.JSON(w, http.StatusOK, statusBody{SetupComplete: n > 0, UserCount: n, BootstrapRequired: n == 0})
}

// providedBootstrapSecret is the X-Bootstrap-Secret header, else the body's
// bootstrapSecret string, both trimmed.
func providedBootstrapSecret(r *http.Request) string {
	if values := r.Header.Values("X-Bootstrap-Secret"); len(values) > 0 {
		if header := strings.TrimSpace(strings.Join(values, ", ")); header != "" {
			return header
		}
	}
	s, _ := httpx.BodyFrom(r).Get("bootstrapSecret").String()
	return strings.TrimSpace(s)
}

type sessionBody struct {
	Message string     `json:"message"`
	Token   string     `json:"token"`
	User    publicUser `json:"user"`
}

// POST /register — create the first admin; requires the bootstrap secret,
// which is claimed atomically so concurrent attempts cannot both succeed.
func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Registration failed"
	ctx := r.Context()
	body := httpx.BodyFrom(r)
	nameV, emailV, passwordV := body.Get("name"), body.Get("email"), body.Get("password")
	if !nameV.Truthy() || !emailV.Truthy() || !passwordV.Truthy() {
		httpx.Error(w, http.StatusBadRequest, "Name, email, and password are required")
		return
	}
	name, nameOK := nameV.String()
	email, emailOK := emailV.String()
	password, passwordOK := passwordV.String()
	if !nameOK || !emailOK || !passwordOK {
		httpx.Error(w, http.StatusBadRequest, "Name, email, and password must be strings")
		return
	}
	if httpx.UTF16Len(password) < 8 {
		httpx.Error(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}
	count, err := h.d.Store.CountUsers(ctx)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if count > 0 {
		httpx.Error(w, http.StatusForbidden, "Setup already complete. Use login instead.")
		return
	}
	bs := h.d.Auth.Bootstrap()
	provided := providedBootstrapSecret(r)
	if !bs.VerifySecret(provided) {
		httpx.Error(w, http.StatusForbidden, msgBootstrapRequired)
		return
	}
	claim := bs.TryClaim()
	if claim == "" {
		httpx.Error(w, http.StatusForbidden, "Registration unavailable. Setup may already be in progress or complete.")
		return
	}
	if !authsvc.VerifySecretAt(claim, provided) {
		bs.AbortClaim(claim)
		httpx.Error(w, http.StatusForbidden, msgBootstrapRequired)
		return
	}
	user, status, err := h.createFirstAdmin(r, name, email, password)
	if err != nil || status != 0 {
		bs.AbortClaim(claim)
		switch {
		case errors.Is(err, store.ErrConflict):
			httpx.Error(w, http.StatusBadRequest, "Email already exists")
		case err != nil:
			h.fail(w, failMsg, err)
		default:
			httpx.Error(w, status, "Setup already complete. Use login instead.")
		}
		return
	}
	bs.FinalizeClaim(claim)
	bs.ConsumeSecret()
	token, err := h.d.Auth.IssueSession(user)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sessionBody{Message: "Registration successful", Token: token, User: toPublicUser(user)})
}

// createFirstAdmin re-checks that no user exists (after the claim) and inserts
// the admin. A non-zero status means "setup already complete".
func (h *handlers) createFirstAdmin(r *http.Request, name, email, password string) (*store.User, int, error) {
	ctx := r.Context()
	count, err := h.d.Store.CountUsers(ctx)
	if err != nil {
		return nil, 0, err
	}
	if count > 0 {
		return nil, http.StatusForbidden, nil
	}
	hash, err := authsvc.HashPassword(password)
	if err != nil {
		return nil, 0, err
	}
	now := db.Now()
	user, err := h.d.Store.CreateUser(ctx, store.UserCreate{
		Name: name, Email: strings.ToLower(email), Password: hash, Role: "admin", PasswordChangedAt: &now,
	})
	return user, 0, err
}

// POST /login
func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Login failed"
	body := httpx.BodyFrom(r)
	emailV, passwordV := body.Get("email"), body.Get("password")
	if !emailV.Truthy() || !passwordV.Truthy() {
		httpx.Error(w, http.StatusBadRequest, "Email and password are required")
		return
	}
	email, emailOK := emailV.String()
	password, passwordOK := passwordV.String()
	if !emailOK || !passwordOK {
		httpx.Error(w, http.StatusBadRequest, "Email and password must be strings")
		return
	}
	user, err := h.d.Store.GetUserByEmail(r.Context(), strings.ToLower(email))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, failMsg, err)
		return
	}
	if user == nil {
		authsvc.EqualizeLoginTiming(password)
		httpx.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	if !authsvc.ComparePassword(user.Password, password) {
		httpx.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	token, err := h.d.Auth.IssueSession(user)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sessionBody{Message: "Login successful", Token: token, User: toPublicUser(user)})
}

type meUser struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	CreatedAt db.Time `json:"created_at"`
}

// GET /me
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	authUser, _ := authsvc.UserFrom(r.Context())
	user, err := h.d.Store.GetUser(r.Context(), authUser.UserID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "User not found")
		return
	}
	if err != nil {
		h.fail(w, "Failed to get user", err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		User meUser `json:"user"`
	}{meUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role, CreatedAt: user.CreatedAt}})
}

// PATCH /profile — change name and email.
func (h *handlers) profile(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to update profile"
	body := httpx.BodyFrom(r)
	nameV, emailV := body.Get("name"), body.Get("email")
	if !nameV.Truthy() || !emailV.Truthy() {
		httpx.Error(w, http.StatusBadRequest, "Name and email are required")
		return
	}
	name, nameOK := nameV.String()
	email, emailOK := emailV.String()
	if !nameOK || !emailOK {
		httpx.Error(w, http.StatusBadRequest, "Name and email must be strings")
		return
	}
	authUser, _ := authsvc.UserFrom(r.Context())
	user, err := h.d.Store.UpdateUser(r.Context(), authUser.UserID, store.UserUpdate{
		Name: store.Some(name), Email: store.Some(strings.ToLower(email)),
	})
	if errors.Is(err, store.ErrConflict) {
		httpx.Error(w, http.StatusBadRequest, "Email already exists")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Message string     `json:"message"`
		User    publicUser `json:"user"`
	}{"Profile updated", toPublicUser(user)})
}

// POST /change-password — invalidates existing sessions via passwordChangedAt
// and returns a fresh token for the caller.
func (h *handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to change password"
	ctx := r.Context()
	body := httpx.BodyFrom(r)
	currentV, nextV := body.Get("currentPassword"), body.Get("newPassword")
	if !currentV.Truthy() || !nextV.Truthy() {
		httpx.Error(w, http.StatusBadRequest, "Current and new passwords are required")
		return
	}
	current, currentOK := currentV.String()
	next, nextOK := nextV.String()
	if !currentOK || !nextOK {
		httpx.Error(w, http.StatusBadRequest, "Current and new passwords must be strings")
		return
	}
	if httpx.UTF16Len(next) < 8 {
		httpx.Error(w, http.StatusBadRequest, "New password must be at least 8 characters")
		return
	}
	authUser, _ := authsvc.UserFrom(ctx)
	user, err := h.d.Store.GetUser(ctx, authUser.UserID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "User not found")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !authsvc.ComparePassword(user.Password, current) {
		httpx.Error(w, http.StatusBadRequest, "Invalid current password")
		return
	}
	hash, err := authsvc.HashPassword(next)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	now := db.Now()
	updated, err := h.d.Store.UpdateUser(ctx, user.ID, store.UserUpdate{Password: store.Some(hash), PasswordChangedAt: store.Some(&now)})
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	token, err := h.d.Auth.IssueSession(updated)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		Message string `json:"message"`
		Token   string `json:"token"`
	}{"Password changed successfully", token})
}

// GET /setup-token — the fresh-install restore token; requires the bootstrap
// secret and is refused once any user exists.
func (h *handlers) setupToken(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to generate setup token"
	count, err := h.d.Store.CountUsers(r.Context())
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if count > 0 {
		httpx.Error(w, http.StatusForbidden, "Setup already complete. Setup tokens are only available before first user registration.")
		return
	}
	bs := h.d.Auth.Bootstrap()
	if !bs.VerifySecret(providedBootstrapSecret(r)) {
		httpx.Error(w, http.StatusForbidden, msgBootstrapRequired)
		return
	}
	token, err := bs.EnsureSetupToken()
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	httpx.JSON(w, http.StatusOK, struct {
		SetupToken string `json:"setupToken"`
	}{token})
}

// POST /sse-token and /terminal-token — 5-minute purpose tokens carrying the
// caller's current password version.
func (h *handlers) purposeToken(purpose, failMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUser, _ := authsvc.UserFrom(r.Context())
		var pwdv int64
		dbUser, err := h.d.Store.GetUser(r.Context(), authUser.UserID)
		switch {
		case err == nil:
			pwdv = dbUser.PasswordVersion()
		case !errors.Is(err, store.ErrNotFound):
			h.fail(w, failMsg, err)
			return
		}
		token, err := h.d.Auth.IssuePurpose(authsvc.TokenSubject{
			UserID: authUser.UserID, Email: authUser.Email, Role: authUser.Role, PasswordVersion: pwdv,
		}, purpose)
		if err != nil {
			h.fail(w, failMsg, err)
			return
		}
		httpx.JSON(w, http.StatusOK, struct {
			Token string `json:"token"`
		}{token})
	}
}

// fail logs err and answers 500 with the route's generic message.
func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error("[auth] "+msg, "error", err)
	httpx.Error(w, http.StatusInternalServerError, msg)
}
