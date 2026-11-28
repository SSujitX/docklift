package httpx

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"docklift/internal/js"
)

// MaxBodyBytes is the JSON and urlencoded body limit (10 MiB).
const MaxBodyBytes = 10 * 1024 * 1024

// maxURLEncodedParams is body-parser's default parameterLimit.
const maxURLEncodedParams = 1000

type bodyKey struct{}

// Body is the parsed request body (JSON or urlencoded). Defined is false
// when no parser
// ran (no body, or a content type other than JSON/urlencoded), which is when
// the body is undefined. Raw holds the exact bytes of a JSON
// body (webhook signatures are computed over it).
type Body struct {
	Defined bool
	Raw     []byte
	value   any
}

// BodyFrom returns the body parsed by ParseBody.
func BodyFrom(r *http.Request) Body {
	b, _ := r.Context().Value(bodyKey{}).(Body)
	return b
}

// WithBody stores a parsed body on the request context (tests and handlers
// that parse bodies themselves).
func WithBody(ctx context.Context, b Body) context.Context {
	return context.WithValue(ctx, bodyKey{}, b)
}

// NewBody returns a defined body holding v, for bodies a handler parses
// itself (multipart text fields). v must be a
// decoded JSON value: nil, bool, json.Number, string, []any or
// map[string]any. Raw is empty.
func NewBody(v any) Body {
	return Body{Defined: true, value: v}
}

// Value returns the whole body as a JS value.
func (b Body) Value() Value {
	if !b.Defined {
		return Value{}
	}
	return Value{v: b.value, defined: true}
}

// Get returns req.body[key]; undefined when the body is not an object or the
// key is absent.
func (b Body) Get(key string) Value {
	return b.Value().Get(key)
}

// ParseBody is a JSON parse (limit 10mb) followed by
// a urlencoded parse (extended, limit 10mb). Parse failures are the
// client's mistake and end the request with a 4xx JSON error (2.x sent
// every body-parser error to its 500 handler; deliberate fix):
//
//	malformed JSON (or a JSON primitive, refused in strict mode)  400 {"error":"Invalid JSON body"}
//	body over MaxBodyBytes (before or after decompression)        413 {"error":"Request body too large"}
//	more than 1000 urlencoded parameters                          413 {"error":"Too many parameters"}
//	charset other than utf-8                                      415 {"error":"Unsupported charset"}
//	Content-Encoding other than identity/gzip/deflate             415 {"error":"Unsupported content encoding"}
//	corrupt compressed body or a failed read                      400 {"error":"Invalid request body"}
//
// The rest of a rejected body is drained (up to maxDrainBytes) before the
// response, as body-parser did, so clients that are still uploading receive
// the error instead of a connection reset. Reading a parsed body is bounded
// by BodyReadTimeout so a client trickling bytes cannot hold a request open
// indefinitely (the server sets no global ReadTimeout because of uploads).
//
// ParseBody holds at most MaxBodyBytes per request and shares
// SessionBodyBudget: mount it only behind an auth guard. Unauthenticated
// routes use ParseBodyWith with a small limit and PublicBodyBudget.
func ParseBody(next http.Handler) http.Handler {
	return ParseBodyWith(BodyOptions{})(next)
}

// BodyOptions configures ParseBodyWith. Zero fields take ParseBody's values.
type BodyOptions struct {
	// Limit is the largest accepted body in bytes, before and after
	// decompression (default MaxBodyBytes).
	Limit int64
	// Budget is shared by every parser that may run at once (default
	// SessionBodyBudget). A request whose body cannot be reserved within a
	// short wait gets 503 {"error": MsgServerBusy} with Retry-After.
	Budget *BodyBudget
	// ReadTimeout bounds reading the body (default BodyReadTimeout).
	ReadTimeout time.Duration
}

