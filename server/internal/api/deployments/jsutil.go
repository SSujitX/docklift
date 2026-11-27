package deployments

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"

	"docklift/internal/httpx"
)

// jsObject is a JSON object that keeps JavaScript key order (first
// assignment fixes the position, a later assignment replaces the value), for
// the maps the TS routes built with `obj[key] = value`.
type jsObject[V any] struct {
	keys []string
	vals map[string]V
}

func (o *jsObject[V]) set(key string, v V) {
	if o.vals == nil {
		o.vals = map[string]V{}
	}
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

// MarshalJSON writes the object in key order without HTML escaping.
func (o jsObject[V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := httpx.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := httpx.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// jsParseInt is JavaScript parseInt(s, 10): leading whitespace, an optional
// sign, then decimal digits; ok is false for NaN.
func jsParseInt(s string) (int, bool) {
	s = strings.TrimLeftFunc(s, isJSSpace)
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
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		n = math.MaxInt32
	}
	if neg {
		n = -n
	}
	return n, true
}

// jsString is String(v) for a decoded JSON value.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return x.String()
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			if e != nil {
				parts[i] = jsString(e)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// queryString is req.query[key] coerced the way the TS route used it: a
// repeated parameter is an array, which String() joins with commas.
func queryString(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	return strings.Join(values, ","), true
}

// isJSSpace is the leading whitespace parseInt skips.
func isJSSpace(r rune) bool { return unicode.IsSpace(r) || r == 0xfeff }
