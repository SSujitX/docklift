package js

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
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

// Number is the value JSON.parse gives a JSON number: beyond the float64
// range it is ±Infinity.
func Number(n json.Number) float64 {
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil && !math.IsInf(f, 0) {
		// Not JSON number syntax (a json.Number built by hand).
		return math.NaN()
	}
	return f
}

// NumberString is String(n) for a JavaScript number.
func NumberString(f float64) string {
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
	return exponent(f)
}

// exponent is Number.prototype.toString in exponent form ("1.5e+21").
func exponent(f float64) string {
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	n, _ := strconv.Atoi(exp)
	sign := "+"
	if n < 0 {
		sign, n = "-", -n
	}
	return mant + "e" + sign + strconv.Itoa(n)
}

var floatPrefix = regexp.MustCompile(`^[+-]?(?:Infinity|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)`)

// ParseFloat is parseFloat: the longest numeric prefix after leading white
// space, NaN when there is none.
func ParseFloat(s string) float64 {
	m := floatPrefix.FindString(TrimStart(s))
	if m == "" {
		return math.NaN()
	}
	if strings.TrimLeft(m, "+-") == "Infinity" {
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

// ParseInt is parseInt(s) without a radix: optional sign, a 0x prefix
// selects hexadecimal, digits up to the first invalid character, NaN when
// there are none.
func ParseInt(s string) float64 { return parseInt(s, true) }

// ParseInt10 is parseInt(s, 10): optional sign, then the longest run of
// decimal digits, NaN when there is none.
func ParseInt10(s string) float64 { return parseInt(s, false) }

func parseInt(s string, allowHex bool) float64 {
	t := TrimStart(s)
	sign := 1.0
	if t != "" && (t[0] == '+' || t[0] == '-') {
		if t[0] == '-' {
			sign = -1
		}
		t = t[1:]
	}
	base := 10
	if allowHex && len(t) >= 2 && t[0] == '0' && (t[1] == 'x' || t[1] == 'X') {
		base = 16
		t = t[2:]
	}
	end := 0
	for end < len(t) && digitValue(t[end]) < base {
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	// big.Int keeps every digit; Float64 rounds to nearest like JavaScript
	// and gives +Inf beyond the float64 range.
	n, _ := new(big.Int).SetString(t[:end], base)
	f, _ := new(big.Float).SetInt(n).Float64()
	return sign * f
}

// digitValue is the value of an ASCII digit or letter in bases up to 36.
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return math.MaxInt
}

// ToFixed is Number.prototype.toFixed(digits): the exact binary value is
// rounded half away from zero (Go's 'f' formatting rounds exact ties to
// even, e.g. 0.125 -> "0.12" where JavaScript gives "0.13"). Magnitudes of
// 1e21 and above use the exponent form, as toFixed does.
func ToFixed(v float64, digits int) string {
	switch {
	case math.IsNaN(v), math.IsInf(v, 0):
		return NumberString(v)
	case math.Abs(v) >= 1e21:
		return exponent(v)
	}
	r := new(big.Rat).SetFloat64(v)
	neg := r.Sign() < 0
	r.Abs(r)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	// n = floor(r + 1/2): ties go up (away from zero once the sign is restored).
	r.Add(r, big.NewRat(1, 2))
	n := new(big.Int).Quo(r.Num(), r.Denom())
	s := n.String()
	if digits > 0 {
		if len(s) <= digits {
			s = strings.Repeat("0", digits-len(s)+1) + s
		}
		s = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	if neg {
		// JavaScript keeps the sign even when the value rounds to zero:
		// (-0.001).toFixed(2) === "-0.00".
		s = "-" + s
	}
	return s
}

// RoundHalfUp is Math.round.
func RoundHalfUp(x float64) float64 { return math.Floor(x + 0.5) }