// ParseBodyWith is ParseBody with its own limit, budget and read timeout.
func ParseBodyWith(o BodyOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasBody(r) {
				next.ServeHTTP(w, r)
				return
			}
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || (mediaType != "application/json" && mediaType != "application/x-www-form-urlencoded") {
				next.ServeHTTP(w, r)
				return
			}
			raw, release, ok := o.read(w, r, params)
			if !ok {
				return
			}
			defer release()
			var body Body
			if mediaType == "application/json" {
				body, err = parseJSONBody(raw, true)
			} else {
				body, err = parseURLEncodedBody(raw)
			}
			if err != nil {
				// The body was read to the end; nothing is left to drain.
				rejectBody(w, r, err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			next.ServeHTTP(w, r.WithContext(WithBody(r.Context(), body)))
		})
	}
}

// ReadJSONBody reads the exact bytes of a JSON body without decoding them,
// for a handler that must authenticate the bytes first (the GitHub webhook
// HMAC) and decode them only afterwards with DecodeJSONBody. ok is false
// when the response has been written: a body over o.Limit, a saturated
// budget or a failed read. raw is nil for a request without a body or with
// a content type other than application/json. The caller must call release
// once it no longer holds raw.
func ReadJSONBody(w http.ResponseWriter, r *http.Request, o BodyOptions) (raw []byte, release func(), ok bool) {
	if !hasBody(r) {
		return nil, func() {}, true
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return nil, func() {}, true
	}
	return o.read(w, r, params)
}

// DecodeJSONBody decodes a body read by ReadJSONBody the way ParseBody
// does (strict mode, numbers kept as json.Number). Errors are *BodyError;
// answer them with WriteBodyError.
func DecodeJSONBody(raw []byte) (Body, error) {
	return parseJSONBody(raw, false)
}

// WriteBodyError answers a body error returned by DecodeJSONBody.
func WriteBodyError(w http.ResponseWriter, r *http.Request, err error) {
	rejectBody(w, r, err)
}

// read reserves budget for the body, then reads it under the read deadline.
// On failure it has answered the request.
func (o BodyOptions) read(w http.ResponseWriter, r *http.Request, params map[string]string) ([]byte, func(), bool) {
	limit, budget, timeout := o.Limit, o.Budget, o.ReadTimeout
	if limit <= 0 {
		limit = MaxBodyBytes
	}
	if budget == nil {
		budget = SessionBodyBudget
	}
	if timeout <= 0 {
		timeout = bodyReadTimeout
	}
	rc := http.NewResponseController(w)
	// Unsupported by test recorders; the deadline is best effort there.
	_ = rc.SetReadDeadline(time.Now().Add(timeout))
	fail := func(err error) ([]byte, func(), bool) {
		if rejectBody(w, r, err) {
			_ = rc.SetReadDeadline(time.Time{})
		}
		return nil, nil, false
	}
	if cs := strings.ToLower(params["charset"]); cs != "" && cs != "utf-8" {
		return fail(&BodyError{Status: http.StatusUnsupportedMediaType, Message: MsgUnsupportedCharset})
	}
	identity := isIdentity(r)
	if identity && r.ContentLength > limit {
		// Refused from the header alone: nothing is reserved or buffered.
		return fail(errBodyTooLarge)
	}
	reserve := limit
	if identity && r.ContentLength >= 0 {
		reserve = r.ContentLength
	}
	if !budget.Acquire(r.Context(), reserve, bodyBudgetWait) {
		// Nothing is buffered: the body is drained into io.Discard (as for
		// any rejected body) so the client still receives the answer.
		w.Header().Set("Retry-After", retryAfter(bodyBudgetWait))
		return fail(&BodyError{Status: http.StatusServiceUnavailable, Message: MsgServerBusy})
	}
	release := func() { budget.Release(reserve) }
	raw, err := readLimited(r, limit, identity)
	if err != nil {
		release()
		return fail(err)
	}
	_ = rc.SetReadDeadline(time.Time{})
	return raw, release, true
}

// isIdentity reports a body sent without Content-Encoding.
func isIdentity(r *http.Request) bool {
	enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	return enc == "" || enc == "identity"
}

// BodyError is a request body the server refuses; Status and Message are
// the response.
type BodyError struct {
	Status  int
	Message string
	// Err is the underlying cause (logged at debug level, never sent).
	Err error
}

func (e *BodyError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *BodyError) Unwrap() error { return e.Err }

