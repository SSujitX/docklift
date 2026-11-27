package httpx

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf16"
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
// express.urlencoded({extended: true, limit: '10mb'}). Any parse failure
// (malformed JSON, oversize body, unsupported encoding or charset) ends the
// request with the Express error-handler response, as body-parser errors did.
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
		if cs := strings.ToLower(params["charset"]); cs != "" && cs != "utf-8" {
			InternalError(w, r, fmt.Errorf("unsupported charset %q", cs))
			return
		}
		raw, err := readLimited(r)
		if err != nil {
			InternalError(w, r, err)
			return
		}
		var body Body
		if mediaType == "application/json" {
			body, err = parseJSONBody(raw)
		} else {
			body, err = parseURLEncodedBody(raw)
		}
		if err != nil {
			InternalError(w, r, err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(w, r.WithContext(WithBody(r.Context(), body)))
	})
}

// hasBody mirrors type-is hasBody: a Transfer-Encoding or Content-Length
// header means the request carries a body (Content-Length: 0 included).
func hasBody(r *http.Request) bool {
	return len(r.TransferEncoding) > 0 || r.ContentLength != 0 || r.Header.Get("Content-Length") != ""
}

var errBodyTooLarge = errors.New("request entity too large")

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
			return nil, fmt.Errorf("gzip body: %w", err)
		}
		defer zr.Close()
		src = zr
	case "deflate":
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			return nil, fmt.Errorf("deflate body: %w", err)
		}
		defer zr.Close()
		src = zr
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", enc)
	}
	raw, err := io.ReadAll(io.LimitReader(src, MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(raw) > MaxBodyBytes {
		return nil, errBodyTooLarge
	}
	return raw, nil
}

func parseJSONBody(raw []byte) (Body, error) {
	if len(raw) == 0 {
		return Body{Defined: true, Raw: raw, value: map[string]any{}}, nil
	}
	// body-parser strict mode: only objects and arrays are accepted.
	first := bytes.TrimLeft(raw, " \t\n\r")
	if len(first) == 0 || (first[0] != '{' && first[0] != '[') {
		return Body{}, errors.New("invalid JSON body: strict mode accepts only objects and arrays")
	}
	if !json.Valid(raw) {
		return Body{}, errors.New("invalid JSON body")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return Body{}, fmt.Errorf("invalid JSON body: %w", err)
	}
	return Body{Defined: true, Raw: raw, value: v}, nil
}

// parseURLEncodedBody parses a form body into an object of strings (repeated
// keys become arrays, as in qs). Bracket nesting (`a[b]=c`) is kept as a
// literal key.
func parseURLEncodedBody(raw []byte) (Body, error) {
	if strings.Count(string(raw), "&")+1 > maxURLEncodedParams {
		return Body{}, errors.New("too many parameters")
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return Body{}, fmt.Errorf("invalid urlencoded body: %w", err)
	}
	obj := make(map[string]any, len(values))
	for k, vs := range values {
		if len(vs) == 1 {
			obj[k] = vs[0]
			continue
		}
		arr := make([]any, len(vs))
		for i, s := range vs {
			arr[i] = s
		}
		obj[k] = arr
	}
	return Body{Defined: true, value: obj}, nil
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
