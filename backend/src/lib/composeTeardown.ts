import { spawnSync, type SpawnSyncReturns } from 'child_process';

export type ComposeResourceProbe = (projectName: string) => {
  containerIds: string[];
  networkIds: string[];
};

/** Exact-label probe: only Compose resources owned by this project name. */
export function probeComposeProjectResources(projectName: string): {
  containerIds: string[];
  networkIds: string[];
} {
  const containers = spawnSync(
    'docker',
    ['ps', '-aq', '--filter', `label=com.docker.compose.project=${projectName}`],
    { encoding: 'utf8', shell: false, timeout: 30000 }
  );
  const networks = spawnSync(
    'docker',
    ['network', 'ls', '-q', '--filter', `label=com.docker.compose.project=${projectName}`],
    { encoding: 'utf8', shell: false, timeout: 30000 }
  );

  const splitIds = (out: string | null | undefined) =>
    String(out || '')
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean);

  return {
    containerIds: containers.status === 0 ? splitIds(containers.stdout) : ['__probe_failed__'],
    networkIds: networks.status === 0 ? splitIds(networks.stdout) : ['__probe_failed__'],
  };
}

/** True when no containers/networks remain for this exact Compose project label. */
export function verifyComposeProjectGone(
  projectName: string,
  probe: ComposeResourceProbe = probeComposeProjectResources
): boolean {
  const { containerIds, networkIds } = probe(projectName);
  return containerIds.length === 0 && networkIds.length === 0;
}

/**
 * Teardown is OK when compose down exits 0, OR (nonzero) when an exact-label
 * postcondition proves no owned containers/networks remain.
 *
 * Never infer absence from stderr text like "not found" — that matches unrelated failures.
 */
export function isComposeTeardownOk(
  result: SpawnSyncReturns<string | Buffer>,
  projectName: string,
  probe: ComposeResourceProbe = probeComposeProjectResources
): boolean {
  if (result.error) return false;
  if (result.status === 0) {
    // Still verify — exit 0 can race with leftover labeled resources
    return verifyComposeProjectGone(projectName, probe);
  }
  return verifyComposeProjectGone(projectName, probe);
}

/** After a failed stop/cancel teardown, proxy must be reattached so domains keep working. */
export function shouldReconnectProxyAfterFailedTeardown(teardownSucceeded: boolean): boolean {
  return !teardownSucceeded;
}
