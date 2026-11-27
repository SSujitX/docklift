package backup

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"docklift/internal/js"
)

// validFilename is isValidBackupFilename: letters, digits, dots, hyphens and
// underscores, ending in .zip.
var validFilename = regexp.MustCompile(`^[a-zA-Z0-9._-]+\.zip$`)

// ValidFilename reports whether name may address a file in the backup or
// uploads directory: it cannot contain a separator or `..`, so it never
// leaves that directory by name (links are checked separately by real path).
func ValidFilename(name string) bool {
	return validFilename.MatchString(name) && !strings.Contains(name, "..")
}

// DefaultLabel names backups created without a name.
const DefaultLabel = "docklift"

// maxLabelUnits is the UTF-16 length a custom backup name is cut to.
const maxLabelUnits = 50

// SanitizeLabel is name.replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 50): every
// UTF-16 code unit outside [A-Za-z0-9_-] becomes '-' (a character outside
// the BMP becomes two), then the result is cut to 50 units.
func SanitizeLabel(name string) string {
	units := utf16.Encode([]rune(name))
	if len(units) > maxLabelUnits {
		units = units[:maxLabelUnits]
	}
	var b strings.Builder
	for _, u := range units {
		switch {
		case u >= 'a' && u <= 'z', u >= 'A' && u <= 'Z', u >= '0' && u <= '9', u == '_', u == '-':
			b.WriteByte(byte(u))
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// Timestamp is the backup file timestamp: toISOString() with ':' and '.'
// replaced by '-', 'T' by '_', cut to 19 characters (2026-01-02_03-04-05).
func Timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02_15-04-05")
}

// BackupFilename is `${label}-backup-${timestamp}.zip`.
func BackupFilename(label string, now time.Time) string {
	return label + "-backup-" + Timestamp(now) + ".zip"
}

// RestoredFilename marks an applied upload: file.zip -> file.restored-<date>.zip
// (the first ".zip" is removed, as String.replace does).
func RestoredFilename(filename string, now time.Time) string {
	return strings.Replace(filename, ".zip", "", 1) + ".restored-" + now.UTC().Format("2006-01-02") + ".zip"
}

// ISO formats t as Date.toISOString() does.
func ISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

var byteUnits = []string{"B", "KB", "MB", "GB", "TB"}

// FormatBytes is the human-readable size of a byte count:
// parseFloat((bytes / 1024^i).toFixed(2)) + ' ' + unit.
func FormatBytes(n int64) string {
	if n == 0 {
		return "0 B"
	}
	v := float64(n)
	i := int(math.Floor(math.Log(v) / math.Log(1024)))
	i = max(0, min(i, len(byteUnits)-1))
	fixed := js.ToFixed(v/math.Pow(1024, float64(i)), 2)
	f, err := strconv.ParseFloat(fixed, 64)
	if err != nil {
		return fixed + " " + byteUnits[i]
	}
	return strconv.FormatFloat(f, 'f', -1, 64) + " " + byteUnits[i]
}

// rule is the 50-character separator line of the progress logs.
var rule = strings.Repeat("=", 50)
