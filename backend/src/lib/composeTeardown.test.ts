import test from 'node:test';
import assert from 'node:assert/strict';
import {
  isComposeTeardownOk,
  shouldReconnectProxyAfterFailedTeardown,
  verifyComposeProjectGone,
} from './composeTeardown.js';

const okResult = { status: 0, stdout: '', stderr: '', pid: 1, output: [], signal: null } as any;
const failResult = {
  status: 1,
  stdout: '',
  stderr: 'error while removing network: has active endpoints',
  pid: 1,
  output: [],
  signal: null,
} as any;
const unrelatedNotFound = {
  status: 1,
  stdout: '',
  stderr: 'configuration file not found: /tmp/missing.yml',
  pid: 1,
  output: [],
  signal: null,
} as any;
const contextNotFound = {
  status: 1,
  stdout: '',
  stderr: 'context "desktop-linux" not found',
  pid: 1,
  output: [],
  signal: null,
} as any;

test('exit 0 is OK only when probe shows no resources', () => {
  assert.equal(
    isComposeTeardownOk(okResult, 'dl-app-aaaaaaaa', () => ({
      containerIds: [],
      networkIds: [],
    })),
    true
  );
  assert.equal(
    isComposeTeardownOk(okResult, 'dl-app-aaaaaaaa', () => ({
      containerIds: ['abc'],
      networkIds: [],
    })),
    false
  );
});

test('nonzero exit is OK only when exact-label probe is empty', () => {
  assert.equal(
    isComposeTeardownOk(failResult, 'dl-app-aaaaaaaa', () => ({
      containerIds: [],
      networkIds: [],
    })),
    true
  );
  assert.equal(
    isComposeTeardownOk(failResult, 'dl-app-aaaaaaaa', () => ({
      containerIds: ['still-here'],
      networkIds: [],
    })),
    false
  );
});

test('unrelated "not found" stderr does NOT authorize teardown', () => {
  const stillPresent = () => ({ containerIds: ['c1'], networkIds: ['n1'] });
  assert.equal(isComposeTeardownOk(unrelatedNotFound, 'dl-app-aaaaaaaa', stillPresent), false);
  assert.equal(isComposeTeardownOk(contextNotFound, 'dl-app-aaaaaaaa', stillPresent), false);
});

test('verifyComposeProjectGone requires empty containers and networks', () => {
  assert.equal(
    verifyComposeProjectGone('x', () => ({ containerIds: [], networkIds: [] })),
    true
  );
  assert.equal(
    verifyComposeProjectGone('x', () => ({ containerIds: [], networkIds: ['n'] })),
    false
  );
});

test('shouldReconnectProxyAfterFailedTeardown only when teardown failed', () => {
  assert.equal(shouldReconnectProxyAfterFailedTeardown(false), true);
  assert.equal(shouldReconnectProxyAfterFailedTeardown(true), false);
});
