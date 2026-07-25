import test from 'node:test';
import assert from 'node:assert/strict';
import { decideRestoreCommit } from './restoreCommitPolicy.js';

test('setup restore commits and consumes only when reconcile ok + admin present', () => {
  assert.deepEqual(
    decideRestoreCommit({
      reconcileOk: true,
      adminCount: 1,
      dbReplaced: true,
      setupAuth: true,
    }),
    { action: 'commit', consumeSetup: true }
  );
});

test('setup restore aborts and rolls back when reconcile incomplete', () => {
  assert.deepEqual(
    decideRestoreCommit({
      reconcileOk: false,
      adminCount: 2,
      dbReplaced: true,
      setupAuth: true,
    }),
    { action: 'abort', reason: 'Reconciliation incomplete', rollbackDb: true }
  );
});

test('setup restore aborts when restored DB has no admin', () => {
  assert.deepEqual(
    decideRestoreCommit({
      reconcileOk: true,
      adminCount: 0,
      dbReplaced: true,
      setupAuth: true,
    }),
    { action: 'abort', reason: 'Restored database has no administrator (role=admin)', rollbackDb: true }
  );
});

test('authenticated incomplete reconcile keeps data and does not consume setup', () => {
  assert.deepEqual(
    decideRestoreCommit({
      reconcileOk: false,
      adminCount: 1,
      dbReplaced: true,
      setupAuth: false,
    }),
    { action: 'commit', consumeSetup: false }
  );
});
