package backup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docklift/internal/maintenance"
)

// InProgressMarkerName is the file in DATA_PATH that exists while a restore
// is past its point of no return (the first change to live data) and not yet
// committed or rolled back. A process that finds it at boot crashed or lost
// power mid-restore: the marker is promoted to the restore-critical seal
// (.restore-critical), which keeps maintenance mode and the restore lock on
// until an operator has checked the data and cleared the seal.
//
// It is separate from the seal so that a restore in progress is reported as
// plain maintenance (and its lock as a 409), exactly as 2.1.6 did.
const InProgressMarkerName = ".restore-in-progress"

// inProgress is the marker's JSON content.
type inProgress struct {
	At             string `json:"at"`
	Stage          string `json:"stage"`
	Source         string `json:"source"`
	LiveDBPath     string `json:"liveDbPath"`
	PreRestorePath string `json:"preRestorePath"`
}

func markerPath(p Paths) string { return filepath.Join(p.DataPath, InProgressMarkerName) }

// writeMarker records (durably) that a restore is applying stage.
func writeMarker(p Paths, m inProgress) error {
	if err := os.MkdirAll(p.DataPath, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileDurable(markerPath(p), raw, 0o600)
}

// clearMarker removes the marker (durably).
func clearMarker(p Paths) error {
	if err := os.Remove(markerPath(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	syncDir(p.DataPath)
	return nil
}

func readMarker(p Paths) (inProgress, bool) {
	raw, err := os.ReadFile(markerPath(p))
	if err != nil {
		return inProgress{}, false
	}
	var m inProgress
	_ = json.Unmarshal(raw, &m)
	return m, true
}

// interruptedDetail is the seal detail for a restore that never finished.
func interruptedDetail(m inProgress) string {
	stage := m.Stage
	if stage == "" {
		stage = "an unknown stage"
	}
	detail := "Restore was interrupted during " + stage
	if m.At != "" {
		detail += " (started " + m.At
		if m.Source != "" {
			detail += ", source " + m.Source
		}
		detail += ")"
	}
	return detail + ": live data may be partially restored. Check the database (the copy taken before the restore is " +
		m.PreRestorePath + ") and the restored directories, then clear the seal."
}

// RecoverInterrupted runs at boot, before traffic is served. If a restore
// was interrupted past its point of no return, it seals the instance
// (maintenance on, restore lock held, .restore-critical written) and removes
// the marker. An existing seal (e.g. a failed rollback) is kept as is. If the
// seal cannot be written the marker stays, so the next boot tries again, and
// this process holds maintenance and the lock in memory.
func RecoverInterrupted(p Paths, state *maintenance.State, log *slog.Logger) {
	m, ok := readMarker(p)
	if !ok {
		return
	}
	if m.LiveDBPath == "" {
		m.LiveDBPath = p.DatabasePath
	}
	if m.PreRestorePath == "" {
		m.PreRestorePath = p.PreRestorePath()
	}
	if !state.Critical() {
		detail := interruptedDetail(m)
		if err := state.EnterCritical(detail, m.LiveDBPath); err != nil {
			log.Error("[CRITICAL] Interrupted restore detected but the seal could not be written", "error", err, "detail", detail)
			state.Enter("CRITICAL restore recovery required — " + detail)
			state.ForceRestoreLock("restore-critical")
			return
		}
		if err := syncFile(state.CriticalMarkerPath()); err != nil {
			log.Warn("could not flush the restore-critical seal", "error", err)
		}
		log.Error("[CRITICAL] Interrupted restore detected at boot — instance sealed", "detail", detail)
	}
	if err := clearMarker(p); err != nil {
		log.Warn("could not remove the restore-in-progress marker", "error", err)
	}
}

// CleanupOnBoot removes the disposable work files a crashed backup or
// restore can leave behind: extraction directories (BACKUP_PATH/temp-restore-*),
// unfinished archives and uploads (*.partial), VACUUM INTO snapshots
// (.docklift-backup-*.db next to the database) and an unfinished
// .pre-restore copy. Restored data, the .pre-restore copy itself and the
// fscopy staging/prev directories inside restore targets are left alone (the
// latter hold the pre-restore content an operator may need; the next
// restore reclaims them).
func CleanupOnBoot(p Paths, log *slog.Logger) {
	remove := func(path string) {
		if err := os.RemoveAll(path); err != nil {
			log.Warn("could not remove leftover backup work file", "path", path, "error", err)
			return
		}
		log.Info("Removed leftover backup work file", "path", path)
	}
	for _, dir := range []string{p.BackupDir, p.UploadsDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			switch {
			case dir == p.BackupDir && e.IsDir() && strings.HasPrefix(name, tempRestorePrefix):
				remove(filepath.Join(dir, name))
			case !e.IsDir() && strings.HasSuffix(name, partialSuffix):
				remove(filepath.Join(dir, name))
			}
		}
	}
	dbDir := filepath.Dir(p.DatabasePath)
	if entries, err := os.ReadDir(dbDir); err == nil {
		preTmp := filepath.Base(p.PreRestorePath() + preRestoreTempSuffix)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				continue
			}
			if (strings.HasPrefix(name, snapshotPrefix) && strings.Contains(name, ".db")) || strings.HasPrefix(name, preTmp) {
				remove(filepath.Join(dbDir, name))
			}
		}
	}
	// Temporary files of an interrupted marker write (writeFileDurable).
	if matches, err := filepath.Glob(filepath.Join(p.DataPath, "."+InProgressMarkerName+".tmp-*")); err == nil {
		for _, m := range matches {
			remove(m)
		}
	}
}

// markerFor builds the marker for a restore of source at stage.
func markerFor(p Paths, source, stage string, started time.Time) inProgress {
	return inProgress{
		At:             ISO(started),
		Stage:          stage,
		Source:         source,
		LiveDBPath:     p.DatabasePath,
		PreRestorePath: p.PreRestorePath(),
	}
}
