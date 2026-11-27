package backup

// Commit actions of DecideCommit.
const (
	ActionCommit = "commit"
	ActionAbort  = "abort"
)

// Abort reasons of DecideCommit (shown in the restore log).
const (
	ReasonNoAdmin          = "Restored database has no administrator (role=admin)"
	ReasonReconcileFailure = "Reconciliation incomplete"
)

// CommitInput is what the commit policy looks at once files and
// reconciliation are done.
type CommitInput struct {
	ReconcileOK bool
	// AdminCount counts users with role "admin" (not all users).
	AdminCount int
	DBReplaced bool
	// SetupAuth is true for a fresh-install restore authorised by the setup token.
	SetupAuth bool
}

// Decision is the outcome of DecideCommit. For ActionCommit, ConsumeSetup
// says whether the one-time setup token and bootstrap secret are spent; for
// ActionAbort, Reason is logged and RollbackDB says whether the database is
// put back from the .pre-restore copy.
type Decision struct {
	Action       string
	ConsumeSetup bool
	Reason       string
	RollbackDB   bool
}

// DecideCommit applies the restore commit policy: a restore
// without an administrator is never committed; a setup-token restore whose
// reconciliation failed is rolled back so the setup code stays usable; an
// authenticated restore keeps its data even when reconciliation failed but
// never consumes setup secrets.
func DecideCommit(in CommitInput) Decision {
	if in.AdminCount < 1 {
		return Decision{Action: ActionAbort, Reason: ReasonNoAdmin, RollbackDB: in.DBReplaced}
	}
	if !in.ReconcileOK {
		if in.SetupAuth {
			return Decision{Action: ActionAbort, Reason: ReasonReconcileFailure, RollbackDB: in.DBReplaced}
		}
		return Decision{Action: ActionCommit}
	}
	return Decision{Action: ActionCommit, ConsumeSetup: in.SetupAuth}
}
