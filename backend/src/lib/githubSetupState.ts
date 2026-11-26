import path from 'path';

/**
 * Setup states are generated as hex (crypto.randomBytes(24)). The value comes
 * back from a public query string / cookie, so it must be a plain token before
 * it is ever used to build a file path.
 */
const SETUP_STATE_TOKEN = /^[A-Za-z0-9_-]{1,128}$/;

export function isSafeSetupStateToken(state: unknown): state is string {
  return typeof state === 'string' && SETUP_STATE_TOKEN.test(state);
}

/** Path of the state file inside `dir`, or null when `state` is not a plain token. */
export function setupStateFilePath(dir: string, state: unknown): string | null {
  if (!isSafeSetupStateToken(state)) return null;
  return path.join(dir, `${state}.json`);
}
