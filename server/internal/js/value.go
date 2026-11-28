package js

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// String is String(v) for a decoded JSON value (nil is null).
func String(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return NumberString(Number(x))
	case float64:
		return NumberString(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			if e != nil {
				parts[i] = String(e)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// Truthy is JavaScript truthiness for a decoded JSON value.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		f := Number(x)
		return f != 0 && !math.IsNaN(f)
	case float64:
		return x != 0 && !math.IsNaN(x)
	}
	return true
}
