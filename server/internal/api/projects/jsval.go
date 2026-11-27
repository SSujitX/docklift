package projects

import (
	"math"

	"docklift/internal/httpx"
	"docklift/internal/js"
)

// val is one request field with the JavaScript semantics the 2.x handlers
// relied on (undefined vs null, truthiness, String()). JSON bodies give
// decoded JSON values; multipart bodies give strings, or []any for a
// repeated field, as 2.x did.
type val struct {
	v       any
	defined bool
}

// fields is req.body: a JSON object, a multipart form, or nothing.
type fields map[string]any

func (f fields) get(key string) val {
	v, ok := f[key]
	return val{v: v, defined: ok}
}

// jsonFields is the parsed JSON body: an object's keys, or no keys
// for anything else (undefined body, arrays), so destructuring never throws.
func jsonFields(b httpx.Body) fields {
	if m, ok := b.Value().Raw().(map[string]any); ok {
		return m
	}
	return fields{}
}

func (v val) isNull() bool { return v.defined && v.v == nil }

// nullish is `v == null` (undefined or null).
func (v val) nullish() bool { return !v.defined || v.v == nil }

func (v val) str() (string, bool) {
	s, ok := v.v.(string)
	return s, ok && v.defined
}

// truthy is JavaScript truthiness.
func (v val) truthy() bool { return v.defined && js.Truthy(v.v) }

// jsString is String(v).
func (v val) jsString() string {
	if !v.defined {
		return "undefined"
	}
	return js.String(v.v)
}

// clampPort is Math.min(65535, Math.max(1, parseInt(v, 10) || 3000)).
func clampPort(v val) int {
	n := js.ParseInt10(v.jsString())
	if n == 0 || math.IsNaN(n) {
		n = 3000
	}
	return int(math.Min(65535, math.Max(1, n)))
}

// baseDirectory is String(v || '.').trim() || '.'.
func baseDirectory(v val) string {
	s := "."
	if v.truthy() {
		s = v.jsString()
	}
	if s = js.Trim(s); s == "" {
		return "."
	}
	return s
}

// dockerfilePath is v ? String(v).trim() : null.
func dockerfilePath(v val) *string {
	if !v.truthy() {
		return nil
	}
	s := js.Trim(v.jsString())
	return &s
}
