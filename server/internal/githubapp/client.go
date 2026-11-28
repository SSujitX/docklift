package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the GitHub REST API.
const DefaultBaseURL = "https://api.github.com"

// Limits applied to every GitHub API call.
const (
	// RequestTimeout bounds one call, response body included.
	RequestTimeout = 30 * time.Second
	// MaxResponseBytes bounds a response body; larger bodies are an error.
	MaxResponseBytes = 16 << 20
	// maxErrorBodyBytes is how much of a failed response is kept for the error message.
	maxErrorBodyBytes = 4 << 10
	maxRedirects      = 5
	apiVersion        = "2022-11-28"
	userAgent         = "Docklift-App"
)

// ErrFetchFailed wraps transport failures (DNS, TLS, timeouts, refused
// redirects). Its message is undici's "fetch failed" so callers that echo
// error messages to the client, as 2.x did, never expose network details.
var ErrFetchFailed = errors.New("fetch failed")

// ErrResponseTooLarge is returned when a response exceeds MaxResponseBytes.
var ErrResponseTooLarge = errors.New("GitHub response too large")

// StatusError is a non-2xx GitHub answer.
type StatusError struct {
	// Status is the HTTP status code; StatusText its reason phrase.
	Status     int
	StatusText string
	// Body is the start of the response body (at most a few KiB).
	Body string
	// msg is the error message, worded like 2.x.
	msg string
}

func (e *StatusError) Error() string { return e.msg }

// StatusOf returns the HTTP status of a *StatusError in err's chain, or 0.
func StatusOf(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status
	}
	return 0
}

// Account is the account an installation belongs to; nil fields were absent
// or null in GitHub's answer.
type Account struct {
	Login     *string `json:"login"`
	AvatarURL *string `json:"avatar_url"`
	Type      *string `json:"type"`
}

// Installation is an item of GET /app/installations.
type Installation struct {
	// ID is kept as the JSON number GitHub sent.
	ID      json.Number `json:"id"`
	Account *Account    `json:"account"`
}

// ManifestConversion is the answer of POST /app-manifests/{code}/conversions.
// It carries the App's secrets: never log it.
type ManifestConversion struct {
	ID            json.Number `json:"id"`
	Slug          string      `json:"slug"`
	Name          string      `json:"name"`
	ClientID      string      `json:"client_id"`
	ClientSecret  string      `json:"client_secret"`
	PEM           string      `json:"pem"`
	WebhookSecret string      `json:"webhook_secret"`
	Owner         *Account    `json:"owner"`
}

// AccessToken is an installation access token.
type AccessToken struct {
	Token string
	// ExpiresAt is zero when GitHub did not say.
	ExpiresAt time.Time
}

// API is the subset of the GitHub REST API Docklift uses. Client implements
// it against a configurable base URL so tests can point it at httptest.
type API interface {
	// ConvertManifest exchanges a manifest code for the new App's credentials.
	ConvertManifest(ctx context.Context, code string) (*ManifestConversion, error)
	// ListInstallations pages through GET /app/installations (App JWT).
	ListInstallations(ctx context.Context, appJWT string) ([]Installation, error)
	// GetInstallation is GET /app/installations/{id} (App JWT).
	GetInstallation(ctx context.Context, appJWT, installationID string) (*Installation, error)
	// CreateInstallationToken is POST /app/installations/{id}/access_tokens (App JWT).
	CreateInstallationToken(ctx context.Context, appJWT, installationID string) (AccessToken, error)
	// RepoInstallationID is GET /repos/{owner}/{repo}/installation (App JWT).
	RepoInstallationID(ctx context.Context, appJWT, owner, repo string) (string, error)
	// ListInstallationRepositories pages through GET /installation/repositories
	// (installation token); repositories are returned as decoded JSON objects.
	ListInstallationRepositories(ctx context.Context, token string) ([]map[string]any, error)
	// ListBranches is GET /repos/{owner}/{repo}/branches?per_page=100; token may
	// be empty for public repositories.
	ListBranches(ctx context.Context, token, owner, repo string) ([]any, error)
	// ListTags is one page of GET /repos/{owner}/{repo}/tags?per_page=100.
	ListTags(ctx context.Context, token, owner, repo string, page int) ([]any, error)
}

// Client is the HTTP implementation of API.
type Client struct {
	// BaseURL is the API root (DefaultBaseURL in production).
	BaseURL string
	// HTTP performs the requests; NewClient sets timeouts and a redirect policy
	// that never leaves the API host.
	HTTP *http.Client
}

// NewClient returns a Client for baseURL (DefaultBaseURL when empty).
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	base, _ := url.Parse(baseURL)
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: RequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return errors.New("too many redirects")
				}
				if base == nil || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
					return errors.New("redirect to another host refused")
				}
				return nil
			},
		},
	}
}

// PathSegment escapes s for use as one URL path segment. Dot segments are
// percent-encoded too so a value can never climb out of its position.
func PathSegment(s string) string {
	e := url.PathEscape(s)
	if e == "." || e == ".." {
		return strings.ReplaceAll(e, ".", "%2E")
	}
	return e
}

type authKind int

const (
	authNone authKind = iota
	authBearer
	authToken
)

// do performs one call and returns the body of a 2xx answer. A non-2xx answer
// is a *StatusError whose message comes from fail.
func (c *Client) do(ctx context.Context, method, path string, kind authKind, credential string, fail func(status int, statusText, body string) string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	switch {
	case kind == authBearer && credential != "":
		req.Header.Set("Authorization", "Bearer "+credential)
	case kind == authToken && credential != "":
		req.Header.Set("Authorization", "token "+credential)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		text := strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
		return nil, &StatusError{Status: resp.StatusCode, StatusText: text, Body: string(b), msg: fail(resp.StatusCode, text, string(b))}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}
	if len(b) > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	return b, nil
}

