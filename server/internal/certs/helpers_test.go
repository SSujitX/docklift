package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"docklift/internal/dnscheck"
	"docklift/internal/store"
)

// memStore is an in-memory Store.
type memStore struct {
	mu       sync.Mutex
	settings map[string]string
	user     *store.User
	failGet  error
}

func newMemStore() *memStore { return &memStore{settings: map[string]string{}} }

func (m *memStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failGet != nil {
		return "", false, m.failGet
	}
	v, ok := m.settings[key]
	return v, ok, nil
}

func (m *memStore) SetSetting(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = value
	return nil
}

func (m *memStore) DeleteSetting(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.settings, key)
	return nil
}

func (m *memStore) OldestUser(context.Context) (*store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.user == nil {
		return nil, store.ErrNotFound
	}
	return m.user, nil
}

// fakeDNS answers CheckDomain from a map (default ok).
type fakeDNS struct {
	mu     sync.Mutex
	status map[string]dnscheck.Status
	calls  []string
}

func (f *fakeDNS) CheckDomain(_ context.Context, host string) dnscheck.Check {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, host)
	st, ok := f.status[host]
	if !ok {
		st = dnscheck.StatusOK
	}
	return dnscheck.Check{Domain: host, Status: st, A: []string{}, AAAA: []string{}, Message: "dns " + string(st) + " for " + host}
}

// fakeRunner records invocations and runs an optional side effect.
type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	fn    func(args []string) (ExecResult, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (ExecResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		return ExecResult{}, nil
	}
	return fn(args)
}

func (f *fakeRunner) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type certSpec struct {
	cn       string
	dns      []string
	ips      []string
	emails   []string
	uris     []string
	notAfter time.Time
}

// genCertPEM makes a self-signed certificate.
func genCertPEM(t testing.TB, spec certSpec) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: spec.cn},
		NotBefore:    spec.notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     spec.notAfter,
		DNSNames:     spec.dns,
	}
	for _, ip := range spec.ips {
		tmpl.IPAddresses = append(tmpl.IPAddresses, net.ParseIP(ip))
	}
	tmpl.EmailAddresses = spec.emails
	for _, u := range spec.uris {
		pu, err := url.Parse(u)
		if err != nil {
			t.Fatal(err)
		}
		tmpl.URIs = append(tmpl.URIs, pu)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func writeLive(t testing.TB, root, name, fullchain string) {
	t.Helper()
	live := filepath.Join(root, "live", name)
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), []byte(fullchain), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "privkey.pem"), []byte("-----BEGIN PRIVATE KEY-----\nPLACEHOLDER\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	svc    *Service
	root   string
	data   string
	store  *memStore
	dns    *fakeDNS
	runner *fakeRunner
	now    time.Time
}

func newFixture(t testing.TB, opts ...Option) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		root:   filepath.Join(dir, "le"),
		data:   filepath.Join(dir, "data"),
		store:  newMemStore(),
		dns:    &fakeDNS{status: map[string]dnscheck.Status{}},
		runner: &fakeRunner{},
		now:    time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC),
	}
	all := append([]Option{WithRunner(f.runner), WithClock(func() time.Time { return f.now })}, opts...)
	f.svc = New(Settings{LetsEncryptPath: f.root, DataPath: f.data, CertbotContainer: "docklift-certbot"}, f.store, f.dns, nil, all...)
	return f
}
