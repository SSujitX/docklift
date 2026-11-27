package execx

import (
	"os"
	"strings"
)

// The backend's own secrets, which no child process may inherit: docker,
// compose, buildx, railpack, git, the terminal shell and /execute commands
// all run with the server's environment minus these. This is the one place
// that decides it (Run applies it to every program; the terminal shell uses
// Environ).
var (
	// sensitiveEnvKeys are exact variable names.
	sensitiveEnvKeys = []string{"JWT_SECRET", "INTERNAL_API_SECRET"}
	// sensitiveEnvPrefixes cover a family of variables (GitHub App id,
	// client id/secret, private key path, and any later DOCKLIFT_GITHUB_*).
	sensitiveEnvPrefixes = []string{"DOCKLIFT_GITHUB_"}
)

// IsSensitiveEnv reports whether the variable name key holds a backend
// secret that children must not inherit. Names compare case-insensitively
// (Windows environment names are case-insensitive).
func IsSensitiveEnv(key string) bool {
	k := strings.ToUpper(key)
	for _, s := range sensitiveEnvKeys {
		if k == s {
			return true
		}
	}
	for _, p := range sensitiveEnvPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// FilterEnv returns env ("KEY=VALUE" entries) without the sensitive
// variables.
func FilterEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if key == "" && strings.HasPrefix(kv, "=") {
			// Windows per-drive entries such as "=C:=C:\dir".
			key, _, _ = strings.Cut(kv[1:], "=")
		}
		if !IsSensitiveEnv(key) {
			out = append(out, kv)
		}
	}
	return out
}

// Environ is os.Environ() without the sensitive variables: the base
// environment of every child process.
func Environ() []string { return FilterEnv(os.Environ()) }
