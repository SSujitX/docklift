package deploy

import (
	"regexp"

	"docklift/internal/store"
)

// SharedEnvService is the service_name of env variables shared by every
// service of a project.
const SharedEnvService = ""

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// IsValidEnvKey reports whether key is a valid env variable name of at most
// 128 characters (isValidEnvKey).
func IsValidEnvKey(key string) bool {
	return envKeyRe.MatchString(key) && len(key) <= 128
}

// NormalizeEnvValue is normalizeEnvValue for a JSON body value: null or
// absent becomes "", other values are converted with JavaScript String()
// semantics, trimmed, and one pair of matching surrounding quotes ("..." or
// '...') is removed.
func NormalizeEnvValue(value any, present bool) string {
	if !present || value == nil {
		return ""
	}
	v := jsTrim(jsString(value))
	if n := len(v); n >= 2 {
		if (v[0] == '"' && v[n-1] == '"') || (v[0] == '\'' && v[n-1] == '\'') {
			v = v[1 : n-1]
		}
	}
	return v
}

// NormalizeEnvServiceName is normalizeEnvServiceName: null or absent is the
// shared scope "", other values are String()-converted and trimmed.
func NormalizeEnvServiceName(name any, present bool) string {
	if !present || name == nil {
		return SharedEnvService
	}
	return jsTrim(jsString(name))
}

// EnvVar is an env variable as the build and compose code consume it.
type EnvVar struct {
	Key   string
	Value string
	// ServiceName is "" for shared variables, else the Docker service name.
	ServiceName string
	IsBuildArg  bool
	IsRuntime   bool
	IsSecret    bool
}

// EnvVarsFromStore converts rows with the defaults the TS deploy route
// applied: is_build_arg null = false, is_runtime null = true, is_secret
// null = false.
func EnvVarsFromStore(rows []*store.EnvVariable) []EnvVar {
	out := make([]EnvVar, 0, len(rows))
	for _, r := range rows {
		out = append(out, EnvVar{
			Key:         r.Key,
			Value:       r.Value,
			ServiceName: r.ServiceName,
			IsBuildArg:  r.IsBuildArg != nil && *r.IsBuildArg,
			IsRuntime:   r.IsRuntime == nil || *r.IsRuntime,
			IsSecret:    r.IsSecret != nil && *r.IsSecret,
		})
	}
	return out
}

// EnvForService merges shared variables with those scoped to serviceName;
// a service-scoped key overrides the shared key of the same name. The
// result keeps first-insertion order of keys, like the TS Map.
func EnvForService(all []EnvVar, serviceName string) []EnvVar {
	index := map[string]int{}
	var out []EnvVar
	put := func(v EnvVar) {
		if i, ok := index[v.Key]; ok {
			out[i] = v
			return
		}
		index[v.Key] = len(out)
		out = append(out, v)
	}
	for _, v := range all {
		if v.ServiceName == SharedEnvService {
			put(v)
		}
	}
	for _, v := range all {
		if v.ServiceName == serviceName {
			put(v)
		}
	}
	return out
}
