package files

import "unicode"

// asciiPunctOrder is the primary order of ASCII punctuation and symbols in
// the ICU root collation (CLDR, variable weighting non-ignorable), which
// String.prototype.localeCompare uses. They sort before digits, digits
// before letters.
const asciiPunctOrder = "\t\n\v\f\r _-,;:!?.'\"()[]{}@*/\\&#%`^+<=>|~$"

var punctWeight = func() [128]int {
	var w [128]int
	for i, c := range asciiPunctOrder {
		w[c] = i + 1
	}
	return w
}()

const (
	digitBase  = 100
	letterBase = 200
	otherBase  = 1000
)

// primary is the primary collation weight of r: letter case is ignored.
func primary(r rune) int {
	switch {
	case r < 128 && punctWeight[r] > 0:
		return punctWeight[r]
	case r >= '0' && r <= '9':
		return digitBase + int(r-'0')
	case r >= 'a' && r <= 'z':
		return letterBase + int(r-'a')
	case r >= 'A' && r <= 'Z':
		return letterBase + int(r-'A')
	case r < 128:
		// Control characters other than whitespace are ignorable in ICU.
		return 0
	default:
		if base, ok := latin1Base(r); ok {
			return letterBase + int(base-'a')
		}
		return otherBase + int(unicode.ToLower(r))
	}
}

// latin1Letters maps U+00C0..U+00FF to the base letter that ICU sorts it
// with ('-' for the symbols × and ÷, and þ, which sort elsewhere).
const latin1Letters = "aaaaaaaceeeeiiiidnooooo-ouuuuy-saaaaaaaceeeeiiiidnooooo-ouuuuy-y"

// latin1Base is the base letter of an accented Latin-1 letter: accents
// differ from the base letter only at the secondary level.
func latin1Base(r rune) (rune, bool) {
	if r < 0xC0 || r > 0xFF {
		return 0, false
	}
	b := rune(latin1Letters[r-0xC0])
	return b, b != '-'
}

// localeCompare approximates a.localeCompare(b) under the ICU root locale for
// file names: primary weights first (punctuation < digits < letters, case
// ignored), then lower case before upper case at the first difference, then
// code point order. Non-ASCII characters sort after ASCII letters.
func localeCompare(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	pa, pb := primaries(ra), primaries(rb)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return cmpInt(pa[i], pb[i])
		}
	}
	if len(pa) != len(pb) {
		return cmpInt(len(pa), len(pb))
	}
	for i := 0; i < len(ra) && i < len(rb); i++ {
		la, lb := unicode.IsLower(ra[i]), unicode.IsLower(rb[i])
		if la != lb && unicode.ToLower(ra[i]) == unicode.ToLower(rb[i]) {
			if la {
				return -1
			}
			return 1
		}
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func primaries(rs []rune) []int {
	out := make([]int, 0, len(rs))
	for _, r := range rs {
		if w := primary(r); w != 0 {
			out = append(out, w)
		}
	}
	return out
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
