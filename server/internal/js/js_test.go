package js

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTrimAndSpace(t *testing.T) {
	if got := Trim("\ufeff \u00a0 x y\u2028\u3000"); got != "x y" {
		t.Errorf("Trim = %q", got)
	}
	if got := TrimStart("\t x "); got != "x " {
		t.Errorf("TrimStart = %q", got)
	}
	// U+0085 is Unicode white space but not JavaScript white space.
	if IsSpace(0x85) || !IsSpace(0x200a) || IsSpace(0x200b) {
		t.Error("IsSpace")
	}
	if got := Lower("ABC\u0130"); got != "abci\u0307" {
		t.Errorf("Lower = %q", got)
	}
}

func TestSliceAndSubstring(t *testing.T) {
	cases := []struct {
		in        string
		n         int
		slice     string
		substring string
	}{
		{"abcdef", 3, "abc", "abc"},
		{"ab🚀cd", 3, "ab\ufffd", "ab"},
		{"ab🚀cd", 4, "ab🚀", "ab🚀"},
		{"short", 50, "short", "short"},
		{"", 2, "", ""},
	}
	for _, c := range cases {
		if got := Slice(c.in, c.n); got != c.slice {
			t.Errorf("Slice(%q, %d) = %q, want %q", c.in, c.n, got, c.slice)
		}
		if got := Substring(c.in, c.n); got != c.substring {
			t.Errorf("Substring(%q, %d) = %q, want %q", c.in, c.n, got, c.substring)
		}
	}
}