// Body error messages.
const (
	MsgInvalidJSON         = "Invalid JSON body"
	MsgBodyTooLarge        = "Request body too large"
	MsgTooManyParameters   = "Too many parameters"
	MsgUnsupportedCharset  = "Unsupported charset"
	MsgUnsupportedEncoding = "Unsupported content encoding"
	MsgInvalidBody         = "Invalid request body"
)

var errBodyTooLarge = &BodyError{Status: http.StatusRequestEntityTooLarge, Message: MsgBodyTooLarge}

// maxDrainBytes bounds how much of a rejected body is read and discarded.
const maxDrainBytes = 64 << 20

// BodyReadTimeout bounds reading (and draining) a JSON or urlencoded body.
const BodyReadTimeout = 5 * time.Minute

// bodyReadTimeout is BodyReadTimeout (shortened by tests).
var bodyReadTimeout = BodyReadTimeout

// rejectBody drains what is left of the request body and answers err. It
// reports whether the body was read to the end; when it was not (a stalled
// or failed read, or more than maxDrainBytes left) the connection is closed
// after the response instead of being reused, and the caller must leave the
// read deadline in place so the server does not wait on the remainder.
func rejectBody(w http.ResponseWriter, r *http.Request, err error) bool {
	var be *BodyError
	if !errors.As(err, &be) {
		be = &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
	}
	slog.Debug("request body rejected", "method", r.Method, "path", r.URL.Path, "status", be.Status, "error", be.Error())
	_, drainErr := io.CopyN(io.Discard, r.Body, maxDrainBytes)
	drained := errors.Is(drainErr, io.EOF)
	if !drained {
		w.Header().Set("Connection", "close")
	}
	Error(w, be.Status, be.Message)
	return drained
}

// hasBody mirrors type-is hasBody: a Transfer-Encoding or Content-Length
// header means the request carries a body (Content-Length: 0 included).
func hasBody(r *http.Request) bool {
	return len(r.TransferEncoding) > 0 || r.ContentLength != 0 || r.Header.Get("Content-Length") != ""
}

// readLimited reads at most limit bytes of body (after decompression). An
// identity body of known length is read into one exactly sized buffer, so a
// body is never held twice while it is read.
func readLimited(r *http.Request, limit int64, identity bool) ([]byte, error) {
	var src io.Reader = r.Body
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); {
	case identity:
		if r.ContentLength > limit {
			return nil, errBodyTooLarge
		}
		if r.ContentLength >= 0 {
			raw := make([]byte, r.ContentLength)
			if _, err := io.ReadFull(r.Body, raw); err != nil {
				return nil, readError(err)
			}
			return raw, nil
		}
	case enc == "gzip":
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
		}
		defer zr.Close()
		src = zr
	case enc == "deflate":
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			return nil, &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
		}
		defer zr.Close()
		src = zr
	default:
		return nil, &BodyError{Status: http.StatusUnsupportedMediaType, Message: MsgUnsupportedEncoding}
	}
	raw, err := io.ReadAll(io.LimitReader(src, limit+1))
	if err != nil {
		return nil, readError(err)
	}
	if int64(len(raw)) > limit {
		return nil, errBodyTooLarge
	}
	return raw, nil
}

func readError(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return errBodyTooLarge
	}
	return &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
}

func invalidJSON(err error) error {
	return &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidJSON, Err: err}
}

// MaxJSONValues bounds the values (array elements, object keys and members)
// of a JSON body ParseBody decodes. Decoding into generic Go values costs
// tens of bytes per value, so without a bound a 10 MB body of tiny values
// (`[0,0,0,...]`) would take hundreds of megabytes; no dashboard request
// comes near it. Over the bound is 413 {"error": MsgBodyTooLarge}.
const MaxJSONValues = 100_000

