import test from 'node:test';
import assert from 'node:assert/strict';
import { isProxyAlreadyConnectedError } from './docker.js';
import { shouldReconnectProxyAfterFailedTeardown } from '../lib/composeTeardown.js';

test('isProxyAlreadyConnectedError recognizes docker duplicate endpoint errors', () => {
  assert.equal(
    isProxyAlreadyConnectedError(new Error('endpoint with name x already exists in network y')),
    true
  );
  assert.equal(isProxyAlreadyConnectedError('container already connected'), true);
  assert.equal(isProxyAlreadyConnectedError(new Error('network not found')), false);
});

test('failed teardown requires proxy reconnect; success does not', () => {
  assert.equal(shouldReconnectProxyAfterFailedTeardown(false), true);
  assert.equal(shouldReconnectProxyAfterFailedTeardown(true), false);
});

test('connectProxyToProjectNetwork surfaces non-duplicate failures', async () => {
  const mod = await import('./docker.js');
  assert.equal(typeof mod.connectProxyToProjectNetwork, 'function');
  assert.equal(typeof mod.disconnectProxyFromProjectNetwork, 'function');

  // Missing project network → must reject (never silent success)
  await assert.rejects(
    () => mod.connectProxyToProjectNetwork('00000000-0000-0000-0000-000000000099'),
    (err: unknown) => err instanceof Error || typeof err === 'object'
  );
});
