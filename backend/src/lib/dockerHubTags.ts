/**
 * Live Docker Hub tags for managed database images.
 * Cached; falls back to static engine.versions when Hub is unreachable.
 */

import type { DatabaseEngineId, DatabaseEngineVersion } from './databaseEngines.js';

const HUB_TTL_MS = 6 * 60 * 60 * 1000; // 6 hours
const HUB_TIMEOUT_MS = 10_000;
const MAX_PAGES = 3;
const PAGE_SIZE = 100;
const MAX_VERSIONS = 36;

type CacheEntry = {
  at: number;
  versions: DatabaseEngineVersion[];
  /** Per-entry TTL — incomplete Hub pages use a short TTL, not 6h. */
  ttlMs: number;
};

const cache = new Map<string, CacheEntry>();
const HUB_INCOMPLETE_TTL_MS = 5 * 60 * 1000; // 5 minutes

/** Official library image on Docker Hub (postgres → library/postgres). */
function libraryRepo(imageRepo: string): string {
  return imageRepo.includes('/') ? imageRepo : `library/${imageRepo}`;
}

type HubFetchResult = {
  names: string[];
  /** False when a page failed/aborted mid-pagination — do not cache. */
  complete: boolean;
};

async function fetchHubTagNames(imageRepo: string): Promise<HubFetchResult> {
  const repo = libraryRepo(imageRepo);
  const names: string[] = [];
  for (let page = 1; page <= MAX_PAGES; page++) {
    const url = `https://hub.docker.com/v2/repositories/${repo}/tags?page_size=${PAGE_SIZE}&page=${page}`;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), HUB_TIMEOUT_MS);
    try {
      const res = await fetch(url, {
        signal: controller.signal,
        headers: { Accept: 'application/json' },
      });
      if (!res.ok) {
        return { names, complete: false };
      }
      const data = (await res.json()) as {
        results?: { name?: string }[];
        next?: string | null;
      };
      const batch = (data.results || [])
        .map((r) => (typeof r.name === 'string' ? r.name : ''))
        .filter(Boolean);
      if (!batch.length) {
        return { names, complete: true };
      }
      names.push(...batch);
      if (!data.next) {
        return { names, complete: true };
      }
    } catch {
      return { names, complete: false };
    } finally {
      clearTimeout(timer);
    }
  }
  // Hit MAX_PAGES with more pages available — usable but incomplete catalog
  return { names, complete: false };
}

function isPreRelease(tag: string): boolean {
  return /alpha|beta|rc|preview|nightly|dev/i.test(tag);
}

/** Keep operator-friendly tags; drop distro/arch noise. */
function tagAllowed(engineId: DatabaseEngineId, tag: string): boolean {
  if (!tag || isPreRelease(tag)) return false;
  if (tag === 'latest') return true;
  // Skip alpine3.xx, bookworm, trixie, oraclelinux, windows, …
  if (/(alpine\d|bookworm|bullseye|trixie|slim|window|nanoserver|ubi|oraclelinux)/i.test(tag)) {
    return false;
  }

  switch (engineId) {
    case 'postgres':
    case 'redis':
      // 18, 18.4, 18-alpine, 18.4-alpine
      return /^\d+(\.\d+)?(-alpine)?$/.test(tag);
    case 'mysql':
      // 8.4, 8.0, 8.4-oracle
      return /^\d+\.\d+(-oracle)?$/.test(tag) || tag === 'latest';
    case 'mariadb':
      // 11, 11.4, 10.11
      return /^\d+(\.\d+)?$/.test(tag) || tag === 'latest';
    case 'mongodb':
      // 8, 8.0, 7.0
      return /^\d+(\.\d+)?$/.test(tag) || tag === 'latest';
    default:
      return false;
  }
}

function parseVersionParts(tag: string): {
  major: number;
  minor: number;
  alpine: boolean;
  latest: boolean;
} {
  if (tag === 'latest') {
    return { major: -1, minor: -1, alpine: false, latest: true };
  }
  const alpine = tag.endsWith('-alpine');
  const core = tag.replace(/-alpine$/, '').replace(/-oracle$/, '');
  const [a, b] = core.split('.');
  return {
    major: Number(a) || 0,
    minor: b != null ? Number(b) || 0 : -1,
    alpine,
    latest: false,
  };
}

