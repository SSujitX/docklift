package sysinfo

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Num is a JavaScript number in a JSON response: NaN and ±Infinity encode as
// null, as JSON.stringify does.
type Num float64

// MarshalJSON implements json.Marshaler.
func (n Num) MarshalJSON() ([]byte, error) {
	f := float64(n)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return []byte("null"), nil
	}
	return json.Marshal(f)
}

var floatPrefix = regexp.MustCompile(`^[+-]?(?:Infinity|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)`)

func trimJSLeft(s string) string {
	return strings.TrimLeftFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == 0xfeff })
}

// ParseFloat is JavaScript parseFloat: the longest numeric prefix after
// leading whitespace, NaN when there is none.
func ParseFloat(s string) float64 {
	m := floatPrefix.FindString(trimJSLeft(s))
	if m == "" {
		return math.NaN()
	}
	switch strings.TrimLeft(m, "+-") {
	case "Infinity":
		if strings.HasPrefix(m, "-") {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	// The prefix is always valid syntax; out-of-range values come back as
	// ±Inf or 0 with a range error, which is what JavaScript yields too.
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

// ParseInt is JavaScript parseInt(s) without a radix: optional sign, a 0x
// prefix selects hexadecimal, digits up to the first invalid character, NaN
// when there are none.
func ParseInt(s string) float64 {
	t := trimJSLeft(s)
	sign := 1.0
	if t != "" && (t[0] == '+' || t[0] == '-') {
		if t[0] == '-' {
			sign = -1
		}
		t = t[1:]
	}
	base := 10.0
	if len(t) >= 2 && t[0] == '0' && (t[1] == 'x' || t[1] == 'X') {
		base = 16
		t = t[2:]
	}
	n, digits := 0.0, 0
	for _, r := range t {
		d := -1
		switch {
		case r >= '0' && r <= '9':
			d = int(r - '0')
		case r >= 'a' && r <= 'f':
			d = int(r-'a') + 10
		case r >= 'A' && r <= 'F':
			d = int(r-'A') + 10
		}
		if d < 0 || float64(d) >= base {
			break
		}
		n = n*base + float64(d)
		digits++
	}
	if digits == 0 {
		return math.NaN()
	}
	return sign * n
}

// ToFixed is parseFloat(x.toFixed(digits)): x rounded to digits decimals,
// ties away from zero on the exact binary value (JavaScript semantics, which
// differ from strconv's round-half-even on exact ties such as 0.25).
func ToFixed(x float64, digits int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) >= 1e21 {
		return x
	}
	// 1100 fractional digits hold the exact decimal expansion of any float64.
	exact := new(big.Float).SetFloat64(math.Abs(x)).Text('f', 1100)
	intPart, frac, _ := strings.Cut(exact, ".")
	keep := intPart + frac[:digits]
	roundUp := frac[digits] >= '5'
	d := []byte(keep)
	if roundUp {
		i := len(d) - 1
		for ; i >= 0; i-- {
			if d[i] == '9' {
				d[i] = '0'
				continue
			}
			d[i]++
			break
		}
		if i < 0 {
			d = append([]byte{'1'}, d...)
		}
	}
	s := string(d)
	if digits > 0 {
		s = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	f, _ := strconv.ParseFloat(s, 64)
	if x < 0 {
		return -f
	}
	return f
}

// RoundHalfUp is Math.round.
func RoundHalfUp(x float64) float64 {
	return math.Floor(x + 0.5)
}

// FormatNumber is String(n) for the numbers the API prints (GHz values,
// counts): the shortest round-trip form without an exponent below 1e21.
func FormatNumber(n float64) string {
	if math.IsNaN(n) {
		return "NaN"
	}
	if math.Abs(n) < 1e21 {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// Substring is s.substring(0, n) on UTF-16 code units.
func Substring(s string, n int) string {
	units := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if w < 0 {
			w = 1
		}
		if units+w > n {
			return s[:i]
		}
		units += w
	}
	return s
}
