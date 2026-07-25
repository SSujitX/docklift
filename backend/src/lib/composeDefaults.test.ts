import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

test('dashboard bind defaults to all interfaces for SERVER_IP:8080 onboarding', () => {
  const compose = fs.readFileSync(path.join(repoRoot, 'docker-compose.yml'), 'utf8');
  assert.match(
    compose,
    /DASHBOARD_BIND:-\s*0\.0\.0\.0/,
    'DASHBOARD_BIND must default to 0.0.0.0 so install prints http://SERVER_IP:8080',
  );
  assert.doesNotMatch(
    compose,
    /DASHBOARD_BIND:-\s*127\.0\.0\.1/,
    'default must not force localhost-only panel (breaks intended install UX)',
  );
});