func decode(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON from GitHub: %w", err)
	}
	return nil
}

// isJSONArray reports whether b holds a JSON array (Array.isArray after
// response.json()).
func isJSONArray(b []byte) bool {
	t := bytes.TrimLeft(b, " \t\r\n")
	return len(t) > 0 && t[0] == '['
}

// ConvertManifest implements API.
func (c *Client) ConvertManifest(ctx context.Context, code string) (*ManifestConversion, error) {
	b, err := c.do(ctx, http.MethodPost, "/app-manifests/"+PathSegment(code)+"/conversions", authNone, "",
		func(status int, _, _ string) string {
			return fmt.Sprintf("GitHub manifest conversion failed: %d", status)
		})
	if err != nil {
		return nil, err
	}
	var m ManifestConversion
	if err := decode(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListInstallations implements API: pages of 100 until a short page, an
// empty page or a non-array answer.
func (c *Client) ListInstallations(ctx context.Context, appJWT string) ([]Installation, error) {
	var all []Installation
	for page := 1; ; page++ {
		b, err := c.do(ctx, http.MethodGet, "/app/installations?per_page=100&page="+strconv.Itoa(page), authBearer, appJWT,
			func(status int, _, _ string) string { return fmt.Sprintf("Failed to list installations: %d", status) })
		if err != nil {
			return nil, err
		}
		if !isJSONArray(b) {
			break
		}
		var batch []Installation
		if err := decode(b, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

// GetInstallation implements API.
func (c *Client) GetInstallation(ctx context.Context, appJWT, installationID string) (*Installation, error) {
	b, err := c.do(ctx, http.MethodGet, "/app/installations/"+PathSegment(installationID), authBearer, appJWT,
		func(status int, _, _ string) string { return fmt.Sprintf("Installation verify failed: %d", status) })
	if err != nil {
		return nil, err
	}
	var inst Installation
	if err := decode(b, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

// CreateInstallationToken implements API.
func (c *Client) CreateInstallationToken(ctx context.Context, appJWT, installationID string) (AccessToken, error) {
	b, err := c.do(ctx, http.MethodPost, "/app/installations/"+PathSegment(installationID)+"/access_tokens", authBearer, appJWT,
		func(_ int, _, body string) string { return "Failed to get installation token: " + body })
	if err != nil {
		return AccessToken{}, err
	}
	var data struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := decode(b, &data); err != nil {
		return AccessToken{}, err
	}
	if data.Token == "" {
		return AccessToken{}, errors.New("Failed to get installation token: no token in response")
	}
	t := AccessToken{Token: data.Token}
	if exp, err := time.Parse(time.RFC3339, data.ExpiresAt); err == nil {
		t.ExpiresAt = exp
	}
	return t, nil
}

// RepoInstallationID implements API.
func (c *Client) RepoInstallationID(ctx context.Context, appJWT, owner, repo string) (string, error) {
	b, err := c.do(ctx, http.MethodGet, "/repos/"+PathSegment(owner)+"/"+PathSegment(repo)+"/installation", authBearer, appJWT,
		func(status int, _, body string) string {
			return fmt.Sprintf("Could not find installation for %s/%s: %d %s", owner, repo, status, body)
		})
	if err != nil {
		return "", err
	}
	var data struct {
		ID json.Number `json:"id"`
	}
	if err := decode(b, &data); err != nil {
		return "", err
	}
	if data.ID == "" {
		return "", fmt.Errorf("Could not find installation for %s/%s: no id in response", owner, repo)
	}
	return data.ID.String(), nil
}

// ListInstallationRepositories implements API.
func (c *Client) ListInstallationRepositories(ctx context.Context, token string) ([]map[string]any, error) {
	var all []map[string]any
	for page := 1; ; page++ {
		b, err := c.do(ctx, http.MethodGet, "/installation/repositories?page="+strconv.Itoa(page)+"&per_page=100", authToken, token,
			func(status int, _, _ string) string {
				return fmt.Sprintf("GitHub repositories page %d failed: %d", page, status)
			})
		if err != nil {
			return nil, err
		}
		var data struct {
			Repositories []map[string]any `json:"repositories"`
		}
		if err := decode(b, &data); err != nil {
			return nil, err
		}
		if len(data.Repositories) == 0 {
			break
		}
		all = append(all, data.Repositories...)
		if len(data.Repositories) < 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) listRepoNames(ctx context.Context, path, token string) ([]any, error) {
	b, err := c.do(ctx, http.MethodGet, path, authToken, token,
		func(_ int, statusText, _ string) string { return "GitHub API Error: " + statusText })
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := decode(b, &items); err != nil {
		return nil, err
	}
	names := make([]any, 0, len(items))
	for _, it := range items {
		names = append(names, it["name"])
	}
	return names, nil
}

// ListBranches implements API; the result holds each branch's name.
func (c *Client) ListBranches(ctx context.Context, token, owner, repo string) ([]any, error) {
	return c.listRepoNames(ctx, "/repos/"+PathSegment(owner)+"/"+PathSegment(repo)+"/branches?per_page=100", token)
}

// ListTags implements API; the result holds each tag's name.
func (c *Client) ListTags(ctx context.Context, token, owner, repo string, page int) ([]any, error) {
	return c.listRepoNames(ctx, "/repos/"+PathSegment(owner)+"/"+PathSegment(repo)+"/tags?per_page=100&page="+strconv.Itoa(page), token)
}
