package contract

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Structural JSON matching.
//
// Expected values:
//   - Obj{...}      object with EXACTLY these keys (Optional(m) marks a key that may be absent)
//   - Arr{...}      array with exactly these elements, in order
//   - string, int, float64, bool, nil   literal (numbers compare by value)
//   - Matcher       volatile values: UUID, ISOms, JWT, Hex(64), Str, Num, ...
type Obj map[string]any
type Arr []any

type Matcher interface {
	Match(v any) error
	String() string
}

type matcherFunc struct {
	name string
	fn   func(any) error
}

func (m matcherFunc) Match(v any) error { return m.fn(v) }
func (m matcherFunc) String() string    { return m.name }

func mf(name string, fn func(any) error) Matcher { return matcherFunc{name, fn} }

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return fmt.Sprintf("%T", v)
}

func strMatcher(name string, ok func(string) bool) Matcher {
	return mf(name, func(v any) error {
		s, isStr := v.(string)
		if !isStr {
			return fmt.Errorf("want %s, got %s %v", name, typeName(v), v)
		}
		if !ok(s) {
			return fmt.Errorf("want %s, got %q", name, s)
		}
		return nil
	})
}

var (
	reUUID  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	reISOms = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	reJWT   = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
	reHex   = regexp.MustCompile(`^[0-9a-f]+$`)
)

var (
	Any      = mf("any value", func(any) error { return nil })
	Str      = strMatcher("string", func(string) bool { return true })
	NonEmpty = strMatcher("non-empty string", func(s string) bool { return s != "" })
	UUID     = strMatcher("uuid v4", reUUID.MatchString)
	ISOms    = strMatcher("ISO-8601 UTC timestamp with ms", reISOms.MatchString)
	JWT      = strMatcher("JWT", reJWT.MatchString)
	Num      = mf("number", func(v any) error {
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("want number, got %s %v", typeName(v), v)
		}
		return nil
	})
	Int = mf("integer", func(v any) error {
		f, ok := v.(float64)
		if !ok || f != math.Trunc(f) {
			return fmt.Errorf("want integer, got %s %v", typeName(v), v)
		}
		return nil
	})
	Bool = mf("boolean", func(v any) error {
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("want boolean, got %s %v", typeName(v), v)
		}
		return nil
	})
	AnyObj = mf("object", func(v any) error {
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("want object, got %s", typeName(v))
		}
		return nil
	})
	AnyArr = mf("array", func(v any) error {
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("want array, got %s", typeName(v))
		}
		return nil
	})
)

func Hex(n int) Matcher {
	return strMatcher(fmt.Sprintf("%d lowercase hex chars", n), func(s string) bool {
		return len(s) == n && reHex.MatchString(s)
	})
}

func Prefix(p string) Matcher {
	return strMatcher(fmt.Sprintf("string starting with %q", p), func(s string) bool { return strings.HasPrefix(s, p) })
}

func Contains(sub string) Matcher {
	return strMatcher(fmt.Sprintf("string containing %q", sub), func(s string) bool { return strings.Contains(s, sub) })
}

func Re(pattern string) Matcher {
	re := regexp.MustCompile(pattern)
	return strMatcher("string matching "+pattern, re.MatchString)
}

// Either matches when any alternative matches (expected values or matchers).
func Either(alts ...any) Matcher {
	return mf(fmt.Sprintf("one of %v", alts), func(v any) error {
		var errs []string
		for _, a := range alts {
			if e := compare("", a, v); len(e) == 0 {
				return nil
			} else {
				errs = append(errs, strings.Join(e, "; "))
			}
		}
		return fmt.Errorf("no alternative matched: %s", strings.Join(errs, " | "))
	})
}

func OrNull(m any) Matcher { return Either(nil, m) }

// ArrayOf matches an array (length >= min) whose every element matches elem.
func ArrayOf(elem any, min int) Matcher {
	return mf("array of matching elements", func(v any) error {
		arr, ok := v.([]any)
		if !ok {
			return fmt.Errorf("want array, got %s", typeName(v))
		}
		if len(arr) < min {
			return fmt.Errorf("want at least %d elements, got %d", min, len(arr))
		}
		for i, el := range arr {
			if e := compare(fmt.Sprintf("[%d]", i), elem, el); len(e) > 0 {
				return fmt.Errorf("%s", strings.Join(e, "; "))
			}
		}
		return nil
	})
}

type optional struct{ inner any }

// Optional marks an object key that may be absent; when present it must match.
func Optional(m any) any { return optional{m} }

