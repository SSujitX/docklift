package certs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// certMtimeFile holds the newest fullchain.pem mtime the watcher reloaded
// nginx for (Unix milliseconds with fraction, like mtimeMs).
const certMtimeFile = "ssl-cert-mtime"

// emptyScansBeforeClear: live/ may be briefly unreadable on boot, so the
// persisted stamp is cleared only after consecutive empty scans.
const emptyScansBeforeClear = 2

func (s *Service) certMtimePath() string { return filepath.Join(s.cfg.DataPath, certMtimeFile) }

func (s *Service) readPersistedCertMtime() float64 {
	raw, err := os.ReadFile(s.certMtimePath())
	if err != nil {
		return 0
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil || n <= 0 || n != n || n > 1e300 {
		return 0
	}
	return n
}

func (s *Service) writePersistedCertMtime(stamp float64) {
	if err := os.MkdirAll(s.cfg.DataPath, 0o755); err != nil {
		s.log.Warn("[SSL] Could not persist cert mtime", "error", err)
		return
	}
	if stamp <= 0 {
		if err := os.Remove(s.certMtimePath()); err != nil && !os.IsNotExist(err) {
			s.log.Warn("[SSL] Could not persist cert mtime", "error", err)
		}
		return
	}
	if err := WriteFileAtomic(s.certMtimePath(), []byte(strconv.FormatFloat(stamp, 'f', -1, 64)), 0o644); err != nil {
		s.log.Warn("[SSL] Could not persist cert mtime", "error", err)
	}
}

// mtimeMs is the modification time in fractional Unix milliseconds: sec * 1e3 + nsec / 1e6.
func mtimeMs(t time.Time) float64 {
	return float64(t.Unix())*1e3 + float64(t.Nanosecond())/1e6
}

// scanCertMtimes returns the newest live/*/fullchain.pem mtime, 0 when none.
func (s *Service) scanCertMtimes() float64 {
	entries, err := os.ReadDir(s.liveRoot())
	if err != nil {
		return 0
	}
	var max float64
	for _, e := range entries {
		if e.Name() == "README" {
			continue
		}
		st, err := os.Stat(filepath.Join(s.liveRoot(), e.Name(), "fullchain.pem"))
		if err != nil {
			continue
		}
		if m := mtimeMs(st.ModTime()); m > max {
			max = m
		}
	}
	return max
}

// renewWatcher is the state of the certificate renewal watcher.
type renewWatcher struct {
	s          *Service
	reload     func(context.Context) error
	lastStamp  float64
	emptyScans int
}

// tick scans once; it returns true when a reload failed and must be retried.
func (w *renewWatcher) tick(ctx context.Context, reason string) (retry bool) {
	next := w.s.scanCertMtimes()
	if next == 0 {
		w.emptyScans++
		if w.lastStamp != 0 && w.emptyScans >= emptyScansBeforeClear {
			w.lastStamp = 0
			w.s.writePersistedCertMtime(0)
		}
		return false
	}
	w.emptyScans = 0
	// Any mtime change (renew, restore) reloads — including renewals that
	// happened while the backend was down.
	if next == w.lastStamp {
		return false
	}
	w.s.log.Info(fmt.Sprintf("[SSL] Certificate files changed (%s) — reloading nginx-proxy", reason))
	if err := w.reload(ctx); err != nil {
		w.s.log.Warn("[SSL] Renew watcher error", "error", err)
		return true
	}
	w.lastStamp = next
	w.s.writePersistedCertMtime(next)
	return false
}

// StartCertRenewWatcher watches the certificate mtimes and calls reload when
// certbot's renewals (or a restore) change the PEM files. The last-seen mtime
// is persisted, so a renewal while the backend was down still triggers a
// reload at startup. It ticks immediately, then every 5 minutes; a failed
// reload is retried after 15 seconds. Scans and reloads are serialised. The
// watcher stops when ctx is cancelled; the returned channel closes then.
func (s *Service) StartCertRenewWatcher(ctx context.Context, reload func(context.Context) error) <-chan struct{} {
	done := make(chan struct{})
	w := &renewWatcher{s: s, reload: reload, lastStamp: s.readPersistedCertMtime()}
	go func() {
		defer close(done)
		poll := time.NewTicker(s.pollInterval)
		defer poll.Stop()
		var retry <-chan time.Time
		var retryTimer *time.Timer
		defer func() {
			if retryTimer != nil {
				retryTimer.Stop()
			}
		}()
		schedule := func(failed bool) {
			if failed && retry == nil {
				retryTimer = time.NewTimer(s.retryDelay)
				retry = retryTimer.C
			}
		}
		if ctx.Err() != nil {
			return
		}
		schedule(w.tick(ctx, "startup"))
		for {
			select {
			case <-ctx.Done():
				return
			case <-poll.C:
				schedule(w.tick(ctx, "poll"))
			case <-retry:
				retry = nil
				schedule(w.tick(ctx, "retry"))
			}
		}
	}()
	return done
}

// WriteFileAtomic writes data to a temporary file in path's directory and
// renames it over path, so readers never see a partial file. The temporary
// name does not end in ".conf", so nginx's conf.d/*.conf include never picks
// it up.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir, base := filepath.Split(path)
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