func TestNumberString(t *testing.T) {
	for f, want := range map[float64]string{
		0: "0", -0.5: "-0.5", 100: "100", 1e21: "1e+21", 1.5e21: "1.5e+21", 1.25e-7: "1.25e-7",
		1e-6: "0.000001", 123456789012: "123456789012", math.Inf(1): "Infinity", math.Inf(-1): "-Infinity",
	} {
		if got := NumberString(f); got != want {
			t.Errorf("NumberString(%v) = %q, want %q", f, got, want)
		}
	}
	if NumberString(math.NaN()) != "NaN" || NumberString(math.Copysign(0, -1)) != "0" {
		t.Error("NaN / -0")
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "null"}, {"a.b", "a.b"}, {true, "true"}, {false, "false"},
		{json.Number("7"), "7"}, {json.Number("8080"), "8080"}, {json.Number("1.50"), "1.5"},
		{json.Number("1e-7"), "1e-7"}, {json.Number("0.000001"), "0.000001"}, {json.Number("-0"), "0"},
		{json.Number("1e21"), "1e+21"}, {json.Number("1e400"), "Infinity"},
		{float64(42), "42"}, {1.5e21, "1.5e+21"}, {1e-7, "1e-7"},
		{[]any{"a", nil, json.Number("2"), float64(1)}, "a,,2,1"},
		{[]any{[]any{"x", "y"}, "z"}, "x,y,z"},
		{map[string]any{}, "[object Object]"},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruthy(t *testing.T) {
	truthy := []any{"x", true, json.Number("1"), json.Number("-0.5"), json.Number("1e400"), float64(2), []any{}, map[string]any{}}
	falsy := []any{nil, "", false, json.Number("0"), json.Number("-0"), float64(0), math.NaN()}
	for _, v := range truthy {
		if !Truthy(v) {
			t.Errorf("%#v should be truthy", v)
		}
	}
	for _, v := range falsy {
		if Truthy(v) {
			t.Errorf("%#v should be falsy", v)
		}
	}
}

func TestParseFloat(t *testing.T) {
	floats := map[string]float64{"3.20GHz": 3.2, "  2400.000": 2400, "-1.5e3x": -1500, ".5": 0.5, "Infinity": math.Inf(1), "-Infinityx": math.Inf(-1)}
	for in, want := range floats {
		if got := ParseFloat(in); got != want {
			t.Errorf("ParseFloat(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "abc", "-", ".", "NaN"} {
		if !math.IsNaN(ParseFloat(in)) {
			t.Errorf("ParseFloat(%q) must be NaN", in)
		}
	}
}

func TestParseInt(t *testing.T) {
	ints := map[string]float64{"16318412": 16318412, " 42 kB": 42, "-7": -7, "0x1A": 26, "12.9": 12, "77abc": 77, "1e3": 1}
	for in, want := range ints {
		if got := ParseInt(in); got != want {
			t.Errorf("ParseInt(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"kB", "", "-", "0x", "+"} {
		if !math.IsNaN(ParseInt(in)) {
			t.Errorf("ParseInt(%q) must be NaN", in)
		}
	}
}

func TestParseInt10(t *testing.T) {
	ints := map[string]float64{
		"10": 10, "  7px": 7, "-3": -3, "+4": 4, "1e3": 1, "\u00a0 42abc": 42, "0x1A": 0, "443.9": 443,
		"99999999999999999999999": 1e23,
	}
	for in, want := range ints {
		if got := ParseInt10(in); got != want {
			t.Errorf("ParseInt10(%q) = %v, want %v", in, got, want)
		}
	}
	if got := ParseInt10("-0"); got != 0 || !math.Signbit(got) {
		t.Errorf("ParseInt10(-0) = %v, want -0", got)
	}
	for _, in := range []string{"abc", "", "-x", "x1", "-"} {
		if !math.IsNaN(ParseInt10(in)) {
			t.Errorf("ParseInt10(%q) must be NaN", in)
		}
	}
}

func TestToFixed(t *testing.T) {
	cases := []struct {
		v      float64
		digits int
		want   string
	}{
		{0, 2, "0.00"},
		{0.125, 2, "0.13"}, // exact tie: JavaScript rounds up, Go's 'f' gives 0.12
		{0.375, 2, "0.38"},
		{1.005, 2, "1.00"}, // 1.005 is 1.00499999... in binary
		{12.3456, 2, "12.35"},
		{-0.001, 2, "-0.00"},
		{-1.125, 2, "-1.13"},
		{2.5, 0, "3"},
		{-2.5, 0, "-3"},
		{0.25, 1, "0.3"},
		{0.35, 1, "0.3"}, // 0.34999... in binary
		{99.95, 1, "100.0"},
		{123.4, 0, "123"},
		{0.5, 2, "0.50"},
		{0.0001, 2, "0.00"},
		{1e21, 2, "1e+21"},
		{math.NaN(), 2, "NaN"},
		{math.Inf(-1), 2, "-Infinity"},
	}
	for _, c := range cases {
		if got := ToFixed(c.v, c.digits); got != c.want {
			t.Errorf("ToFixed(%v, %d) = %q, want %q", c.v, c.digits, got, c.want)
		}
	}
}

func TestRoundHalfUp(t *testing.T) {
	for in, want := range map[float64]float64{2.5: 3, -2.5: -2, 2.4: 2, 0: 0} {
		if got := RoundHalfUp(in); got != want {
			t.Errorf("RoundHalfUp(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestNumJSON(t *testing.T) {
	b, _ := json.Marshal([]Num{Num(math.NaN()), Num(math.Inf(1)), 12, 1.5, 16710533120})
	if string(b) != "[null,null,12,1.5,16710533120]" {
		t.Fatalf("got %s", b)
	}
}

func TestEncodeURIComponent(t *testing.T) {
	for in, want := range map[string]string{
		"abc-_.!~*'()": "abc-_.!~*'()",
		"a b/c?d=e&f":  "a%20b%2Fc%3Fd%3De%26f",
		"p@ss:w0rd#":   "p%40ss%3Aw0rd%23",
		"\u00e9":       "%C3%A9",
		"":             "",
	} {
		if got := EncodeURIComponent(in); got != want {
			t.Errorf("EncodeURIComponent(%q) = %q, want %q", in, got, want)
		}
	}
}
