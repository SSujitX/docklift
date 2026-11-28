package certs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"docklift/internal/dnscheck"
	"docklift/internal/store"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCertificateStatusJSONShape(t *testing.T) {
	f := newFixture(t)
	st, err := f.svc.GetCertificateStatus(context.Background(), "  Missing.Example.com ")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"status":"missing","domain":"missing.example.com","expiresAt":null,"error":null,"diagnosticCommand":null,"certPath":null}`
	if got := mustJSON(t, st); got != want {
		t.Fatalf("got %s", got)
	}
	rep, err := f.svc.GetCertificateReport(context.Background(), "missing.example.com")
	if err != nil {
		t.Fatal(err)
	}
	wantRep := `{"status":"missing","domain":"missing.example.com","expiresAt":null,"error":null,"diagnosticCommand":null,"certPath":null,"expired":false,"renewalOverdue":false,"daysUntilExpiry":null}`
	if got := mustJSON(t, rep); got != wantRep {
		t.Fatalf("got %s", got)
	}
}

func TestGetCertificateStatusByExpiry(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name     string
		left     time.Duration
		status   SslStatus
		expired  bool
		overdue  bool
		daysLeft int
	}{
		{"active", 60 * day, StatusActive, false, false, 60},
		{"active just outside expiring", 21*day + time.Hour, StatusActive, false, false, 21},
		{"expiring at 21 days", 21*day - time.Second, StatusExpiring, false, false, 20},
		{"expiring at exactly 20 days is not overdue", 20 * day, StatusExpiring, false, false, 20},
		{"renewal overdue under 20 days", 20*day - time.Second, StatusExpiring, false, true, 19},
		{"overdue last day", 2 * time.Hour, StatusExpiring, false, true, 0},
		{"expired", -time.Second, StatusExpired, true, true, -1},
		{"expired two months", -61 * day, StatusExpired, true, true, -61},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			// Certificates carry whole seconds.
			notAfter := f.now.Add(tt.left).Truncate(time.Second)
			if tt.left < 0 && notAfter.After(f.now.Add(tt.left)) {
				notAfter = notAfter.Add(-time.Second)
			}
			writeLive(t, f.root, "app.example.com", genCertPEM(t, certSpec{cn: "app.example.com", dns: []string{"app.example.com"}, notAfter: notAfter}))
			rep, err := f.svc.GetCertificateReport(context.Background(), "app.example.com")
			if err != nil {
				t.Fatal(err)
			}
			if rep.Status != tt.status {
				t.Fatalf("status %q want %q", rep.Status, tt.status)
			}
			wantExp := notAfter.UTC().Format("2006-01-02T15:04:05.000Z")
			if rep.ExpiresAt == nil || *rep.ExpiresAt != wantExp {
				t.Fatalf("expiresAt %v want %s", rep.ExpiresAt, wantExp)
			}
			wantPath := filepath.Join(f.root, "live", "app.example.com", "fullchain.pem")
			if rep.CertPath == nil || *rep.CertPath != wantPath || rep.Error != nil || rep.DiagnosticCommand != nil {
				t.Fatalf("%+v", rep.CertificateStatus)
			}
			if rep.Expired != tt.expired || rep.RenewalOverdue != tt.overdue || rep.DaysUntilExpiry == nil || *rep.DaysUntilExpiry != tt.daysLeft {
				t.Fatalf("health %+v days=%v", rep.Health, *rep.DaysUntilExpiry)
			}
		})
	}
}

func TestGetCertificateStatusFromMeta(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if err := f.svc.setMeta(ctx, "p.example.com", StatusPending, nil); err != nil {
		t.Fatal(err)
	}
	st, _ := f.svc.GetCertificateStatus(ctx, "p.example.com")
	if mustJSON(t, st) != `{"status":"pending","domain":"p.example.com","expiresAt":null,"error":null,"diagnosticCommand":null,"certPath":null}` {
		t.Fatal(mustJSON(t, st))
	}
	if raw := f.store.settings["ssl_meta_p.example.com"]; raw != `{"status":"pending","error":null,"updatedAt":"2026-11-27T12:00:00.000Z"}` {
		t.Fatalf("meta encoding %s", raw)
	}

	msg := "boom"
	if err := f.svc.setMeta(ctx, "f.example.com", StatusFailed, &msg); err != nil {
		t.Fatal(err)
	}
	st, _ = f.svc.GetCertificateStatus(ctx, "f.example.com")
	want := `{"status":"failed","domain":"f.example.com","expiresAt":null,"error":"boom","diagnosticCommand":"sudo docker exec docklift-certbot tail -n 100 /var/log/letsencrypt/letsencrypt.log","certPath":null}`
	if got := mustJSON(t, st); got != want {
		t.Fatal(got)
	}
	rep, _ := f.svc.GetCertificateReport(ctx, "f.example.com")
	if rep.Expired || rep.RenewalOverdue || rep.DaysUntilExpiry != nil {
		t.Fatalf("failed without cert has no verdict: %+v", rep.Health)
	}

	// Active meta without files is missing; corrupt meta is ignored.
	_ = f.svc.setMeta(ctx, "a.example.com", StatusActive, nil)
	f.store.settings["ssl_meta_c.example.com"] = "{nope"
	for _, h := range []string{"a.example.com", "c.example.com"} {
		if st, _ := f.svc.GetCertificateStatus(ctx, h); st.Status != StatusMissing {
			t.Fatalf("%s: %q", h, st.Status)
		}
	}
	if err := f.svc.ClearSSLMeta(ctx, "f.example.com"); err != nil {
		t.Fatal(err)
	}
	if st, _ := f.svc.GetCertificateStatus(ctx, "f.example.com"); st.Status != StatusMissing {
		t.Fatal(st.Status)
	}

	f.store.failGet = errors.New("db down")
	if _, err := f.svc.GetCertificateStatus(ctx, "x.example.com"); err == nil {
		t.Fatal("store error swallowed")
	}
}

func TestGetCertificateStatusUsesCoveringLineageAndUnreadable(t *testing.T) {
	ctx := context.Background()
	f := coveringFixture(t)
	st, _ := f.svc.GetCertificateStatus(ctx, "b.example.com")
	if st.Status != StatusActive || st.Domain != "b.example.com" || *st.CertPath != filepath.Join(f.root, "live", "a.example.com", "fullchain.pem") {
		t.Fatalf("%+v", st)
	}
	writeLive(t, f.root, "bad.example.com", "garbage")
	st, _ = f.svc.GetCertificateStatus(ctx, "bad.example.com")
	if st.Status != StatusFailed || *st.Error != "Certificate file unreadable" || st.CertPath == nil || st.DiagnosticCommand != nil {
		t.Fatalf("%+v", st)
	}
	msg := "previous failure"
	_ = f.svc.setMeta(ctx, "bad.example.com", StatusFailed, &msg)
	if st, _ = f.svc.GetCertificateStatus(ctx, "bad.example.com"); *st.Error != "previous failure" {
		t.Fatal(*st.Error)
	}
}

func TestAcmeEmail(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.svc.GetAcmeEmail(ctx); !errors.Is(err, ErrNoAcmeEmail) {
		t.Fatal(err)
	}
	f.store.user = &store.User{Email: "admin@example.com"}
	if e, _ := f.svc.GetAcmeEmail(ctx); e != "admin@example.com" {
		t.Fatal(e)
	}
	f.svc.cfg.CertbotEmail = "  env@example.com "
	if e, _ := f.svc.GetAcmeEmail(ctx); e != "env@example.com" {
		t.Fatal(e)
	}
	f.store.settings["ssl_acme_email"] = "no-at-sign"
	if e, _ := f.svc.GetAcmeEmail(ctx); e != "env@example.com" {
		t.Fatal(e)
	}
	for _, bad := range []string{"", "nope"} {
		if err := f.svc.SetAcmeEmail(ctx, bad); !errors.Is(err, ErrInvalidEmail) || err.Error() != "Invalid email" {
			t.Fatal(err)
		}
	}
	if err := f.svc.SetAcmeEmail(ctx, "  Ops@Example.COM "); err != nil {
		t.Fatal(err)
	}
	if e, _ := f.svc.GetAcmeEmail(ctx); e != "ops@example.com" {
		t.Fatal(e)
	}
}

func TestSummarizeCertbotError(t *testing.T) {
	tests := []struct{ name, stdout, stderr, want string }{
		{"domain detail block", "", "Saving debug log to /var/log/letsencrypt/letsencrypt.log\nCertbot failed to authenticate some domains (authenticator: webroot).\n  Domain: app.example.com\n  Type:   dns\n  Detail: DNS problem: NXDOMAIN looking up A for app.example.com\n\nHint: check DNS\nSome challenges have failed.\nAsk for help or search for solutions at https://community.letsencrypt.org.",
			"app.example.com: DNS problem: NXDOMAIN looking up A for app.example.com"},
		{"ansi and CRLF", "", "\x1b[31m  Domain: a.test\r\n  Type: unauthorized\r\n  Detail: 1.2.3.4: Invalid response from http://a.test/.well-known/acme-challenge/x: 404\x1b[0m\r\n",
			"a.test: 1.2.3.4: Invalid response from http://a.test/.well-known/acme-challenge/x: 404"},
		{"detail too far away falls back", "", "Domain: a.test\n1\n2\n3\n4\n5\n6\n7\nDetail: connection refused\n", "Detail: connection refused"},
		{"next domain block stops the search", "", "Domain: a.test\nDomain: b.test\nDetail: for b\n", "b.test: for b"},
		{"useful line skips boilerplate", "Requesting a certificate for a.test", "Saving debug log to x\nAn unexpected error occurred:\nToo many certificates already issued: rate limit\n",
			"Too many certificates already issued: rate limit"},
		{"stderr before stdout", "error in stdout", "error in stderr", "error in stderr"},
		{"first non-boilerplate line", "", "Saving debug log to x\nSomething odd happened\n", "Something odd happened"},
		{"only boilerplate", "", "Saving debug log to x\nAsk for help\n", "Certificate issuance failed for an unknown reason"},
		{"empty", "", "", "Certificate issuance failed for an unknown reason"},
		{"truncated", "", "error " + strings.Repeat("x", 800), "error " + strings.Repeat("x", 694)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SummarizeCertbotError(tt.stdout, tt.stderr); got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
	if got := truncate(strings.Repeat("é", 10), 4); got != "éééé" {
		t.Fatal(got)
	}
	if got := truncate("ab"+string(rune(0x1F600))+"c", 3); got != "ab" {
		t.Fatalf("surrogate pair split: %q", got)
	}
}

func TestCertbotArgs(t *testing.T) {
	r := CertbotRequest{Container: "docklift-certbot", Email: "ops@example.com", CertName: "a.example.com", Hosts: []string{"a.example.com", "b.example.com"}}
	want := "exec docklift-certbot certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos --email ops@example.com --cert-name a.example.com -d a.example.com -d b.example.com"
	if got := strings.Join(r.Args(), " "); got != want {
		t.Fatal(got)
	}
	r.Staging, r.Expand, r.ForceRenewal = true, true, true
	if got := strings.Join(r.Args(), " "); !strings.Contains(got, "--cert-name a.example.com --staging --expand --force-renewal -d a.example.com") {
		t.Fatal(got)
	}
}

func TestPlanOrder(t *testing.T) {
	tests := []struct {
		name            string
		existing, order []string
		force           bool
		expand, renew   bool
	}{
		{"new lineage", nil, []string{"a"}, false, false, false},
		{"new lineage forced", nil, []string{"a"}, true, false, true},
		{"same set", []string{"a", "b"}, []string{"b", "a"}, false, false, false},
		{"same set forced", []string{"a", "b"}, []string{"a", "b"}, true, false, true},
		{"pure expand", []string{"a"}, []string{"a", "b"}, false, true, false},
		{"pure expand forced keeps expand", []string{"a"}, []string{"a", "b"}, true, true, true},
		{"shrink", []string{"a", "b"}, []string{"a"}, false, false, true},
		{"replace", []string{"a", "b"}, []string{"a", "c"}, false, false, true},
	}
	for _, tt := range tests {
		e, r := planOrder(tt.existing, tt.order, tt.force)
		if e != tt.expand || r != tt.renew {
			t.Errorf("%s: expand=%t renew=%t", tt.name, e, r)
		}
	}
}

// issueSuccess makes the fake certbot write a lineage covering the -d hosts.
func issueSuccess(t *testing.T, f *fixture, days int) func(args []string) (ExecResult, error) {
	return func(args []string) (ExecResult, error) {
		var hosts []string
		certName := ""
		for i := 0; i < len(args)-1; i++ {
			switch args[i] {
			case "-d":
				hosts = append(hosts, args[i+1])
			case "--cert-name":
				certName = args[i+1]
			}
		}
		writeLive(t, f.root, certName, genCertPEM(t, certSpec{cn: hosts[0], dns: hosts, notAfter: f.now.Add(time.Duration(days) * 24 * time.Hour).Truncate(time.Second)}))
		return ExecResult{Stdout: "Successfully received certificate."}, nil
	}
}

func eventMessages(evs []Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = string(e.Level) + ": " + e.Message
	}
	return out
}

func TestIssueReusesCoveringCertificate(t *testing.T) {
	ctx := context.Background()
	f := coveringFixture(t)
	st, err := f.svc.IssueCertificate(ctx, []string{"b.example.com", "a.example.com"}, IssueOptions{})
	if err != nil || st.Status != StatusActive || st.Domain != "b.example.com" {
		t.Fatal(st, err)
	}
	if len(f.runner.Calls()) != 0 {
		t.Fatal("certbot ran for a covered host set")
	}
	evs := eventMessages(f.svc.GetSSLEvents([]string{"b.example.com"}))
	if !slices.Equal(evs, []string{"info: Existing certificate (a.example.com) already covers these hostnames — reusing it."}) {
		t.Fatal(evs)
	}
	f.svc.ClearSSLEvents([]string{"a.example.com"})
	_, _ = f.svc.IssueCertificate(ctx, []string{"a.example.com"}, IssueOptions{})
	if evs := eventMessages(f.svc.GetSSLEvents([]string{"a.example.com"})); evs[len(evs)-1] != "info: Existing certificate already covers these hostnames — reusing it." {
		t.Fatal(evs)
	}
}

func TestIssueForcedRenewalAndSuccess(t *testing.T) {
	ctx := context.Background()
	f := coveringFixture(t)
	f.store.user = &store.User{Email: "admin@example.com"}
	f.runner.fn = issueSuccess(t, f, 90)
	f.svc.cfg.CertbotStaging = true
	st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com", "b.example.com"}, IssueOptions{Force: true})
	if err != nil || st.Status != StatusActive {
		t.Fatal(st, err)
	}
	calls := f.runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls %v", calls)
	}
	want := "docker exec docklift-certbot certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos --email admin@example.com --cert-name a.example.com --staging --force-renewal -d a.example.com -d b.example.com"
	if got := strings.Join(calls[0], " "); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	var m meta
	_ = json.Unmarshal([]byte(f.store.settings["ssl_meta_a.example.com"]), &m)
	if m.Status != StatusActive || m.Error != nil {
		t.Fatalf("meta %+v", m)
	}
	evs := eventMessages(f.svc.GetSSLEvents([]string{"a.example.com"}))
	if evs[0] != "info: Requesting certificate for a.example.com, b.example.com (staging CA)" ||
		evs[len(evs)-2] != "info: Running certbot HTTP-01 challenge in docklift-certbot…" ||
		!strings.HasPrefix(evs[len(evs)-1], "success: Certificate issued — valid until ") || !strings.HasSuffix(evs[len(evs)-1], " GMT") {
		t.Fatal(evs)
	}
}

func TestIssueExpandKeepsExtrasAndDropsMissing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.store.user = &store.User{Email: "admin@example.com"}
	writeLive(t, f.root, "a.example.com", genCertPEM(t, certSpec{cn: "a.example.com", dns: []string{"a.example.com", "old.example.com", "gone.example.com"}, notAfter: f.now.Add(80 * 24 * time.Hour)}))
	f.dns.status["gone.example.com"] = dnscheck.StatusMissing
	f.dns.status["old.example.com"] = dnscheck.StatusMismatch
	f.runner.fn = issueSuccess(t, f, 90)
	st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com", "new.example.com"}, IssueOptions{})
	if err != nil || st.Status != StatusActive {
		t.Fatal(st, err)
	}
	got := strings.Join(f.runner.Calls()[0], " ")
	// gone dropped → not a superset → force-renewal, no expand; old kept.
	if !strings.Contains(got, "--cert-name a.example.com --force-renewal -d a.example.com -d new.example.com -d old.example.com") || strings.Contains(got, "gone") || strings.Contains(got, "--expand") {
		t.Fatal(got)
	}
	evs := strings.Join(eventMessages(f.svc.GetSSLEvents([]string{"a.example.com"})), "\n")
	for _, want := range []string{"warn: Leaving gone.example.com off this order — DNS record missing (certificate set will shrink).",
		"info: dns ok for a.example.com", "warn: dns mismatch for old.example.com"} {
		if !strings.Contains(evs, want) {
			t.Fatalf("missing %q in\n%s", want, evs)
		}
	}

	// A pure superset uses --expand without --force-renewal.
	f2 := newFixture(t)
	f2.store.user = &store.User{Email: "admin@example.com"}
	writeLive(t, f2.root, "a.example.com", genCertPEM(t, certSpec{cn: "a.example.com", dns: []string{"a.example.com"}, notAfter: f2.now.Add(80 * 24 * time.Hour)}))
	f2.runner.fn = issueSuccess(t, f2, 90)
	if _, err := f2.svc.IssueCertificate(ctx, []string{"a.example.com", "b.example.com"}, IssueOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f2.runner.Calls()[0], " "); !strings.Contains(got, "--cert-name a.example.com --expand -d a.example.com -d b.example.com") || strings.Contains(got, "--force-renewal") {
		t.Fatal(got)
	}
}

func TestIssueFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("no domains", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.svc.IssueCertificate(ctx, []string{" ", ""}, IssueOptions{}); !errors.Is(err, ErrNoDomains) || err.Error() != "No domains provided" {
			t.Fatal(err)
		}
	})
	t.Run("hostile hostname never reaches certbot", func(t *testing.T) {
		f := newFixture(t)
		for _, h := range []string{"a.test --staging", "-d.test", "a.test\n-d evil.test", "../../etc"} {
			if _, err := f.svc.IssueCertificate(ctx, []string{h}, IssueOptions{}); err == nil {
				t.Fatalf("%q accepted", h)
			}
		}
		if len(f.runner.Calls()) != 0 {
			t.Fatal("certbot ran")
		}
	})
	t.Run("no email", func(t *testing.T) {
		f := newFixture(t)
		st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com"}, IssueOptions{})
		if err != nil || st.Status != StatusMissing {
			t.Fatal(st, err)
		}
		got, _ := f.svc.GetCertificateStatus(ctx, "a.example.com")
		if got.Status != StatusFailed || *got.Error != ErrNoAcmeEmail.Error() {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("dns missing skips certbot", func(t *testing.T) {
		f := newFixture(t)
		f.store.user = &store.User{Email: "admin@example.com"}
		f.dns.status["a.example.com"] = dnscheck.StatusMissing
		st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com", "b.example.com"}, IssueOptions{})
		if err != nil || st.Status != StatusMissing || len(f.runner.Calls()) != 0 {
			t.Fatal(st, err, f.runner.Calls())
		}
		got, _ := f.svc.GetCertificateStatus(ctx, "a.example.com")
		if *got.Error != "DNS record missing for a.example.com — create an A record pointing at this server, then retry SSL." {
			t.Fatal(*got.Error)
		}
		evs := eventMessages(f.svc.GetSSLEvents([]string{"b.example.com"}))
		if evs[len(evs)-1] != "error: Skipped Let’s Encrypt: DNS is not ready yet." {
			t.Fatal(evs)
		}
	})
	t.Run("certbot error is summarised", func(t *testing.T) {
		f := newFixture(t)
		f.store.user = &store.User{Email: "admin@example.com"}
		f.runner.fn = func([]string) (ExecResult, error) {
			return ExecResult{ExitCode: 1, Stderr: "Domain: a.example.com\nDetail: Timeout during connect (likely firewall problem)\n"}, nil
		}
		st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com"}, IssueOptions{})
		if err != nil || st.Status != StatusMissing {
			t.Fatal(st, err)
		}
		got, _ := f.svc.GetCertificateStatus(ctx, "a.example.com")
		if got.Status != StatusFailed || *got.Error != "a.example.com: Timeout during connect (likely firewall problem)" || got.DiagnosticCommand == nil {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("docker unavailable", func(t *testing.T) {
		f := newFixture(t)
		f.store.user = &store.User{Email: "admin@example.com"}
		f.runner.fn = func([]string) (ExecResult, error) {
			return ExecResult{ExitCode: -1}, errors.New(`exec: "docker": executable file not found in $PATH`)
		}
		_, _ = f.svc.IssueCertificate(ctx, []string{"a.example.com"}, IssueOptions{})
		got, _ := f.svc.GetCertificateStatus(ctx, "a.example.com")
		if *got.Error != `exec: "docker": executable file not found in $PATH` {
			t.Fatal(*got.Error)
		}
	})
	t.Run("exit 0 but lineage does not cover", func(t *testing.T) {
		f := newFixture(t)
		f.store.user = &store.User{Email: "admin@example.com"}
		writeLive(t, f.root, "a.example.com", genCertPEM(t, certSpec{cn: "a.example.com", dns: []string{"a.example.com"}, notAfter: f.now.Add(80 * 24 * time.Hour)}))
		st, err := f.svc.IssueCertificate(ctx, []string{"a.example.com", "b.example.com"}, IssueOptions{})
		if err != nil || st.Status != StatusActive {
			t.Fatal(st, err)
		}
		raw := f.store.settings["ssl_meta_a.example.com"]
		if !strings.Contains(raw, `"status":"failed"`) || !strings.Contains(raw, "Certificate at live/a.example.com does not cover a.example.com, b.example.com") {
			t.Fatal(raw)
		}
	})
}

func TestEventLog(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewEventLog()
	l.now = func() time.Time { now = now.Add(time.Millisecond); return now }
	l.Append([]string{" A.example.com ", "b.example.com", ""}, LevelInfo, "shared")
	l.Append([]string{"b.example.com"}, LevelWarn, "b only")
	l.Append([]string{"a.example.com"}, LevelError, "a only")
	got := eventMessages(l.Get([]string{"b.example.com", "A.EXAMPLE.COM"}))
	if !slices.Equal(got, []string{"info: shared", "warn: b only", "error: a only"}) {
		t.Fatal(got)
	}
	if evs := l.Get([]string{"none.example.com"}); evs == nil || len(evs) != 0 || mustJSON(t, evs) != "[]" {
		t.Fatal(evs)
	}
	ev := l.Get([]string{"a.example.com"})[0]
	if mustJSON(t, ev) != `{"at":"2026-01-01T00:00:00.001Z","level":"info","message":"shared"}` {
		t.Fatal(mustJSON(t, ev))
	}
	for i := range 100 {
		l.Append([]string{"a.example.com"}, LevelInfo, fmt.Sprint("m", i))
	}
	evs := l.Get([]string{"a.example.com", "b.example.com"})
	if len(evs) != MaxEventsPerDomain || evs[len(evs)-1].Message != "m99" || evs[0].Message != "m60" {
		t.Fatalf("%d %v", len(evs), evs[0])
	}
	if len(l.byHost["a.example.com"]) != MaxEventsPerDomain {
		t.Fatal("per-host log not bounded")
	}
	l.Clear([]string{"A.example.com"})
	if len(l.Get([]string{"a.example.com"})) != 0 || len(l.Get([]string{"b.example.com"})) != 2 {
		t.Fatal("clear")
	}
}

func TestEventLogConcurrent(t *testing.T) {
	l := NewEventLog()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				l.Append([]string{"a.example.com", fmt.Sprint("h", g, ".example.com")}, LevelInfo, fmt.Sprint(g, "-", i))
				_ = l.Get([]string{"a.example.com"})
				if i%50 == 0 {
					l.Clear([]string{fmt.Sprint("h", g, ".example.com")})
				}
			}
		}()
	}
	wg.Wait()
	if n := len(l.Get([]string{"a.example.com"})); n != MaxEventsPerDomain {
		t.Fatal(n)
	}
}

func TestHealthOf(t *testing.T) {
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	exp := func(d time.Duration) CertificateStatus {
		s := now.Add(d).Format("2006-01-02T15:04:05.000Z")
		return CertificateStatus{Status: StatusActive, ExpiresAt: &s}
	}
	if h := HealthOf(exp(0), now); !h.Expired || !h.RenewalOverdue || *h.DaysUntilExpiry != 0 {
		t.Fatalf("%+v", h)
	}
	if h := HealthOf(exp(-36*time.Hour), now); *h.DaysUntilExpiry != -2 {
		t.Fatal(*h.DaysUntilExpiry)
	}
	bad := "nope"
	if h := HealthOf(CertificateStatus{ExpiresAt: &bad}, now); h.DaysUntilExpiry != nil {
		t.Fatal("unparseable expiry")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("DOCKLIFT_CERTS_HELPER") != "" {
		helperProcess()
		return
	}
	os.Exit(m.Run())
}
