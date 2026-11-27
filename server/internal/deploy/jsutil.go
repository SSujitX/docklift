package deploy

import (
	"math"
	"strconv"
	"strings"
)

// jsWhitespace reports whether r is removed by String.prototype.trim
// (WhiteSpace and LineTerminator of ECMA-262).
func jsWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// jsTrim is String.prototype.trim.
func jsTrim(s string) string { return strings.TrimFunc(s, jsWhitespace) }

// jsLower is String.prototype.toLowerCase for the purpose of Docker slugs.
// Go's per-rune simple mapping matches JavaScript's full mapping except for
// U+0130 (İ), which JavaScript lowers to "i̇".
func jsLower(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "İ", "i̇"))
}

// jsSlice is s.slice(0, n) on UTF-16 code units (JavaScript string indices).
func jsSlice(s string, n int) string {
	units := 0
	for i, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if units+w > n {
			if units < n {
				// slice() splits the surrogate pair; the lone high surrogate
				// encodes as U+FFFD in UTF-8, as Node writes it.
				return s[:i] + "\uFFFD"
			}
			return s[:i]
		}
		units += w
	}
	return s
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
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
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

// jsString is String(v) for a value decoded from JSON (nil is null).
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return jsNumberString(x)
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
