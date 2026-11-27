package deployments

import (
	"bytes"
	"math"
	"strings"

	"docklift/internal/httpx"
	"docklift/internal/js"
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

// parseIntParam is parseInt(s, 10) for a limit/offset query value; ok is
// false for NaN. Values beyond the int32 range clamp to it (Prisma's take
// and skip are 32-bit).
func parseIntParam(s string) (int, bool) {
	f := js.ParseInt10(s)
	if math.IsNaN(f) {
		return 0, false
	}
	return int(math.Max(math.MinInt32, math.Min(math.MaxInt32, f))), true
}

// queryString is req.query[key] coerced the way the TS route used it: a
// repeated parameter is an array, which String() joins with commas.
func queryString(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	return strings.Join(values, ","), true
}
