import { describe, expect, test } from 'bun:test';
import {
  buildKeepImageSet,
  dockliftImageTag,
  isDockliftAppImage,
  parseImageTagsJson,
  pickTaggedImageMaps,
  reconstructInProgressTags,
  rollbackTargetGuard,
} from './imageCleanup.js';

describe('pickTaggedImageMaps', () => {
  const deployA = { api: 'docklift-abcdef12-api:aaaaaaaa' };
  const deployB = { api: 'docklift-abcdef12-api:bbbbbbbb' };
  const deployC = { api: 'docklift-abcdef12-api:cccccccc' };

  test('stop / restart rows do not use up a keep slot', () => {
    // Newest first: deploy B, then a stop and a restart, then deploy A.
    const rows = [
      { image_tags: deployB },
      { image_tags: null },
      { image_tags: null },
      { image_tags: deployA },
    ];
    expect(pickTaggedImageMaps(rows, 2)).toEqual([deployB, deployA]);
    // The previous image stays in the keep-set, so rollback to A still works.
    const keep = buildKeepImageSet(pickTaggedImageMaps(rows, 2));
    expect(keep.has(deployA.api)).toBe(true);
    expect(keep.has(deployB.api)).toBe(true);
  });

  test('still keeps only the newest two tagged deploys', () => {
    const rows = [{ image_tags: deployC }, { image_tags: deployB }, { image_tags: deployA }];
    expect(pickTaggedImageMaps(rows, 2)).toEqual([deployC, deployB]);
  });

  test('handles empty and untagged-only history', () => {
    expect(pickTaggedImageMaps([], 2)).toEqual([]);
    expect(pickTaggedImageMaps([{ image_tags: null }, { image_tags: {} }], 2)).toEqual([]);
  });
});

describe('isDockliftAppImage', () => {
  test('accepts docklift project service tags', () => {
    expect(isDockliftAppImage('docklift-abcdef12-api:deadbeef')).toBe(true);
    expect(isDockliftAppImage('docklift-abcdef12-web-app:12345678')).toBe(true);
  });

  test('rejects upstream and platform images', () => {
    expect(isDockliftAppImage('postgres:16')).toBe(false);
    expect(isDockliftAppImage('nginx:alpine')).toBe(false);
    expect(isDockliftAppImage('docklift-backend:latest')).toBe(false);
    expect(isDockliftAppImage('docklift-abcdef1-api:tag')).toBe(false); // project8 too short
  });
});

describe('buildKeepImageSet', () => {
  test('unions tags from last two success maps', () => {
    const keep = buildKeepImageSet([
      { api: 'docklift-abcdef12-api:11111111', web: 'docklift-abcdef12-web:11111111' },
      { api: 'docklift-abcdef12-api:22222222', web: 'docklift-abcdef12-web:22222222' },
    ]);
    expect(keep.size).toBe(4);
    expect(keep.has('docklift-abcdef12-api:11111111')).toBe(true);
    expect(keep.has('docklift-abcdef12-api:22222222')).toBe(true);
  });

  test('ignores managed / non-docklift images in maps', () => {
    const keep = buildKeepImageSet([
      { api: 'docklift-abcdef12-api:aaaaaaaa', db: 'postgres:16' },
    ]);
    expect([...keep]).toEqual(['docklift-abcdef12-api:aaaaaaaa']);
  });

  test('applies fallback current when persistence races', () => {
    const keep = buildKeepImageSet([], {
      api: 'docklift-abcdef12-api:cccccccc',
    });
    expect(keep.has('docklift-abcdef12-api:cccccccc')).toBe(true);
  });

  test('never includes empty or invalid maps', () => {
    const keep = buildKeepImageSet([null, undefined, {}, { api: '' }]);
    expect(keep.size).toBe(0);
  });
});

describe('parseImageTagsJson', () => {
  test('parses service→tag object', () => {
    expect(parseImageTagsJson({ api: 'docklift-abcdef12-api:1' })).toEqual({
      api: 'docklift-abcdef12-api:1',
    });
  });

  test('rejects arrays and empty objects', () => {
    expect(parseImageTagsJson([])).toBeNull();
    expect(parseImageTagsJson({})).toBeNull();
    expect(parseImageTagsJson(null)).toBeNull();
  });
});

describe('reconstructInProgressTags', () => {
  test('builds deterministic tags from project + deployment + services', () => {
    const projectId = 'abcdef12-9999-4000-8000-000000000001';
    const deploymentId = 'deadbeef-1111-4000-8000-000000000002';
    const tags = reconstructInProgressTags(projectId, deploymentId, ['api', 'web']);
    expect(tags.api).toBe(dockliftImageTag(projectId, 'api', deploymentId));
    expect(tags.web).toBe(dockliftImageTag(projectId, 'web', deploymentId));
    expect(isDockliftAppImage(tags.api)).toBe(true);
  });
});

describe('rollbackTargetGuard', () => {
  const tags = { api: 'docklift-abcdef12-api:oldold01' };

  test('rejects current success', () => {
    const r = rollbackTargetGuard({
      targetId: 'dep-a',
      latestSuccessId: 'dep-a',
      status: 'success',
      imageTags: tags,
    });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.status).toBe(400);
  });

  test('rejects missing image tags', () => {
    const r = rollbackTargetGuard({
      targetId: 'dep-b',
      latestSuccessId: 'dep-a',
      status: 'success',
      imageTags: null,
    });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.status).toBe(409);
  });

  test('rejects non-success', () => {
    const r = rollbackTargetGuard({
      targetId: 'dep-b',
      latestSuccessId: 'dep-a',
      status: 'failed',
      imageTags: tags,
    });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.status).toBe(404);
  });

  test('accepts previous success with tags', () => {
    const r = rollbackTargetGuard({
      targetId: 'dep-b',
      latestSuccessId: 'dep-a',
      status: 'success',
      imageTags: tags,
    });
    expect(r).toEqual({ ok: true });
  });
});
