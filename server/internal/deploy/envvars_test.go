package deploy

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"docklift/internal/store"
)

func TestEnvForService(t *testing.T) {
	rows := []EnvVar{
		{Key: "SHARED", Value: "a", ServiceName: SharedEnvService},
		{Key: "API_ONLY", Value: "b", ServiceName: "api"},
		{Key: "SHARED", Value: "override", ServiceName: "api"},
		{Key: "WEB_ONLY", Value: "c", ServiceName: "web"},
	}
	api := EnvForService(rows, "api")
	got := map[string]string{}
	var order []string
	for _, v := range api {
		got[v.Key] = v.Value
		order = append(order, v.Key)
	}
	if got["SHARED"] != "override" || got["API_ONLY"] != "b" || got["WEB_ONLY"] != "" {
		t.Fatalf("api env %v", got)
	}
	// The overridden key keeps its first position, like a JS Map.
	if !reflect.DeepEqual(order, []string{"SHARED", "API_ONLY"}) {
		t.Fatalf("order %v", order)
	}
	other := EnvForService(rows, "worker")
	if len(other) != 1 || other[0].Key != "SHARED" || other[0].Value != "a" {
		t.Fatalf("worker env %+v", other)
	}
	if shared := EnvForService(rows, SharedEnvService); len(shared) != 1 {
		t.Fatalf("shared scope %+v", shared)
	}
}

func TestIsValidEnvKey(t *testing.T) {
	for key, want := range map[string]bool{
		"A": true, "_x": true, "DATABASE_URL": true, "a1": true,
		"": false, "1A": false, "A-B": false, "A B": false, "É": false,
		strings.Repeat("K", 128): true, strings.Repeat("K", 129): false,
	} {
		if got := IsValidEnvKey(key); got != want {
			t.Errorf("IsValidEnvKey(%q) = %v", key, got)
		}
	}
}

func TestNormalizeEnvValue(t *testing.T) {
	cases := []struct {
		v       any
		present bool
		want    string
	}{
		{nil, false, ""},
		{nil, true, ""},
		{"  plain  ", true, "plain"},
		{`"quoted"`, true, "quoted"},
		{`'single'`, true, "single"},
		{`"mismatch'`, true, `"mismatch'`},
		{`""`, true, ""},
		{`"`, true, `"`},
		{` "a b" `, true, "a b"},
		{`""x""`, true, `"x"`},
		{string(rune(0xfeff)) + " bom" + string(rune(0xa0)), true, "bom"},
		{float64(42), true, "42"},
		{json.Number("8080"), true, "8080"},
		{json.Number("1.50"), true, "1.5"},
		{json.Number("1e21"), true, "1e+21"},
		{1.5e21, true, "1.5e+21"},
		{1e-7, true, "1e-7"},
		{true, true, "true"},
		{[]any{"a", float64(1), nil}, true, "a,1,"},
		{map[string]any{}, true, "[object Object]"},
	}
	for _, c := range cases {
		if got := NormalizeEnvValue(c.v, c.present); got != c.want {
			t.Errorf("NormalizeEnvValue(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
	if NormalizeEnvServiceName(json.Number("7"), true) != "7" {
		t.Error("NormalizeEnvServiceName must print numbers as JavaScript does")
	}
	if NormalizeEnvServiceName(nil, true) != "" || NormalizeEnvServiceName(" api ", true) != "api" || NormalizeEnvServiceName(nil, false) != "" {
		t.Error("NormalizeEnvServiceName")
	}
}

func TestEnvVarsFromStoreDefaults(t *testing.T) {
	tr, fa := true, false
	got := EnvVarsFromStore([]*store.EnvVariable{
		{Key: "A", Value: "1"},
		{Key: "B", Value: "2", ServiceName: "api", IsBuildArg: &tr, IsRuntime: &fa, IsSecret: &tr},
	})
	want := []EnvVar{
		{Key: "A", Value: "1", IsRuntime: true},
		{Key: "B", Value: "2", ServiceName: "api", IsBuildArg: true, IsSecret: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