// parseJSONBody decodes a strict-mode JSON body. bounded applies
// MaxJSONValues (the webhook decodes only after its signature is verified
// and takes GitHub's payloads whole).
func parseJSONBody(raw []byte, bounded bool) (Body, error) {
	if len(raw) == 0 {
		return Body{Defined: true, Raw: raw, value: map[string]any{}}, nil
	}
	// body-parser strict mode: only objects and arrays are accepted.
	first := bytes.TrimLeft(raw, " \t\n\r")
	if len(first) == 0 || (first[0] != '{' && first[0] != '[') {
		return Body{}, invalidJSON(errors.New("strict mode accepts only objects and arrays"))
	}
	// json.Valid scans without allocating; the decoder below then never
	// builds values for a body it would reject halfway.
	if !json.Valid(raw) {
		return Body{}, invalidJSON(nil)
	}
	if bounded && countJSONValues(raw) > MaxJSONValues {
		return Body{}, errBodyTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return Body{}, invalidJSON(err)
	}
	return Body{Defined: true, Raw: raw, value: v}, nil
}

// countJSONValues is an upper bound of the values in valid JSON: one plus
// every separator (`,` `:`) and container opening outside strings.
func countJSONValues(raw []byte) int {
	n, inString, escaped := 1, false, false
	for _, c := range raw {
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == ',' || c == ':' || c == '[' || c == '{':
			n++
		}
	}
	return n
}

// parseURLEncodedBody parses a form body into an object of strings (repeated
// keys become arrays, as in qs). Decoding is as lenient as qs: `+` is a
// space and a part whose percent-encoding is invalid is kept as sent.
// Bracket nesting (`a[b]=c`) is kept as a literal key.
func parseURLEncodedBody(raw []byte) (Body, error) {
	parts := strings.Split(string(raw), "&")
	if len(parts) > maxURLEncodedParams {
		return Body{}, &BodyError{Status: http.StatusRequestEntityTooLarge, Message: MsgTooManyParameters}
	}
	obj := map[string]any{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		key, val := qsDecode(k), qsDecode(v)
		switch prev := obj[key].(type) {
		case nil:
			obj[key] = val
		case string:
			obj[key] = []any{prev, val}
		case []any:
			obj[key] = append(prev, val)
		}
	}
	return Body{Defined: true, value: obj}, nil
}

// qsDecode is qs's default decoder: `+` to space, then decodeURIComponent,
// falling back to the undecoded string.
func qsDecode(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if decoded, err := url.PathUnescape(s); err == nil && utf8.ValidString(decoded) {
		return decoded
	}
	return s
}

// Value is a request value with JavaScript semantics: it distinguishes
// undefined (absent) from null, and offers truthiness and typeof checks the
// 2.x handlers relied on. Numbers are json.Number.
type Value struct {
	v       any
	defined bool
}

// Defined reports whether the value is not undefined.
func (v Value) Defined() bool { return v.defined }

// IsNull reports a JSON null.
func (v Value) IsNull() bool { return v.defined && v.v == nil }

// Raw returns the decoded Go value (nil, bool, json.Number, string, []any,
// map[string]any).
func (v Value) Raw() any { return v.v }

// Get is property access: v[key] for objects, undefined otherwise.
func (v Value) Get(key string) Value {
	obj, ok := v.v.(map[string]any)
	if !ok {
		return Value{}
	}
	x, ok := obj[key]
	return Value{v: x, defined: ok}
}

// Truthy is JavaScript truthiness.
func (v Value) Truthy() bool { return v.defined && js.Truthy(v.v) }

// String returns the value when typeof v === 'string'.
func (v Value) String() (string, bool) {
	s, ok := v.v.(string)
	return s, ok && v.defined
}

// Bool returns the value when typeof v === 'boolean'.
func (v Value) Bool() (bool, bool) {
	b, ok := v.v.(bool)
	return b, ok && v.defined
}

// Length is `v.length`: UTF-16 code units for strings, element count for
// arrays, a numeric `length` property of objects; ok is false where
// JavaScript yields undefined or a non-number.
func (v Value) Length() (float64, bool) {
	switch x := v.v.(type) {
	case string:
		return float64(UTF16Len(x)), true
	case []any:
		return float64(len(x)), true
	case map[string]any:
		if n, isNum := x["length"].(json.Number); isNum {
			f, err := n.Float64()
			return f, err == nil
		}
	}
	return 0, false
}

// UTF16Len is the JavaScript string length of s.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
