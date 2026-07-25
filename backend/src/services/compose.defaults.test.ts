import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import os from 'os';
import path from 'path';
import yaml from 'js-yaml';
import {
  dockerfileMountsSecret,
  generateRuntimeCompose,
} from './compose.js';

test('runtime compose defaults omit cap_drop ALL and hard mem/cpu caps', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-compose-'));
  const composePath = path.join(dir, 'compose.yml');
  generateRuntimeCompose(
    composePath,
    [{
      name: 'app',
      image: 'node:20',
      internal_port: 3000,
      container_name: 'dl_test_app',
    }],
    [],
    { projectId: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', publishHostPort: false }
  );
  const generated = yaml.load(fs.readFileSync(composePath, 'utf8')) as any;
  assert.equal(generated.services.app.cap_drop, undefined);
  assert.equal(generated.services.app.mem_limit, undefined);
  assert.equal(generated.services.app.cpus, undefined);
  assert.deepEqual(generated.services.app.security_opt, ['no-new-privileges:true']);
  fs.rmSync(dir, { recursive: true, force: true });
});

test('dockerfileMountsSecret detects BuildKit secret mounts', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-df-'));
  const df = path.join(dir, 'Dockerfile');
  fs.writeFileSync(
    df,
    'FROM node:20\nRUN --mount=type=secret,id=NPM_TOKEN cat /run/secrets/NPM_TOKEN\n'
  );
  assert.equal(dockerfileMountsSecret(df, 'NPM_TOKEN'), true);
  assert.equal(dockerfileMountsSecret(df, 'OTHER'), false);
  fs.rmSync(dir, { recursive: true, force: true });
});
