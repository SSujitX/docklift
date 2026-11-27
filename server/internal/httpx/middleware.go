package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
)

// SecurityHeaders sets the headers of the first middleware in index.ts. HSTS
// is sent only when the request is HTTPS: the raw X-Forwarded-Proto header
// equals "https", or there is no such header and the connection uses TLS.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		proto := strings.Join(r.Header.Values("X-Forwarded-Proto"), ", ")
		if proto == "" {
			proto = RequestProtocol(r)
		}
		if proto == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// corsMethods is the cors package default methods list.
const corsMethods = "GET,HEAD,PUT,PATCH,POST,DELETE"

// CORS reproduces cors({origin: callback, credentials: true}) from index.ts.
// Requests without Origin are allowed (the JWT is still the gate) and get
// Vary/credentials headers but no Allow-Origin; trusted origins are
// reflected; untrusted origins get no CORS headers and continue unchanged.
// Allowed preflights are answered with 204.
func CORS(allow []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && !IsTrustedOrigin(origin, r, RequestProtocol(r), allow) {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			if origin != "" {
				h.Set("Access-Control-Allow-Origin", origin)
			}
			addVary(h, "Origin")
			h.Set("Access-Control-Allow-Credentials", "true")
			if r.Method != http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Methods", corsMethods)
			addVary(h, "Access-Control-Request-Headers")
			if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
				h.Set("Access-Control-Allow-Headers", reqHeaders)
			}
			h.Set("Content-Length", "0")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// addVary appends a field to Vary unless already listed (the vary package).
func addVary(h http.Header, field string) {
	current := h.Get("Vary")
	if current == "*" {
		return
	}
	for _, f := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(f), field) {
			return
		}
	}
	if current == "" {
		h.Set("Vary", field)
		return
	}
	h.Set("Vary", current+", "+field)
}

// Recover turns a handler panic into the Express error-handler response
// (500 `{"error":"Something went wrong!"}`), logging the stack.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p)
			}
			slog.Error("panic serving request", "method", r.Method, "path", r.URL.Path,
				"panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			Error(w, http.StatusInternalServerError, InternalErrorMessage)
		}()
		next.ServeHTTP(w, r)
	})
}
