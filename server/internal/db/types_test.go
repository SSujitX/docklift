package db

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimeScan(t *testing.T) {
	want := time.Date(2026, 1, 2, 3, 4, 5, 678e6, time.UTC)
	wantSec := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		src  any
		want time.Time
	}{
		{"prisma integer ms", int64(1767323045678), want},
		{"real ms", float64(1767323045678), want},
		{"driver time", time.Date(2026, 1, 2, 3, 4, 5, 678_900_000, time.UTC), want},
		{"current_timestamp text", "2026-01-02 03:04:05", wantSec},
		{"text with fraction", "2026-01-02 03:04:05.678", want},
		{"iso Z", "2026-01-02T03:04:05.678Z", want},
		{"iso offset", "2026-01-02T05:04:05.678+02:00", want},
		{"iso no zone", "2026-01-02T03:04:05.678", want},
		{"bytes", []byte("2026-01-02 03:04:05"), wantSec},
		{"date only", "2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		var got Time
		if err := got.Scan(c.src); err != nil {
			t.Errorf("%s: Scan(%v) error %v", c.name, c.src, err)
			continue
		}
		if !got.Time().Equal(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got.Time(), c.want)
		}
	}
	for _, bad := range []any{nil, "yesterday", true} {
		var got Time
		if err := got.Scan(bad); err == nil {
			t.Errorf("Scan(%#v) must fail", bad)
		}
	}
}

func TestTimeValueAndJSON(t *testing.T) {
	tm := NewTime(time.Date(2026, 1, 2, 3, 4, 5, 678_999_999, time.FixedZone("x", 3600)))
	v, err := tm.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(1767319445678) {
		t.Errorf("Value = %#v, want Prisma integer milliseconds", v)
	}
	b, err := json.Marshal(struct {
		At  Time  `json:"at"`
		Nil *Time `json:"nil"`
	}{At: tm})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"at":"2026-01-02T02:04:05.678Z","nil":null}` {
		t.Errorf("JSON = %s", b)
	}
	var back Time
	if err := json.Unmarshal([]byte(`"2026-01-02T02:04:05.678Z"`), &back); err != nil || back != tm {
		t.Errorf("round trip = %v, %v", back, err)
	}
	if FromUnixMilli(0).ISO() != "1970-01-01T00:00:00.000Z" {
		t.Error("epoch formatting")
	}
}

func TestJSONColumn(t *testing.T) {
	var j JSON
	if err := j.Scan(`{"web":"img:1"}`); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(struct {
		Tags *JSON `json:"tags"`
		None *JSON `json:"none"`
	}{Tags: &j})
	if string(b) != `{"tags":{"web":"img:1"},"none":null}` {
		t.Errorf("JSON = %s", b)
	}
	if err := j.Scan("{not json"); err == nil {
		t.Error("invalid JSON must be rejected on scan")
	}
	if _, err := JSON("{").Value(); err == nil {
		t.Error("invalid JSON must be rejected on write")
	}
	v, err := MarshalJSONValue(map[string]string{"a": "b"})
	if err != nil || string(v) != `{"a":"b"}` {
		t.Errorf("MarshalJSONValue = %s, %v", v, err)
	}
}

func TestNewID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewID()
		if len(id) != 36 || id[14] != '4' || seen[id] {
			t.Fatalf("bad or duplicate id %q", id)
		}
		seen[id] = true
	}
}
