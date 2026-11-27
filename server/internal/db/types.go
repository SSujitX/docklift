package db

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"uuid"
)

// Time is a Prisma DateTime column value.
//
// On disk, Prisma 6 writes DateTime as an INTEGER of Unix milliseconds (UTC),
// while SQL defaults (`DEFAULT CURRENT_TIMESTAMP`) and older installs hold TEXT
// such as "2026-01-02 03:04:05" or "2026-01-02T03:04:05.678Z". Scan accepts all
// of these; Value always writes Prisma's integer encoding. Precision is
// milliseconds, like a JavaScript Date. JSON output is the ISO-8601 form that
// Date.prototype.toJSON produces ("2026-01-02T03:04:05.678Z").
type Time struct {
	t time.Time
}

// NewTime truncates t to milliseconds, as storing it in a JavaScript Date would.
func NewTime(t time.Time) Time {
	return Time{t: time.UnixMilli(t.UnixMilli()).UTC()}
}

// Now is the current time at millisecond precision.
func Now() Time { return NewTime(time.Now()) }

// FromUnixMilli builds a Time from Unix milliseconds.
func FromUnixMilli(ms int64) Time { return Time{t: time.UnixMilli(ms).UTC()} }

// Time returns the value as a UTC time.Time.
func (t Time) Time() time.Time { return t.t }

// UnixMilli is the value as Unix milliseconds (Date.prototype.getTime).
func (t Time) UnixMilli() int64 { return t.t.UnixMilli() }

// ISO formats the value like Date.prototype.toISOString.
func (t Time) ISO() string { return FormatISO(t.t) }

// FormatISO formats t like Date.prototype.toISOString: UTC with milliseconds.
func FormatISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// Value implements driver.Valuer with Prisma's integer-milliseconds encoding.
func (t Time) Value() (driver.Value, error) { return t.t.UnixMilli(), nil }

// MarshalJSON implements json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.ISO() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler for ISO-8601 strings.
func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("db.Time: %w", err)
	}
	parsed, err := ParseTimeText(s)
	if err != nil {
		return err
	}
	*t = NewTime(parsed)
	return nil
}

// Scan implements sql.Scanner. NULL is rejected; scan nullable columns into
// *Time (a **Time destination), which database/sql sets to nil for NULL.
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*t = FromUnixMilli(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("db.Time: invalid numeric value %v", v)
		}
		*t = FromUnixMilli(int64(math.Round(v)))
	case time.Time:
		*t = NewTime(v)
	case string:
		parsed, err := ParseTimeText(v)
		if err != nil {
			return err
		}
		*t = NewTime(parsed)
	case []byte:
		parsed, err := ParseTimeText(string(v))
		if err != nil {
			return err
		}
		*t = NewTime(parsed)
	case nil:
		return fmt.Errorf("db.Time: NULL value (scan into *db.Time for nullable columns)")
	default:
		return fmt.Errorf("db.Time: unsupported type %T", src)
	}
	return nil
}

// timeTextLayouts are the TEXT encodings found in Docklift databases: SQLite's
// CURRENT_TIMESTAMP and datetime() forms (UTC, optional fraction) and ISO-8601
// with Z or an explicit offset. Layouts without a zone are read as UTC.
var timeTextLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04Z07:00",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// ParseTimeText parses a TEXT DateTime value.
func ParseTimeText(s string) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	for _, layout := range timeTextLayouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("db.Time: unrecognised datetime text %q", s)
}

// JSON is a Prisma Json column value, stored as TEXT. It holds the raw JSON
// document; a nil JSON means SQL NULL when used as *JSON.
type JSON []byte

// MarshalJSON implements json.Marshaler by emitting the stored document.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON implements json.Unmarshaler by keeping the raw document.
func (j *JSON) UnmarshalJSON(b []byte) error {
	*j = append((*j)[:0], b...)
	return nil
}

// Value implements driver.Valuer, writing the document as TEXT like Prisma.
func (j JSON) Value() (driver.Value, error) {
	if !json.Valid(j) {
		return nil, fmt.Errorf("db.JSON: invalid JSON document")
	}
	return string(j), nil
}

// Scan implements sql.Scanner.
func (j *JSON) Scan(src any) error {
	var b []byte
	switch v := src.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = bytes.Clone(v)
	default:
		return fmt.Errorf("db.JSON: unsupported type %T", src)
	}
	if !json.Valid(b) {
		return fmt.Errorf("db.JSON: column holds invalid JSON")
	}
	*j = b
	return nil
}

// NewID returns a random RFC 9562 version 4 UUID in canonical form, the
// format Prisma's @default(uuid()) generates.
func NewID() string { return uuid.NewV4().String() }
