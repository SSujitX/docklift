package projects

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"docklift/internal/httpx"
)

// val is one request field with the JavaScript semantics the TS handlers
// relied on (undefined vs null, truthiness, String()). JSON bodies give
// decoded JSON values; multipart bodies give strings, or []any for a
// repeated field, as multer did.
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

// jsonFields is req.body after express.json: an object's keys, or no keys
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
func (v val) truthy() bool {
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
		f := numberValue(x)
		return f != 0 && !math.IsNaN(f)
	}
	return true
}

// jsString is String(v).
func (v val) jsString() string {
	if !v.defined {
		return "undefined"
	}
	return jsString(v.v)
}

func numberValue(n json.Number) float64 {
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil {
		// Out of float64 range: JSON.parse yields ±Infinity.
		if strings.HasPrefix(n.String(), "-") {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	return f
}

// jsString is String(x) for a decoded JSON value (nil is null).
func jsString(x any) string {
	switch t := x.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return jsNumberString(numberValue(t))
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			if e != nil {
				parts[i] = jsString(e)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// jsNumberString is String(n) for a JavaScript number.
func jsNumberString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	if abs := math.Abs(f); abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	n, _ := strconv.Atoi(exp)
	sign := "+"
	if n < 0 {
		sign, n = "-", -n
	}
	return mant + "e" + sign + strconv.Itoa(n)
}

// jsWhitespace reports whether r is removed by String.prototype.trim.
func jsWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// jsTrim is String.prototype.trim.
func jsTrim(s string) string { return strings.TrimFunc(s, jsWhitespace) }

// parseInt10 is parseInt(String(v), 10): leading whitespace, an optional
// sign, then the longest run of decimal digits; NaN when there is none.
func parseInt10(s string) float64 {
	s = strings.TrimLeftFunc(s, jsWhitespace)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		f = math.Inf(1)
	}
	if neg {
		f = -f
	}
	return f
}

// clampPort is Math.min(65535, Math.max(1, parseInt(v, 10) || 3000)).
func clampPort(v val) int {
	n := parseInt10(v.jsString())
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
	if s = jsTrim(s); s == "" {
		return "."
	}
	return s
}

// dockerfilePath is v ? String(v).trim() : null.
func dockerfilePath(v val) *string {
	if !v.truthy() {
		return nil
	}
	s := jsTrim(v.jsString())
	return &s
}
