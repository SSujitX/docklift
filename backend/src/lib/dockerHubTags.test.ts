import { describe, expect, test } from 'bun:test';
import {
  filterHubTagsToVersions,
  matchesEngineTagPattern,
} from './dockerHubTags.js';

describe('dockerHubTags', () => {
  test('filters postgres majors and alpine, prefers alpine recommended', () => {
    const versions = filterHubTagsToVersions(
      'postgres',
      [
        'latest',
        '18.4-trixie',
        '18.4',
        '18',
        '18-alpine',
        '18.4-alpine',
        '17',
        '17-alpine',
        '16-bookworm',
        '16-alpine',
        '19beta2-alpine',
        '15-alpine',
      ],
      '16-alpine',
    );
    const tags = versions.map((v) => v.tag);
    expect(tags).toContain('18-alpine');
    expect(tags).toContain('18');
    expect(tags).toContain('17-alpine');
    expect(tags).toContain('latest');
    expect(tags).not.toContain('18.4-trixie');
    expect(tags).not.toContain('16-bookworm');
    expect(tags).not.toContain('19beta2-alpine');
    // Patch dropped when major exists
    expect(tags).not.toContain('18.4');
    expect(tags).not.toContain('18.4-alpine');
    // Static recommended kept when present
    expect(versions.find((v) => v.recommended)?.tag).toBe('16-alpine');
  });

  test('recommends newest alpine when static rec missing', () => {
    const versions = filterHubTagsToVersions(
      'postgres',
      ['18-alpine', '17-alpine', '18', '17'],
      '99-alpine',
    );
    expect(versions[0]?.tag).toBe('18-alpine');
    expect(versions.find((v) => v.recommended)?.tag).toBe('18-alpine');
  });

  test('matchesEngineTagPattern', () => {
    expect(matchesEngineTagPattern('postgres', '18-alpine')).toBe(true);
    expect(matchesEngineTagPattern('postgres', '18.4-bookworm')).toBe(false);
    expect(matchesEngineTagPattern('mysql', '8.4')).toBe(true);
    expect(matchesEngineTagPattern('mongodb', '8')).toBe(true);
  });
});
