// Package httpx holds the HTTP plumbing shared by every route group: JSON
// responses, request-body parsing, and the middleware
// stack (security headers, CORS, body limits, rate
// limits, maintenance gate, panic recovery) and the trusted-proxy model that
// decides when X-Forwarded-* headers are believed (proxy.go).
package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// ErrorBody is the `{"error": "..."}` shape every route uses.
type ErrorBody struct {
	Error string `json:"error"`
}

// InternalErrorMessage is the body of the generic 500 error response.
const InternalErrorMessage = "Something went wrong!"

// JSON writes v as the response body with the given status, encoded like
// JSON.stringify (no HTML escaping, no trailing newline,
// `application/json; charset=utf-8`).
func JSON(w http.ResponseWriter, status int, v any) {
	body, err := Marshal(v)
	if err != nil {
		slog.Error("encode JSON response", "error", err)
		status = http.StatusInternalServerError
		body = []byte(`{"error":"` + InternalErrorMessage + `"}`)
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Marshal encodes v like JSON.stringify (no HTML escaping, no newline).
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Error writes `{"error": msg}` with the given status.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorBody{Error: msg})
}

// InternalError writes the generic error response (500
// `{"error":"Something went wrong!"}`) after logging err.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	Error(w, http.StatusInternalServerError, InternalErrorMessage)
}

// NotFound answers unknown API paths with a JSON 404.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	Error(w, http.StatusNotFound, "Not found")
}
