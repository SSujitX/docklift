package certs

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func fixturePEM(t testing.TB, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// coveringFixture reproduces certs.covering.test.ts: live/a has SANs a+b,
// live/b is an orphan with b only.
func coveringFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	writeLive(t, f.root, "a.example.com", fixturePEM(t, "covering-a-b.pem"))
	writeLive(t, f.root, "b.example.com", fixturePEM(t, "orphan-b.pem"))
	return f
}

func TestCoveringPrefersMultiSANAfterReorder(t *testing.T) {
	f := coveringFixture(t)
	if got := f.svc.FindCoveringCertName([]string{"b.example.com", "a.example.com"}); got != "a.example.com" {
		t.Fatalf("orphan live/b must not win when it does not cover a: %q", got)
	}
	if got := f.svc.FindCoveringCertName([]string{"a.example.com", "b.example.com"}); got != "a.example.com" {
		t.Fatal(got)
	}
}

func TestResolveCertNamePrefersHealthierMultiSAN(t *testing.T) {
	f := coveringFixture(t)
	if got := f.svc.ResolveCertName("b.example.com"); got != "a.example.com" {
		t.Fatal(got)
	}
	if got := f.svc.FindCoveringCertName([]string{"b.example.com"}); got != "a.example.com" {
		t.Fatal(got)
	}
	if got := f.svc.ResolveCertName("  A.Example.com "); got != "a.example.com" {
		t.Fatal(got)
	}
	if got := f.svc.ResolveCertName("   "); got != "" {
		t.Fatal(got)
	}
	if got := f.svc.ResolveCertName("c.example.com"); got != "" {
		t.Fatal(got)
	}
}

func TestSelectCertLineage(t *testing.T) {
	f := coveringFixture(t)
	l := f.svc.SelectCertLineage([]string{"b.example.com", "a.example.com", "c.example.com"})
	if l != (Lineage{CertName: "a.example.com", Covering: "", NeedsExpand: true}) {
		t.Fatalf("expand the covering lineage, not the orphan: %+v", l)
	}
	l = f.svc.SelectCertLineage([]string{"b.example.com", "a.example.com"})
	if l != (Lineage{CertName: "a.example.com", Covering: "a.example.com", NeedsExpand: false}) {
		t.Fatalf("reuse covering without expand: %+v", l)
	}
	l = f.svc.SelectCertLineage([]string{"new.example.com"})
	if l != (Lineage{CertName: "new.example.com"}) {
		t.Fatalf("fresh lineage: %+v", l)
	}
	if l = f.svc.SelectCertLineage(nil); l.CertName != "unknown" {
		t.Fatalf("empty: %+v", l)
	}
}

func TestListCertDNSNamesOnlyDNS(t *testing.T) {
	f := coveringFixture(t)
	names := f.svc.ListCertDNSNames("a.example.com")
	slices.Sort(names)
	if !slices.Equal(names, []string{"a.example.com", "b.example.com"}) {
		t.Fatal(names)
	}
	writeLive(t, f.root, "mixed.example.com", genCertPEM(t, certSpec{
		cn: "Mixed.Example.com", dns: []string{"X.example.com", "x.example.com", "y.example.com"},
		ips: []string{"203.0.113.1"}, emails: []string{"ops@example.com"}, uris: []string{"https://u.example.com"},
		notAfter: f.now.Add(90 * 24 * time.Hour),
	}))
	if got := f.svc.ListCertDNSNames("mixed.example.com"); !slices.Equal(got, []string{"x.example.com", "y.example.com", "mixed.example.com"}) {
		t.Fatal(got)
	}
	// Unreadable PEM, missing privkey, missing dir, unsafe names.
	writeLive(t, f.root, "garbage.example.com", "not a pem")
	writeLive(t, f.root, "nokey.example.com", fixturePEM(t, "orphan-b.pem"))
	if err := os.Remove(filepath.Join(f.root, "live", "nokey.example.com", "privkey.pem")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"garbage.example.com", "nokey.example.com", "none.example.com", "../live/a.example.com", "a.example.com/../a.example.com", "", ".."} {
		if got := f.svc.ListCertDNSNames(name); len(got) != 0 {
			t.Fatalf("%q: %v", name, got)
		}
	}
	if f.svc.CertificateFilesExist("nokey.example.com") {
		t.Fatal("privkey missing")
	}
}

