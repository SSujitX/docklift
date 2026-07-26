// Git service - repository cloning and pulling operations
import { simpleGit, SimpleGit } from 'simple-git';
import fs from 'fs';
import path from 'path';
import { Response } from 'express';

// Clone a repository
export async function cloneRepo(url: string, targetPath: string, branch?: string, res?: Response): Promise<void> {
  const git = simpleGit();
  
  if (res) {
    res.write(`📥 Cloning repository...\n`);
    res.write(`   URL: ${url}\n`);
    if (branch) res.write(`   Ref: ${branch}\n`);
  }
  
  // Remove existing directory if it exists
  if (fs.existsSync(targetPath)) {
    fs.rmSync(targetPath, { recursive: true, force: true });
  }
  
  // `-b` works for branch names and tags
  const options = branch ? ['-b', branch] : [];
  await git.clone(url, targetPath, options);
  
  if (res) {
    res.write(`✅ Repository cloned successfully\n\n`);
  }
}

// Pull latest from repository using fetch + reset (professional approach)
// This guarantees the local copy exactly matches the remote, unlike git pull
// which can fail silently on merge conflicts or diverged histories.
export async function pullRepo(projectPath: string, res: Response, branch?: string): Promise<void> {
  const git = simpleGit(projectPath);
  
  res.write(`📥 Fetching latest changes...\n`);
  
  try {
    // Determine which branch/tag to sync to
    let targetRef = branch;
    if (!targetRef) {
      try {
        targetRef = (await git.revparse(['--abbrev-ref', 'HEAD'])).trim();
        if (targetRef === 'HEAD') {
          targetRef = (await git.revparse(['HEAD'])).trim();
        }
      } catch {
        targetRef = 'main';
      }
    }
    res.write(`   Ref: ${targetRef}\n`);
    
    // Get current commit before fetch for comparison
    const beforeCommit = (await git.revparse(['HEAD'])).trim().substring(0, 7);
    
    // Fetch branches + tags; prune-tags drops local tags deleted on the remote
    await git.fetch(['origin', '--tags', '--prune', '--prune-tags', '--force']);
    res.write(`   ✅ Fetched from origin (branches + tags)\n`);

    const remoteBranches = await git.branch(['-r']);
    const remoteBranch = `origin/${targetRef}`;
    const isBranch = remoteBranches.all.includes(remoteBranch);

    if (isBranch) {
      await git.reset(['--hard', remoteBranch]);
      res.write(`   ✅ Reset to ${remoteBranch}\n`);
    } else {
      // Fail closed: require the tag to exist on the remote (not only a stale local tag)
      const tagRef = `refs/tags/${targetRef}`;
      const remoteTag = (
        await git.raw(['ls-remote', '--tags', '--refs', 'origin', tagRef])
      ).trim();
      if (!remoteTag) {
        throw new Error(
          `Ref "${targetRef}" not found as remote branch (${remoteBranch}) or tag (${tagRef})`,
        );
      }
      // Refresh the local tag from the ls-remote result, then reset
      try {
        await git.raw(['fetch', 'origin', `+${tagRef}:${tagRef}`, '--force']);
      } catch {
        /* fetch --tags above usually suffices; local verify next */
      }
      try {
        await git.raw(['rev-parse', '--verify', `${tagRef}^{commit}`]);
      } catch {
        throw new Error(
          `Ref "${targetRef}" exists on remote but could not be resolved locally`,
        );
      }
      await git.reset(['--hard', tagRef]);
      res.write(`   ✅ Reset to tag ${targetRef}\n`);
    }
    
    // Clean untracked files and directories (removes stale artifacts)
    await git.clean('f', ['-d']);
    res.write(`   ✅ Cleaned untracked files\n`);
    
    // Get new commit for comparison
    const afterCommit = (await git.revparse(['HEAD'])).trim().substring(0, 7);
    
    if (beforeCommit !== afterCommit) {
      res.write(`   📝 Updated: ${beforeCommit} → ${afterCommit}\n`);
    } else {
      res.write(`   Already up to date (${afterCommit})\n`);
    }
    res.write(`\n`);
  } catch (error: any) {
    const errorMsg = error?.message || String(error);
    res.write(`   ❌ Git sync failed: ${errorMsg}\n\n`);
    // CRITICAL: Throw so deploy aborts instead of continuing with old code
    throw new Error(`Git sync failed: ${errorMsg}`);
  }
}

// Get current branch
export async function getCurrentBranch(projectPath: string): Promise<string | null> {
  try {
    const git = simpleGit(projectPath);
    return await git.revparse(['--abbrev-ref', 'HEAD']);
  } catch {
    return null;
  }
}

// Get last commit message
export async function getLastCommitMessage(projectPath: string): Promise<string | null> {
  try {
    if (!fs.existsSync(path.join(projectPath, '.git'))) return null;
    const git = simpleGit(projectPath);
    const log = await git.log({ maxCount: 1 });
    return log.latest?.message || null;
  } catch {
    return null;
  }
}

// SECURITY: Remove credentials from origin remote after clone/pull
export async function scrubOriginRemote(projectPath: string, cleanUrl: string): Promise<void> {
  if (!cleanUrl || !fs.existsSync(path.join(projectPath, '.git'))) return;
  const git = simpleGit(projectPath);
  await git.remote(['set-url', 'origin', cleanUrl]);
}
