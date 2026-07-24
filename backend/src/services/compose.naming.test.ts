import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import os from 'os';
import path from 'path';
import { dedupeScannedServices, scanDockerfiles } from './compose.js';
import { storageVolumeComposeKey, shortPathHash } from '../lib/naming.js';

function fixture(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-compose-naming-'));
}

test('scanDockerfiles dedupes colliding service names with path hash suffix', () => {
  const root = fixture();
  fs.mkdirSync(path.join(root, 'a', 'b'), { recursive: true });
  fs.mkdirSync(path.join(root, 'a-b'), { recursive: true });
  fs.writeFileSync(path.join(root, 'a', 'b', 'Dockerfile'), 'FROM scratch\nEXPOSE 3000\n');
  fs.writeFileSync(path.join(root, 'a-b', 'Dockerfile'), 'FROM scratch\nEXPOSE 4000\n');

  const services = scanDockerfiles(root);
  const names = services.map((svc) => svc.name);
  assert.equal(services.length, 2);
  assert.equal(new Set(names).size, 2);
  assert.equal(names.filter((n) => n === 'a-b').length, 1);
  assert.ok(names.some((n) => /^a-b-[a-f0-9]{4}$/.test(n)));

  fs.rmSync(root, { recursive: true, force: true });
});

test('dedupeScannedServices rejects names that stay duplicated after hashing', () => {
  const dockerfilePath = 'a/Dockerfile';
  const suffix = shortPathHash(dockerfilePath);
  const services = [
    {
      name: 'app',
      dockerfile_path: dockerfilePath,
      context_path: 'a',
      internal_port: 3000,
    },
    {
      name: `app-${suffix}`,
      dockerfile_path: 'b/Dockerfile',
      context_path: 'b',
      internal_port: 3000,
    },
    {
      name: 'app',
      dockerfile_path: dockerfilePath,
      context_path: 'a',
      internal_port: 3000,
    },
  ];

  assert.throws(
    () => dedupeScannedServices(services),
    /Duplicate service name/,
  );
});

test('storageVolumeComposeKey includes docker slug and volume name hash', () => {
  const key = storageVolumeComposeKey('My Service!', 0, 'dl-abc123-data');
  assert.match(key, /^storage_my-service_0_[a-f0-9]{4}$/);
});
