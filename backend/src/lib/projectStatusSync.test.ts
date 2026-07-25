import { test } from 'node:test';
import assert from 'node:assert/strict';
import { aggregateProjectStatus } from './projectStatusSync.js';

test('aggregate: all running → running', () => {
  assert.equal(aggregateProjectStatus(['running', 'running']), 'running');
});

test('aggregate: any error → error', () => {
  assert.equal(aggregateProjectStatus(['running', 'error']), 'error');
});

test('aggregate: mixed running+stopped → degraded (not running)', () => {
  assert.equal(aggregateProjectStatus(['running', 'stopped']), 'degraded');
});

test('aggregate: all stopped → stopped', () => {
  assert.equal(aggregateProjectStatus(['stopped', 'stopped']), 'stopped');
});
