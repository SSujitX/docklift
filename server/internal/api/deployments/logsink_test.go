package deployments

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRedactorAcrossWrites(t *testing.T) {
	cases := []struct {
		name    string
		secrets []string
		chunks  []string
		want    string
	}{
		{"whole", []string{"hunter22"}, []string{"pw=hunter22 ok"}, "pw=*** ok"},
		{"split", []string{"hunter22"}, []string{"pw=hun", "ter", "22 ok"}, "pw=*** ok"},
		{"split at end", []string{"hunter22"}, []string{"pw=hunte", "r22"}, "pw=***"},
		{"prefix only", []string{"hunter22"}, []string{"hunt", "ing"}, "hunting"},
		{"longest first", []string{"abcd", "abcdefgh"}, []string{"x abcdefgh abcd"}, "x *** ***"},
		{"too short ignored", []string{"abc"}, []string{"abc"}, "abc"},
		{"none", nil, []string{"plain ", "text"}, "plain text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r redactor
			r.add(tc.secrets...)
			var out strings.Builder
			for _, c := range tc.chunks {
				out.Write(r.write([]byte(c)))
			}
			out.Write(r.flush())
			if out.String() != tc.want {
				t.Fatalf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

func TestDeployLogStreams(t *testing.T) {
	client := newClientStream()
	l := newDeployLog(client)
	l.redact("topsecret")
	_, _ = l.Write([]byte("build topsec"))
	_, _ = l.Write([]byte("ret done\n"))
	l.line("line\n")
	l.record("db only\n")
	l.toClient("client only topsecret\n")
	if got := l.final(); got != "build *** done\nline\ndb only\n" {
		t.Fatalf("log = %q", got)
	}
	if got := string(client.take()); got != "build *** done\nline\nclient only ***\n" {
		t.Fatalf("client = %q", got)
	}
}

func TestClientStreamNeverBlocks(t *testing.T) {
	c := newClientStream()
	chunk := make([]byte, 1<<20)
	for range 20 {
		if n, err := c.Write(chunk); n != len(chunk) || err != nil {
			t.Fatal("write must succeed")
		}
	}
	if got := len(c.take()); got > clientStreamLimit {
		t.Fatalf("buffered %d bytes past the limit", got)
	}
	c.detach()
	_, _ = c.Write([]byte("late"))
	if len(c.take()) != 0 {
		t.Fatal("a detached stream drops output")
	}
}

func TestParseIntParam(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"10", 10, true}, {"  7px", 7, true}, {"-3", -3, true}, {"+4", 4, true},
		{"abc", 0, false}, {"", 0, false}, {"-x", 0, false}, {"1e3", 1, true},
		{"99999999999", math.MaxInt32, true}, {"-99999999999", math.MinInt32, true},
	}
	for _, tc := range cases {
		got, ok := parseIntParam(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseIntParam(%q) = %d, %v", tc.in, got, ok)
		}
	}
}

func TestJSObjectOrder(t *testing.T) {
	var o jsObject[int]
	o.set("b", 1)
	o.set("a", 2)
	o.set("b", 3)
	got, err := json.Marshal(o)
	if err != nil || string(got) != `{"b":3,"a":2}` {
		t.Fatalf("got %s %v", got, err)
	}
}
