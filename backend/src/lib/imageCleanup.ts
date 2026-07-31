// Docklift-scoped image retention (keep-2) + BuildKit prune helpers.
// Never host-wide system prune; never delete non-docklift-* images.
import { spawn } from 'child_process';
import Docker from 'dockerode';
import { dockerSlug } from './naming.js';

const docker = new Docker();

export type ImageTagsMap = Record<string, string>;

export function projectId8(projectId: string): string {
  return projectId.slice(0, 8);
}

export function dockliftImageRepo(projectId: string, serviceName: string): string {
  return `docklift-${projectId8(projectId)}-${dockerSlug(serviceName)}`;
}

export function dockliftImageTag(
  projectId: string,
  serviceName: string,
  deploymentId: string,
): string {
  return `${dockliftImageRepo(projectId, serviceName)}:${deploymentId.slice(0, 8)}`;
}

/** True if ref looks like a Docklift-built app image (not upstream postgres/nginx/etc.). */
export function isDockliftAppImage(ref: string): boolean {
  const name = ref.includes(':') ? ref.slice(0, ref.indexOf(':')) : ref;
  return /^docklift-[a-f0-9]{8}-[a-z0-9][a-z0-9_-]*$/i.test(name);
}

/**
 * Keep-set from the last N successful deployments' image_tags maps.
 * Also accepts a fallback map for the current deploy if not yet persisted.
 */
export function buildKeepImageSet(
  successImageTagMaps: Array<ImageTagsMap | null | undefined>,
  fallbackCurrent?: ImageTagsMap | null,
): Set<string> {
  const keep = new Set<string>();
  for (const map of successImageTagMaps) {
    if (!map || typeof map !== 'object') continue;
    for (const tag of Object.values(map)) {
      if (typeof tag === 'string' && tag && isDockliftAppImage(tag)) {
        keep.add(tag);
      }
    }
  }
  if (fallbackCurrent) {
    for (const tag of Object.values(fallbackCurrent)) {
      if (typeof tag === 'string' && tag && isDockliftAppImage(tag)) {
        keep.add(tag);
      }
    }
  }
  return keep;
}

export function parseImageTagsJson(value: unknown): ImageTagsMap | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const out: ImageTagsMap = {};
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    if (typeof v === 'string' && v) out[k] = v;
  }
  return Object.keys(out).length ? out : null;
}

/** Pure eligibility checks for Restore previous (step-up / image existence checked separately). */
export function rollbackTargetGuard(opts: {
  targetId: string;
  latestSuccessId: string | null | undefined;
  status: string;
  imageTags: ImageTagsMap | null;
}): { ok: true } | { ok: false; status: number; error: string } {
  if (opts.status !== 'success') {
    return { ok: false, status: 404, error: 'Target successful deployment not found' };
  }
  if (opts.latestSuccessId && opts.latestSuccessId === opts.targetId) {
    return {
      ok: false,
      status: 400,
      error: 'That deployment is already the current success',
    };
  }
  if (!opts.imageTags || Object.keys(opts.imageTags).length === 0) {
    return {
      ok: false,
      status: 409,
      error: 'This deployment has no stored image tags (pre-keep-2). Redeploy from git instead.',
    };
  }
  return { ok: true };
}

function runDocker(args: string[]): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const child = spawn('docker', args, { shell: false });
    let stdout = '';
    let stderr = '';
    child.stdout?.on('data', (d) => {
      stdout += d.toString();
    });
    child.stderr?.on('data', (d) => {
      stderr += d.toString();
    });
    child.on('error', (err) => {
      resolve({ code: 1, stdout, stderr: err.message });
    });
    child.on('close', (code) => {
      resolve({ code: code ?? 1, stdout, stderr });
    });
  });
}

export type ListedDockliftImage = {
  id: string;
  /** Repo tags like docklift-abc12345-app:deadbeef */
  tags: string[];
};

