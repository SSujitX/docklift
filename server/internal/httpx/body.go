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
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxBodyBytes is the express.json / express.urlencoded limit ('10mb').
const MaxBodyBytes = 10 * 1024 * 1024

// maxURLEncodedParams is body-parser's default parameterLimit.
const maxURLEncodedParams = 1000

type bodyKey struct{}

// Body is the parsed request body, the equivalent of Express req.body after
// express.json() and express.urlencoded(). Defined is false when no parser
// ran (no body, or a content type other than JSON/urlencoded), which is when
// req.body was undefined in Express 5. Raw holds the exact bytes of a JSON
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

// ParseBody is express.json({limit: '10mb'}) followed by
// express.urlencoded({extended: true, limit: '10mb'}). Parse failures are the
// client's mistake and end the request with a 4xx JSON error (Express sent
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
func ParseBody(next http.Handler) http.Handler {
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
		rc := http.NewResponseController(w)
		// Unsupported by test recorders; the deadline is best effort there.
		_ = rc.SetReadDeadline(time.Now().Add(bodyReadTimeout))
		body, raw, err := readBody(r, mediaType, params)
		if err != nil {
			if rejectBody(w, r, err) {
				_ = rc.SetReadDeadline(time.Time{})
			}
			return
		}
		_ = rc.SetReadDeadline(time.Time{})
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(w, r.WithContext(WithBody(r.Context(), body)))
	})
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

func readBody(r *http.Request, mediaType string, params map[string]string) (Body, []byte, error) {
	if cs := strings.ToLower(params["charset"]); cs != "" && cs != "utf-8" {
		return Body{}, nil, &BodyError{Status: http.StatusUnsupportedMediaType, Message: MsgUnsupportedCharset}
	}
	raw, err := readLimited(r)
	if err != nil {
		return Body{}, nil, err
	}
	var body Body
	if mediaType == "application/json" {
		body, err = parseJSONBody(raw)
	} else {
		body, err = parseURLEncodedBody(raw)
	}
	return body, raw, err
}

// hasBody mirrors type-is hasBody: a Transfer-Encoding or Content-Length
// header means the request carries a body (Content-Length: 0 included).
func hasBody(r *http.Request) bool {
	return len(r.TransferEncoding) > 0 || r.ContentLength != 0 || r.Header.Get("Content-Length") != ""
}

func readLimited(r *http.Request) ([]byte, error) {
	var src io.Reader = r.Body
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
		if r.ContentLength > MaxBodyBytes {
			return nil, errBodyTooLarge
		}
	case "gzip":
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
		}
		defer zr.Close()
		src = zr
	case "deflate":
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			return nil, &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
		}
		defer zr.Close()
		src = zr
	default:
		return nil, &BodyError{Status: http.StatusUnsupportedMediaType, Message: MsgUnsupportedEncoding}
	}
	raw, err := io.ReadAll(io.LimitReader(src, MaxBodyBytes+1))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, errBodyTooLarge
		}
		return nil, &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidBody, Err: err}
	}
	if len(raw) > MaxBodyBytes {
		return nil, errBodyTooLarge
	}
	return raw, nil
}

func invalidJSON(err error) error {
	return &BodyError{Status: http.StatusBadRequest, Message: MsgInvalidJSON, Err: err}
}

func parseJSONBody(raw []byte) (Body, error) {
	if len(raw) == 0 {
		return Body{Defined: true, Raw: raw, value: map[string]any{}}, nil
	}
	// body-parser strict mode: only objects and arrays are accepted.
	first := bytes.TrimLeft(raw, " \t\n\r")
	if len(first) == 0 || (first[0] != '{' && first[0] != '[') {
		return Body{}, invalidJSON(errors.New("strict mode accepts only objects and arrays"))
	}
	if !json.Valid(raw) {
		return Body{}, invalidJSON(nil)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return Body{}, invalidJSON(err)
	}
	return Body{Defined: true, Raw: raw, value: v}, nil
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
// TypeScript handlers relied on. Numbers are json.Number.
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
func (v Value) Truthy() bool {
	if !v.defined {
		return false
	}
	switch x := v.v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		f, err := x.Float64()
		return err != nil || (f != 0 && !math.IsNaN(f))
	default:
		return true
	}
}

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