func TestExpandableTieBreaksTowardRicherSANs(t *testing.T) {
	f := coveringFixture(t)
	if got := f.svc.FindBestExpandableCertName([]string{"b.example.com", "a.example.com"}); got != "a.example.com" {
		t.Fatal(got)
	}
}

func TestExpandableIgnoresForeignLineageAfterMove(t *testing.T) {
	f := coveringFixture(t)
	if got := f.svc.FindBestExpandableCertName([]string{"b.example.com", "c.example.com"}); got != "b.example.com" {
		t.Fatal(got)
	}
	l := f.svc.SelectCertLineage([]string{"b.example.com", "c.example.com"})
	if l.CertName != "b.example.com" || !l.NeedsExpand {
		t.Fatalf("orphan b covers b only — expand to add c: %+v", l)
	}
	if got := f.svc.FindBestExpandableCertName([]string{"c.example.com"}); got != "" {
		t.Fatal(got)
	}
	if got := f.svc.FindBestExpandableCertName(nil); got != "" {
		t.Fatal(got)
	}
}

func TestEqualOverlapPrefersPrimaryCoveringLineage(t *testing.T) {
	f := coveringFixture(t)
	writeLive(t, f.root, "prod.example.com", fixturePEM(t, "prod-only.pem"))
	writeLive(t, f.root, "staging.example.com", fixturePEM(t, "staging-only.pem"))
	if got := f.svc.FindBestExpandableCertName([]string{"prod.example.com", "staging.example.com"}); got != "prod.example.com" {
		t.Fatalf("must not pick newer staging-only cert when primary is prod: %q", got)
	}
	if got := f.svc.SelectCertLineage([]string{"prod.example.com", "staging.example.com"}).CertName; got != "prod.example.com" {
		t.Fatal(got)
	}
}

func TestCoveringPrefersValidOverExpiredSameFolder(t *testing.T) {
	f := newFixture(t)
	writeLive(t, f.root, "app.example.com", genCertPEM(t, certSpec{cn: "app.example.com", dns: []string{"app.example.com"}, notAfter: f.now.Add(-24 * time.Hour)}))
	writeLive(t, f.root, "app.example.com-0001", genCertPEM(t, certSpec{cn: "app.example.com", dns: []string{"app.example.com"}, notAfter: f.now.Add(60 * 24 * time.Hour)}))
	if got := f.svc.FindCoveringCertName([]string{"app.example.com"}); got != "app.example.com-0001" {
		t.Fatalf("expired same-named folder won: %q", got)
	}
}

func TestIsValidHostnameRejectsInjection(t *testing.T) {
	good := []string{"a.test", "app.example.com", "x-1.y-2.example", "A.B", "a1.b2.c3"}
	bad := []string{
		"", "localhost", "a..test", "-a.test", "a-.test", "a.test.", ".a.test", "bad_domain.test",
		"a.test;b", "a.test\n", "a.test\nserver_name evil.com", "a.test b.test", "a.test{", "a.test}",
		"a.test'", `a.test"`, "$host.test", "a.test#", "*.example.com", "a.test/../x", `a\b.test`, "a.test\r", "a.test\x00",
		"exämple.com",
	}
	for _, s := range good {
		if !IsValidHostname(s) {
			t.Errorf("%q rejected", s)
		}
	}
	for _, s := range bad {
		if IsValidHostname(s) {
			t.Errorf("%q accepted", s)
		}
	}
	for _, s := range []string{"a.example.com-0001", "example.com"} {
		if !IsValidCertName(s) {
			t.Errorf("cert name %q rejected", s)
		}
	}
	for _, s := range []string{"", "..", "../x", "a/b", `a\b`, "a b", "a;b", ".hidden", "a..b"} {
		if IsValidCertName(s) {
			t.Errorf("cert name %q accepted", s)
		}
	}
}

func TestCleanHosts(t *testing.T) {
	got := CleanHosts([]string{" B.example.com", "", "b.example.com", "a.example.com", "  "})
	if !slices.Equal(got, []string{"b.example.com", "a.example.com"}) {
		t.Fatal(got)
	}
}

func TestNginxCertPaths(t *testing.T) {
	fc, pk := NginxCertPaths("a.example.com")
	if fc != "/etc/letsencrypt/live/a.example.com/fullchain.pem" || pk != "/etc/letsencrypt/live/a.example.com/privkey.pem" {
		t.Fatal(fc, pk)
	}
}