function labelForTag(tag: string): string {
  if (tag === 'latest') return 'latest';
  if (tag.endsWith('-alpine')) {
    return `${tag.slice(0, -'-alpine'.length)} · Alpine`;
  }
  if (tag.endsWith('-oracle')) {
    return `${tag.slice(0, -'-oracle'.length)} · Oracle Linux`;
  }
  return tag;
}

function sortTags(a: string, b: string): number {
  const pa = parseVersionParts(a);
  const pb = parseVersionParts(b);
  if (pa.latest !== pb.latest) return pa.latest ? 1 : -1; // latest last
  if (pb.major !== pa.major) return pb.major - pa.major;
  if (pb.minor !== pa.minor) return pb.minor - pa.minor;
  // Prefer alpine before debian for same version
  if (pa.alpine !== pb.alpine) return pa.alpine ? -1 : 1;
  return a.localeCompare(b);
}

/**
 * Prefer major lines (16, 16-alpine) over every patch (16.14) when both exist,
 * but keep minors when no bare major is published.
 */
function preferMajorLines(tags: string[]): string[] {
  const set = new Set(tags);
  const majors = new Set<string>();
  for (const tag of tags) {
    if (tag === 'latest') continue;
    const parts = parseVersionParts(tag);
    const suffix = tag.includes('-oracle')
      ? '-oracle'
      : parts.alpine
        ? '-alpine'
        : '';
    const majorKey = `${parts.major}${suffix}`;
    if (set.has(majorKey)) majors.add(majorKey);
  }

  return tags.filter((tag) => {
    if (tag === 'latest') return true;
    const parts = parseVersionParts(tag);
    if (parts.minor < 0) return true; // already a major tag
    const suffix = tag.includes('-oracle')
      ? '-oracle'
      : parts.alpine
        ? '-alpine'
        : '';
    const majorKey = `${parts.major}${suffix}`;
    // Drop patch/minor when the major tag exists
    return !majors.has(majorKey);
  });
}

export function filterHubTagsToVersions(
  engineId: DatabaseEngineId,
  tagNames: string[],
  recommendedTag?: string,
): DatabaseEngineVersion[] {
  const allowed = [...new Set(tagNames.filter((t) => tagAllowed(engineId, t)))];
  const trimmed = preferMajorLines(allowed).sort(sortTags).slice(0, MAX_VERSIONS);

  let recommended = recommendedTag && trimmed.includes(recommendedTag)
    ? recommendedTag
    : undefined;
  if (!recommended) {
    // Prefer newest alpine major, else newest non-latest
    recommended =
      trimmed.find((t) => t.endsWith('-alpine')) ||
      trimmed.find((t) => t !== 'latest') ||
      trimmed[0];
  }

  return trimmed.map((tag) => ({
    tag,
    label: labelForTag(tag),
    ...(tag === recommended ? { recommended: true as const } : {}),
  }));
}

export async function fetchLiveEngineVersions(
  engineId: DatabaseEngineId,
  imageRepo: string,
  fallback: DatabaseEngineVersion[],
): Promise<DatabaseEngineVersion[]> {
  const cached = cache.get(engineId);
  if (cached && Date.now() - cached.at < cached.ttlMs) {
    return cached.versions;
  }

  const recommended =
    fallback.find((v) => v.recommended)?.tag || fallback[0]?.tag;

  try {
    const { names, complete } = await fetchHubTagNames(imageRepo);
    if (!names.length) {
      return fallback;
    }
    const versions = filterHubTagsToVersions(engineId, names, recommended);
    if (!versions.length) {
      return fallback;
    }
    // Complete catalogs: 6h. Truncated MAX_PAGES sets: short TTL so Hub is not hammered
    // but a partial list cannot freeze for half a day.
    cache.set(engineId, {
      at: Date.now(),
      versions,
      ttlMs: complete ? HUB_TTL_MS : HUB_INCOMPLETE_TTL_MS,
    });
    if (!complete) {
      console.warn(
        `[dockerHubTags] ${engineId}: incomplete Hub page set — short-TTL cache`,
      );
    }
    return versions;
  } catch (err) {
    console.warn(
      `[dockerHubTags] ${engineId}: using static versions (${err instanceof Error ? err.message : err})`,
    );
    return fallback;
  }
}

/** True if tag looks like a publishable Hub tag for this engine (create-time gate). */
export function matchesEngineTagPattern(
  engineId: DatabaseEngineId,
  tag: string,
): boolean {
  return tagAllowed(engineId, tag);
}

/** Test helper — clear TTL cache. */
export function clearDockerHubTagCache(): void {
  cache.clear();
}
