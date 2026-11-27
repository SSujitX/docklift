package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

// LoadDotenv applies dir/.env (never overriding variables already set) and
// then dir/.env.local (overriding), like dotenv called twice, with
// override=false and override=true. Missing files are
// ignored exactly like dotenv does. It returns the files that were applied.
func LoadDotenv(dir string) ([]string, error) {
	var applied []string
	for _, f := range []struct {
		name     string
		override bool
	}{
		{".env", false},
		{".env.local", true},
	} {
		path := dir + string(os.PathSeparator) + f.name
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return applied, fmt.Errorf("read %s: %w", path, err)
		}
		for _, kv := range ParseDotenv(string(data)) {
			if _, exists := os.LookupEnv(kv.Key); exists && !f.override {
				continue
			}
			if err := os.Setenv(kv.Key, kv.Value); err != nil {
				return applied, fmt.Errorf("set %s from %s: %w", kv.Key, path, err)
			}
		}
		applied = append(applied, path)
	}
	return applied, nil
}

// KeyValue is one assignment parsed from a dotenv file.
type KeyValue struct {
	Key   string
	Value string
}

// dotenvLine is the line regexp used by the dotenv package (lib/main.js LINE).
var dotenvLine = regexp.MustCompile(`(?m)(?:^|^)\s*(?:export\s+)?([\w.-]+)(?:\s*=\s*?|:\s+?)(\s*'(?:\\'|[^'])*'|\s*"(?:\\"|[^"])*"|\s*` + "`" + `(?:\\` + "`" + `|[^` + "`" + `])*` + "`" + `|[^#\r\n]+)?\s*(?:#.*)?(?:$|$)`)

// ParseDotenv parses dotenv file contents with the same rules as the dotenv
// npm package: optional `export`, `KEY=value` or `KEY: value`, single, double
// or backtick quotes (multi-line allowed), `\n`/`\r` expansion inside double
// quotes, and `#` comments after unquoted values. Later keys win.
func ParseDotenv(src string) []KeyValue {
	lines := strings.ReplaceAll(src, "\r\n", "\n")
	lines = strings.ReplaceAll(lines, "\r", "\n")
	index := map[string]int{}
	var out []KeyValue
	for _, m := range dotenvLine.FindAllStringSubmatch(lines, -1) {
		key := m[1]
		value := strings.TrimSpace(m[2])
		quote := byte(0)
		if len(value) > 0 {
			quote = value[0]
		}
		if len(value) >= 2 && (quote == '\'' || quote == '"' || quote == '`') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
		}
		if quote == '"' {
			value = strings.ReplaceAll(value, `\n`, "\n")
			value = strings.ReplaceAll(value, `\r`, "\r")
		}
		if i, ok := index[key]; ok {
			out[i].Value = value
			continue
		}
		index[key] = len(out)
		out = append(out, KeyValue{Key: key, Value: value})
	}
	return out
}
