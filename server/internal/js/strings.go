// Package js reproduces the JavaScript coercions the 2.x backend
// relied on (String(), truthiness, trim, parseInt/parseFloat, toFixed,
// UTF-16 string indices), so ported handlers produce byte-identical output.
// Values are those decoded from JSON: nil, bool, string, json.Number (what
// httpx yields), float64 (encoding/json without UseNumber), []any and
// map[string]any.
package js

import (
	"strings"
	"unicode/utf16"
)

// IsSpace reports whether r is JavaScript white space or a line terminator
// (WhiteSpace and LineTerminator of ECMA-262): what trim() removes and what
// parseInt/parseFloat skip.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// Trim is String.prototype.trim.
func Trim(s string) string { return strings.TrimFunc(s, IsSpace) }

// TrimStart is String.prototype.trimStart.
func TrimStart(s string) string { return strings.TrimLeftFunc(s, IsSpace) }

// Lower is String.prototype.toLowerCase. Go's per-rune simple mapping
// matches JavaScript's full mapping except for U+0130 (capital I with dot
// above), which JavaScript lowers to "i" followed by U+0307.
func Lower(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "\u0130", "i\u0307"))
}

// Slice is s.slice(0, n) on UTF-16 code units. When n falls inside a
// surrogate pair, the lone high surrogate that JavaScript keeps is written
// as U+FFFD, as UTF-8 encoding does.
func Slice(s string, n int) string {
	units := 0
	for i, r := range s {
		w := runeUnits(r)
		if units+w > n {
			if units < n {
				return s[:i] + "\ufffd"
			}
			return s[:i]
		}
		units += w
	}
	return s
}

// Substring is s.substring(0, n) for text that is only logged or compared:
// a surrogate pair split by n is dropped entirely.
func Substring(s string, n int) string {
	units := 0
	for i, r := range s {
		w := runeUnits(r)
		if units+w > n {
			return s[:i]
		}
		units += w
	}
	return s
}

func runeUnits(r rune) int {
	if w := utf16.RuneLen(r); w > 0 {
		return w
	}
	return 1
}

// EncodeURIComponent is encodeURIComponent: A-Z a-z 0-9 - _ . ! ~ * ' ( )
// are kept, every other byte of the UTF-8 encoding becomes %XX.
func EncodeURIComponent(s string) string {
	const upperHex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}
