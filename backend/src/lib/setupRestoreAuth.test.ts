import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import path from 'path';
import {
  validateSetupToken,
  consumeSetupRestoreSecrets,
  setupTokenPath,
} from './setupRestoreAuth.js';
import { ensureBootstrapSecret, readBootstrapSecret } from './bootstrap.js';
import { config } from './config.js';

test('validateSetupToken does not consume the token file', () => {
  fs.mkdirSync(config.dataPath, { recursive: true });
  const tokenPath = setupTokenPath();
  const token = 'a'.repeat(64);
  const prev = fs.existsSync(tokenPath) ? fs.readFileSync(tokenPath, 'utf8') : null;
  try {
    fs.writeFileSync(tokenPath, token, { mode: 0o600 });
    assert.equal(validateSetupToken(token), true);
    assert.equal(fs.existsSync(tokenPath), true);
    assert.equal(validateSetupToken('wrong'), false);
    assert.equal(fs.existsSync(tokenPath), true);
  } finally {
    if (prev != null) fs.writeFileSync(tokenPath, prev, { mode: 0o600 });
    else if (fs.existsSync(tokenPath)) fs.unlinkSync(tokenPath);
  }
});

test('consumeSetupRestoreSecrets removes setup token and bootstrap secret', () => {
  fs.mkdirSync(config.dataPath, { recursive: true });
  const tokenPath = setupTokenPath();
  const token = 'b'.repeat(64);
  const bootstrapPath = path.join(config.dataPath, '.bootstrap-secret');
  const prevToken = fs.existsSync(tokenPath) ? fs.readFileSync(tokenPath, 'utf8') : null;
  const prevBootstrap = fs.existsSync(bootstrapPath)
    ? fs.readFileSync(bootstrapPath, 'utf8')
    : null;
  try {
    fs.writeFileSync(tokenPath, token, { mode: 0o600 });
    ensureBootstrapSecret();
    assert.ok(readBootstrapSecret());
    consumeSetupRestoreSecrets();
    assert.equal(fs.existsSync(tokenPath), false);
    assert.equal(readBootstrapSecret(), null);
  } finally {
    if (prevToken != null) fs.writeFileSync(tokenPath, prevToken, { mode: 0o600 });
    if (prevBootstrap != null) fs.writeFileSync(bootstrapPath, prevBootstrap, { mode: 0o600 });
  }
});
