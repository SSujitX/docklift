/**
 * Decide whether a restore may consume one-time setup credentials and/or
 * roll the database back after reconciliation.
 */
export type RestoreCommitDecision =
  | { action: 'commit'; consumeSetup: boolean }
  | { action: 'abort'; reason: string; rollbackDb: boolean };

export function decideRestoreCommit(input: {
  reconcileOk: boolean;
  /** Count of users with role === 'admin' (not all users). */
  adminCount: number;
  dbReplaced: boolean;
  setupAuth: boolean;
}): RestoreCommitDecision {
  if (input.adminCount < 1) {
    return {
      action: 'abort',
      reason: 'Restored database has no administrator (role=admin)',
      rollbackDb: input.dbReplaced,
    };
  }

  if (!input.reconcileOk) {
    // Fresh-setup must abort + roll DB back so the setup code can still be used.
    // Authenticated restores keep applied data but never consume setup secrets.
    if (input.setupAuth) {
      return {
        action: 'abort',
        reason: 'Reconciliation incomplete',
        rollbackDb: input.dbReplaced,
      };
    }
    return { action: 'commit', consumeSetup: false };
  }

  return { action: 'commit', consumeSetup: input.setupAuth };
}
