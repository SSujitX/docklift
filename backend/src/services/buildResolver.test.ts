import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import os from 'os';
import path from 'path';
import yaml from 'js-yaml';
import { resolveProjectBuild } from './buildResolver.js';
import { buildServiceImage, summarizeBuildFailure } from './buildRunner.js';
import { generateRuntimeCompose } from './compose.js';

function fixture(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-build-'));
}

test('Auto prefers a repository Dockerfile', () => {
  const root = fixture();
  fs.writeFileSync(path.join(root, 'Dockerfile'), 'FROM node:24-alpine\nEXPOSE 4321\n');
  const result = resolveProjectBuild(root, { buildType: 'auto', internalPort: 3000 });
  assert.equal(result.resolvedType, 'dockerfile');
  assert.equal(result.services[0].dockerfilePath, 'Dockerfile');
  assert.equal(result.services[0].internalPort, 4321);
  fs.rmSync(root, { recursive: true, force: true });
});

test('Auto falls back to Railpack and reports manifests', () => {
  const root = fixture();
  fs.writeFileSync(path.join(root, 'package.json'), '{"scripts":{"start":"node index.js"}}');
  const result = resolveProjectBuild(root, { buildType: 'auto', internalPort: 8080 });
  assert.equal(result.resolvedType, 'railpack');
  assert.deepEqual(result.manifests, ['package.json']);
  assert.equal(result.services[0].internalPort, 8080);
  fs.rmSync(root, { recursive: true, force: true });
});

test('Base directory scopes monorepo detection', () => {
  const root = fixture();
  fs.mkdirSync(path.join(root, 'apps', 'web'), { recursive: true });
  fs.writeFileSync(path.join(root, 'package.json'), '{"private":true}');
  fs.writeFileSync(path.join(root, 'apps', 'web', 'package.json'), '{"scripts":{"start":"vite"}}');
  const result = resolveProjectBuild(root, {
    buildType: 'auto',
    baseDirectory: 'apps/web',
    internalPort: 4173,
  });
  assert.equal(result.resolvedType, 'railpack');
  assert.equal(result.baseDirectory, 'apps/web');
  assert.equal(result.services[0].contextPath, 'apps/web');
  assert.equal(result.services[0].internalPort, 4173);
  fs.rmSync(root, { recursive: true, force: true });
});

test('Build paths cannot escape the project', () => {
  const root = fixture();
  assert.throws(
    () => resolveProjectBuild(root, { buildType: 'dockerfile', dockerfilePath: '../Dockerfile' }),
    /inside the project/
  );
  fs.rmSync(root, { recursive: true, force: true });
});

test('Protected host variables cannot be used as build variables', async () => {
  const root = fixture();
  fs.writeFileSync(path.join(root, 'Dockerfile'), 'FROM scratch\n');
  await assert.rejects(
    buildServiceImage({
      projectPath: root,
      statePath: path.join(root, '.state'),
      service: {
        name: 'app',
        builder: 'dockerfile',
        contextPath: '.',
        dockerfilePath: 'Dockerfile',
        internalPort: 3000,
      },
      imageTag: 'unused:test',
      envVars: [{ key: 'DOCKER_HOST', value: 'tcp://attacker', is_build_arg: true }],
      writeLog: () => {},
    }),
    /reserved by DockLift/
  );
  fs.rmSync(root, { recursive: true, force: true });
});

test('Stale npm lockfiles produce an actionable build error', () => {
  const message = summarizeBuildFailure(
    'docker',
    'npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.',
    1
  );
  assert.match(message, /Run "npm install"/);
  assert.match(message, /commit the updated package-lock\.json/);
});

test('Runtime compose uses images and persistent named volumes without touching source compose', () => {
  const root = fixture();
  const sourceCompose = path.join(root, 'docker-compose.yml');
  const stateCompose = path.join(root, '.state', 'compose.yml');
  fs.writeFileSync(sourceCompose, 'services:\n  user-service:\n    image: user/image\n');
  const projectId = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';
  generateRuntimeCompose(
    stateCompose,
    [{
      name: 'app',
      image: 'docklift-app:test',
      internal_port: 3000,
      port: 5500,
      container_name: 'dl_test_app',
      volumes: [{ key: 'storage_app_0', name: 'dl-test-data', mountPath: '/app/data' }],
    }],
    [
      { key: 'NODE_ENV', value: 'production', is_build_arg: false, is_runtime: true },
      {
        key: 'DATABASE_URL',
        value: 'postgresql://db.example/app',
        is_build_arg: false,
        is_runtime: true,
      },
    ],
    { projectId, publishHostPort: true }
  );
  const generated = yaml.load(fs.readFileSync(stateCompose, 'utf8')) as any;
  assert.equal(generated.services.app.image, 'docklift-app:test');
  assert.equal(generated.services.app.build, undefined);
  assert.deepEqual(generated.services.app.volumes, ['storage_app_0:/app/data']);
  assert.equal(generated.volumes.storage_app_0.name, 'dl-test-data');
  assert.equal(generated.volumes.storage_app_0.external, true);
  assert.equal(generated.services.app.environment.DATABASE_URL, 'postgresql://db.example/app');
  assert.deepEqual(generated.services.app.ports, ['5500:3000']);
  assert.equal(generated.networks.project.name, 'dl-net-aaaaaaaa');
  assert.equal(generated.services.app.labels['com.docklift.project'], projectId);
  assert.match(fs.readFileSync(sourceCompose, 'utf8'), /user\/image/);

  // Without publishHostPort, no host ports
  const noPublish = path.join(root, '.state', 'compose-nopub.yml');
  generateRuntimeCompose(
    noPublish,
    [{
      name: 'app',
      image: 'docklift-app:test',
      internal_port: 3000,
      port: 5500,
      container_name: 'dl_test_app',
    }],
    [],
    { projectId, publishHostPort: false }
  );
  const nopub = yaml.load(fs.readFileSync(noPublish, 'utf8')) as any;
  assert.equal(nopub.services.app.ports, undefined);

  fs.rmSync(root, { recursive: true, force: true });
});