func compare(path string, want, got any) []string {
	if path == "" {
		path = "$"
	}
	switch w := want.(type) {
	case optional:
		return compare(path, w.inner, got)
	case Matcher:
		if err := w.Match(got); err != nil {
			return []string{fmt.Sprintf("%s: %v", path, err)}
		}
		return nil
	case Obj:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want object, got %s %s", path, typeName(got), short(got))}
		}
		var errs []string
		for k, wv := range w {
			gv, present := g[k]
			if !present {
				if _, opt := wv.(optional); !opt {
					errs = append(errs, fmt.Sprintf("%s.%s: missing key", path, k))
				}
				continue
			}
			errs = append(errs, compare(path+"."+k, wv, gv)...)
		}
		var extra []string
		for k := range g {
			if _, ok := w[k]; !ok {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range extra {
			errs = append(errs, fmt.Sprintf("%s.%s: unexpected key (value %s)", path, k, short(g[k])))
		}
		return errs
	case map[string]any:
		return compare(path, Obj(w), got)
	case Arr:
		g, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want array, got %s %s", path, typeName(got), short(got))}
		}
		if len(g) != len(w) {
			return []string{fmt.Sprintf("%s: want %d elements, got %d: %s", path, len(w), len(g), short(got))}
		}
		var errs []string
		for i := range w {
			errs = append(errs, compare(fmt.Sprintf("%s[%d]", path, i), w[i], g[i])...)
		}
		return errs
	case []any:
		return compare(path, Arr(w), got)
	case []string:
		a := make(Arr, len(w))
		for i, s := range w {
			a[i] = s
		}
		return compare(path, a, got)
	case nil:
		if got != nil {
			return []string{fmt.Sprintf("%s: want null, got %s %s", path, typeName(got), short(got))}
		}
		return nil
	case string:
		if g, ok := got.(string); !ok || g != w {
			return []string{fmt.Sprintf("%s: want %q, got %s", path, w, short(got))}
		}
		return nil
	case bool:
		if g, ok := got.(bool); !ok || g != w {
			return []string{fmt.Sprintf("%s: want %v, got %s", path, w, short(got))}
		}
		return nil
	case int:
		return compare(path, float64(w), got)
	case int64:
		return compare(path, float64(w), got)
	case float64:
		if g, ok := got.(float64); !ok || g != w {
			return []string{fmt.Sprintf("%s: want %v, got %s", path, w, short(got))}
		}
		return nil
	}
	return []string{fmt.Sprintf("%s: unsupported expectation %T", path, want)}
}

func short(v any) string {
	b, _ := json.Marshal(v)
	return truncate(string(b), 300)
}

// ---------------------------------------------------------------------------
// Response assertions

// JSON decodes the body; it fails the test unless the response is JSON.
func (r *Resp) JSON(t *testing.T) any {
	t.Helper()
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s: want application/json, got Content-Type %q", r, ct)
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(r.Body))
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: invalid JSON: %v", r, err)
	}
	return v
}

func (r *Resp) Obj(t *testing.T) map[string]any {
	t.Helper()
	m, ok := r.JSON(t).(map[string]any)
	if !ok {
		t.Fatalf("%s: want JSON object", r)
	}
	return m
}

func (r *Resp) Arr(t *testing.T) []any {
	t.Helper()
	a, ok := r.JSON(t).([]any)
	if !ok {
		t.Fatalf("%s: want JSON array", r)
	}
	return a
}

// Expect asserts the exact status code and the structural JSON body.
func (r *Resp) Expect(t *testing.T, status int, want any) any {
	t.Helper()
	if r.Status != status {
		t.Fatalf("want status %d\n  got: %s", status, r)
	}
	got := r.JSON(t)
	if errs := compare("", want, got); len(errs) > 0 {
		t.Fatalf("%s %s: body mismatch:\n  %s\n  body: %s", r.Method, r.Path, strings.Join(errs, "\n  "), truncate(string(r.Body), 2000))
	}
	return got
}

// ExpectError asserts status and an exact {"error": msg} body.
func (r *Resp) ExpectError(t *testing.T, status int, msg any) {
	t.Helper()
	r.Expect(t, status, Obj{"error": msg})
}

// ExpectStatus asserts only the status code.
func (r *Resp) ExpectStatus(t *testing.T, status int) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("want status %d\n  got: %s", status, r)
	}
}

// match asserts a decoded value against an expectation.
func match(t *testing.T, label string, want, got any) {
	t.Helper()
	if errs := compare("", want, got); len(errs) > 0 {
		t.Fatalf("%s mismatch:\n  %s\n  value: %s", label, strings.Join(errs, "\n  "), short(got))
	}
}

// findBy returns the first array element whose key equals value.
func findBy(arr []any, key string, value any) map[string]any {
	for _, el := range arr {
		if m, ok := el.(map[string]any); ok && m[key] == value {
			return m
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// HS256 JWT minting (tests that need tokens the server did not issue)

func signJWT(secret string, claims map[string]any) string {
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	p, _ := json.Marshal(claims)
	unsigned := enc.EncodeToString(h) + "." + enc.EncodeToString(p)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return unsigned + "." + enc.EncodeToString(mac.Sum(nil))
}

func hmacHex(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
