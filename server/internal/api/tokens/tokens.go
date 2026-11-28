// Package tokens serves /api/tokens: the API tokens scripts and AI agents use
// instead of the admin password. Every route needs a dashboard session (an
// API token can never list, create or revoke tokens), and creating or
// revoking one re-verifies the account password.
package tokens

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"docklift/internal/app"
	authsvc "docklift/internal/auth"
	"docklift/internal/db"
	"docklift/internal/httpx"
	"docklift/internal/js"
	"docklift/internal/store"
)

// Limits of a new token.
const (
	MaxNameLength    = 100
	MaxExpiresInDays = 3650
)

// Token states reported by the list.
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
)

type handlers struct {
	d   *app.Deps
	now func() time.Time
}

// Mount registers the routes on r (already scoped to /api/tokens and
// behind the session guard).
func Mount(r chi.Router, d *app.Deps) {
	h := &handlers{d: d, now: time.Now}
	r.Use(authsvc.SessionOnly)
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Delete("/{id}", h.revoke)
}

// Item is one token as the API returns it.
type Item struct {
	*store.APIToken
	// Status is active, expired, or revoked (the owner's password changed
	// after the token was created).
	Status string `json:"status"`
}

// ItemOf returns t with its status at now.
func ItemOf(t *store.APIToken, owner *store.User, now time.Time) Item {
	switch authsvc.APITokenUsable(t, owner, now) {
	case "":
		return Item{t, StatusActive}
	case authsvc.MsgAPITokenExpired:
		return Item{t, StatusExpired}
	default:
		return Item{t, StatusRevoked}
	}
}

// GET / — every token (no secret material), newest first.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.d.Store.ListAPITokens(ctx)
	if err != nil {
		h.fail(w, "Failed to list API tokens", err)
		return
	}
	owners := map[string]*store.User{}
	now := h.now()
	items := make([]Item, 0, len(rows))
	for _, t := range rows {
		owner, seen := owners[t.UserID]
		if !seen {
			owner, err = h.d.Store.GetUser(ctx, t.UserID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				h.fail(w, "Failed to list API tokens", err)
				return
			}
			owners[t.UserID] = owner
		}
		items = append(items, ItemOf(t, owner, now))
	}
	httpx.JSON(w, http.StatusOK, struct {
		Tokens []Item `json:"tokens"`
	}{items})
}

// CreateInput is a validated token request.
type CreateInput struct {
	Name           string
	Scope          string
	AllowDangerous bool
	// ExpiresInDays is 0 for a token that never expires.
	ExpiresInDays int
}

// ParseCreate validates the body of POST /: name (1-100 characters),
// scope ("read" default, or "full"), allowDangerous (boolean, full scope
// only) and expiresInDays (integer 1-3650, or null/absent for no expiry).
// It returns the 400 message on failure.
func ParseCreate(b httpx.Body) (CreateInput, string) {
	var in CreateInput
	name, ok := b.Get("name").String()
	in.Name = js.Trim(name)
	if !ok || in.Name == "" || httpx.UTF16Len(in.Name) > MaxNameLength {
		return in, "name is required (max 100 characters)"
	}
	in.Scope = authsvc.ScopeRead
	if v := b.Get("scope"); v.Defined() && !v.IsNull() {
		s, _ := v.String()
		if !authsvc.ValidScope(s) {
			return in, `scope must be "read" or "full"`
		}
		in.Scope = s
	}
	if v := b.Get("allowDangerous"); v.Defined() && !v.IsNull() {
		flag, isBool := v.Raw().(bool)
		if !isBool {
			return in, "allowDangerous must be a boolean"
		}
		in.AllowDangerous = flag
	}
	if in.AllowDangerous && in.Scope != authsvc.ScopeFull {
		return in, `allowDangerous needs scope "full"`
	}
	if v := b.Get("expiresInDays"); v.Defined() && !v.IsNull() {
		n, isNum := v.Raw().(json.Number)
		f, err := n.Float64()
		if !isNum || err != nil || f != math.Trunc(f) || f < 1 || f > MaxExpiresInDays {
			return in, "expiresInDays must be a whole number from 1 to 3650, or null for no expiry"
		}
		in.ExpiresInDays = int(f)
	}
	return in, ""
}

// Issue creates a token for userID and returns the stored row and the
// plain token (shown once).
func Issue(ctx context.Context, st *store.Store, userID string, in CreateInput, now time.Time) (*store.APIToken, string, error) {
	plain, hash, prefix, err := authsvc.NewAPIToken()
	if err != nil {
		return nil, "", err
	}
	var expires *db.Time
	if in.ExpiresInDays > 0 {
		t := db.NewTime(now.Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour))
		expires = &t
	}
	row, err := st.CreateAPIToken(ctx, store.APITokenCreate{
		Name: in.Name, TokenHash: hash, Prefix: prefix, UserID: userID,
		Scope: in.Scope, AllowDangerous: in.AllowDangerous, ExpiresAt: expires,
	})
	if err != nil {
		return nil, "", err
	}
	return row, plain, nil
}

// POST / — create a token. Needs the step-up password; the plain token is
// in the response and nowhere else.
func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to create API token"
	in, msg := ParseCreate(httpx.BodyFrom(r))
	if msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !ok {
		return
	}
	u, _ := authsvc.UserFrom(r.Context())
	row, plain, err := Issue(r.Context(), h.d.Store, u.UserID, in, h.now())
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	h.d.Log.Info("[AUDIT] API token created: " + row.Name + " (" + row.Prefix + ", scope " + row.Scope +
		dangerousNote(row.AllowDangerous) + ") by " + u.Email + " from IP: " + httpx.ClientIP(r))
	httpx.JSON(w, http.StatusCreated, struct {
		Token    string `json:"token"`
		APIToken Item   `json:"apiToken"`
	}{plain, Item{row, StatusActive}})
}

func dangerousNote(on bool) string {
	if on {
		return ", dangerous actions allowed"
	}
	return ""
}

// DELETE /:id — revoke a token at once. Needs the step-up password.
func (h *handlers) revoke(w http.ResponseWriter, r *http.Request) {
	const failMsg = "Failed to revoke API token"
	id := chi.URLParam(r, "id")
	t, err := h.d.Store.GetAPIToken(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "API token not found")
		return
	}
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.fail(w, failMsg, err)
		return
	}
	if !ok {
		return
	}
	if err := h.d.Store.DeleteAPIToken(r.Context(), t.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, failMsg, err)
		return
	}
	u, _ := authsvc.UserFrom(r.Context())
	h.d.Log.Info("[AUDIT] API token revoked: " + t.Name + " (" + t.Prefix + ") by " + u.Email + " from IP: " + httpx.ClientIP(r))
	httpx.JSON(w, http.StatusOK, struct {
		Success bool `json:"success"`
	}{true})
}

func (h *handlers) fail(w http.ResponseWriter, msg string, err error) {
	h.d.Log.Error("[tokens] "+msg, "error", err)
	httpx.Error(w, http.StatusInternalServerError, msg)
}
