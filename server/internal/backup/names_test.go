package backup

import (
	"strings"
	"testing"
	"time"
)

func TestValidFilename(t *testing.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"docklift-backup-2026-11-27_09-52-00.zip", true},
		{"restore-1764236400000-0123456789abcdef.zip", true},
		{"restore-1.restored-2026-01-02.zip", true},
		{"x.zip", true},
		{".zip", false},
		{"_.zip", true},
		{"notazip.txt", false},
		{"a..zip", false},
		{"sp ace.zip", false},
		{"x.zip.exe", false},
		{"uploads", false},
		{".zip\x00", false},
		{"../x.zip", false},
		{`..\x.zip`, false},
		{"a/b.zip", false},
		{"x.ZIP", false},
		{"", false},
	} {
		if got := ValidFilename(tc.name); got != tc.ok {
			t.Errorf("ValidFilename(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

func TestSanitizeLabel(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"contract test!ab", "contract-test-ab"},
		{"ok_name-1", "ok_name-1"},
		{"héllo", "h-llo"},
		// A character outside the BMP is two UTF-16 units, so two dashes.
		{"a😀b", "a--b"},
		{strings.Repeat("L", 62), strings.Repeat("L", 50)},
		// The cut happens on UTF-16 units before the replacement.
		{strings.Repeat("x", 49) + "😀", strings.Repeat("x", 49) + "-"},
	} {
		if got := SanitizeLabel(tc.in); got != tc.want {
			t.Errorf("SanitizeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTimestampsAndNames(t *testing.T) {
	at := time.Date(2026, 11, 27, 9, 52, 0, 678e6, time.UTC)
	if got := BackupFilename("docklift", at); got != "docklift-backup-2026-11-27_09-52-00.zip" {
		t.Fatalf("BackupFilename = %q", got)
	}
	local := at.In(time.FixedZone("x", 5*3600))
	if got := Timestamp(local); got != "2026-11-27_09-52-00" {
		t.Fatalf("Timestamp must be UTC, got %q", got)
	}
	if got := ISO(local); got != "2026-11-27T09:52:00.678Z" {
		t.Fatalf("ISO = %q", got)
	}
	for _, tc := range []struct{ in, want string }{
		{"restore-1-ab.zip", "restore-1-ab.restored-2026-11-27.zip"},
		{"x.zip.zip", "x.zip.restored-2026-11-27.zip"},
	} {
		if got := RestoredFilename(tc.in, at); got != tc.want {
			t.Errorf("RestoredFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1 KB"},
		{1536, "1.5 KB"},
		{11694080, "11.15 MB"},
		{1048576, "1 MB"},
		{5 * 1024 * 1024 * 1024, "5 GB"},
		{1 << 50, "1024 TB"},
	} {
		if got := FormatBytes(tc.n); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// The restore commit policy, including the authenticated no-admin and
// no-database cases.
func TestDecideCommit(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   CommitInput
		want Decision
	}{
		{"setup restore commits and consumes only when reconcile ok + admin present",
			CommitInput{ReconcileOK: true, AdminCount: 1, DBReplaced: true, SetupAuth: true},
			Decision{Action: ActionCommit, ConsumeSetup: true}},
		{"setup restore aborts and rolls back when reconcile incomplete",
			CommitInput{ReconcileOK: false, AdminCount: 2, DBReplaced: true, SetupAuth: true},
			Decision{Action: ActionAbort, Reason: "Reconciliation incomplete", RollbackDB: true}},
		{"setup restore aborts when restored DB has no admin",
			CommitInput{ReconcileOK: true, AdminCount: 0, DBReplaced: true, SetupAuth: true},
			Decision{Action: ActionAbort, Reason: "Restored database has no administrator (role=admin)", RollbackDB: true}},
		{"authenticated incomplete reconcile keeps data and does not consume setup",
			CommitInput{ReconcileOK: false, AdminCount: 1, DBReplaced: true, SetupAuth: false},
			Decision{Action: ActionCommit}},
		{"authenticated restore without admin aborts",
			CommitInput{ReconcileOK: true, AdminCount: 0, DBReplaced: true},
			Decision{Action: ActionAbort, Reason: ReasonNoAdmin, RollbackDB: true}},
		{"archive without database never rolls back",
			CommitInput{ReconcileOK: true, AdminCount: 0, DBReplaced: false, SetupAuth: true},
			Decision{Action: ActionAbort, Reason: ReasonNoAdmin}},
	} {
		if got := DecideCommit(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