/** List local images whose repo matches docklift-<project8>-* (or all docklift-* if projectId omitted). */
export async function listDockliftImages(projectId?: string): Promise<ListedDockliftImage[]> {
  const images = await docker.listImages({ all: true });
  const prefix = projectId
    ? `docklift-${projectId8(projectId)}-`
    : 'docklift-';

  const out: ListedDockliftImage[] = [];
  for (const img of images) {
    const repoTags = (img.RepoTags || []).filter(
      (t) => t && t !== '<none>:<none>' && t.startsWith(prefix) && isDockliftAppImage(t),
    );
    if (repoTags.length === 0) continue;
    out.push({ id: img.Id, tags: repoTags });
  }
  return out;
}

export async function imageExistsLocally(tag: string): Promise<boolean> {
  try {
    await docker.getImage(tag).inspect();
    return true;
  } catch {
    return false;
  }
}

/** Force-remove an image by tag or id. Ignores not-found. */
export async function removeImage(tagOrId: string): Promise<boolean> {
  try {
    await docker.getImage(tagOrId).remove({ force: true });
    return true;
  } catch (err: unknown) {
    const msg = String((err as { message?: string })?.message || err || '');
    if (/no such image|not found/i.test(msg)) return false;
    throw err instanceof Error ? err : new Error(msg);
  }
}

/** Image refs currently used by a running/created container name. */
export async function getContainerImageRef(containerName: string): Promise<string | null> {
  if (!containerName?.trim()) return null;
  try {
    const info = await docker.getContainer(containerName).inspect();
    return info.Config?.Image || info.Image || null;
  } catch {
    return null;
  }
}

export async function pruneBuildKitUnused(): Promise<{ ok: boolean; output: string }> {
  const res = await runDocker(['builder', 'prune', '-f']);
  return {
    ok: res.code === 0,
    output: (res.stdout || res.stderr || '').trim(),
  };
}

export async function pruneBuildKitAll(): Promise<{ ok: boolean; output: string }> {
  const res = await runDocker(['builder', 'prune', '-af']);
  return {
    ok: res.code === 0,
    output: (res.stdout || res.stderr || '').trim(),
  };
}

export type CleanupResult = {
  kept: string[];
  removed: string[];
  skippedInUse: string[];
  errors: string[];
  buildKit?: { ok: boolean; mode: 'unused' | 'all'; output: string };
};

/**
 * Remove docklift images for a project (or all docklift-* if projectId omitted)
 * that are not in keepSet and not referenced by inUseRefs.
 */
/**
 * Reconstruct expected tags for an in-progress deploy (image_tags may not be
 * persisted until compose succeeds).
 */
export function reconstructInProgressTags(
  projectId: string,
  deploymentId: string,
  serviceNames: string[],
): ImageTagsMap {
  const out: ImageTagsMap = {};
  for (const name of serviceNames) {
    if (!name) continue;
    out[name] = dockliftImageTag(projectId, name, deploymentId);
  }
  return out;
}

export async function removeUnusedDockliftImages(opts: {
  projectId?: string;
  keepSet: Set<string>;
  inUseRefs: Set<string>;
}): Promise<CleanupResult> {
  const kept = [...opts.keepSet];
  const removed: string[] = [];
  const skippedInUse: string[] = [];
  const errors: string[] = [];

  const images = await listDockliftImages(opts.projectId);
  for (const img of images) {
    for (const tag of img.tags) {
      if (opts.keepSet.has(tag)) continue;
      const idBare = img.id.replace(/^sha256:/, '');
      const inUse =
        opts.inUseRefs.has(tag) ||
        opts.inUseRefs.has(img.id) ||
        opts.inUseRefs.has(idBare) ||
        [...opts.inUseRefs].some(
          (r) =>
            r === tag ||
            r.replace(/^sha256:/, '') === idBare ||
            (r.startsWith('sha256:') && img.id.startsWith(r)),
        );
      if (inUse) {
        skippedInUse.push(tag);
        continue;
      }
      try {
        const did = await removeImage(tag);
        if (did) removed.push(tag);
      } catch (err: unknown) {
        errors.push(`${tag}: ${(err as Error)?.message || err}`);
      }
    }
  }

  return { kept, removed, skippedInUse, errors };
}
