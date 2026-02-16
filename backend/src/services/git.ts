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
    if (branch) res.write(`   Branch: ${branch}\n`);
  }
  
  // Remove existing directory if it exists
  if (fs.existsSync(targetPath)) {
    fs.rmSync(targetPath, { recursive: true, force: true });
  }
  
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
    // Determine which branch to sync to
    let targetBranch = branch;
    if (!targetBranch) {
      try {
        targetBranch = (await git.revparse(['--abbrev-ref', 'HEAD'])).trim();
      } catch {
        targetBranch = 'main';
      }
    }
    res.write(`   Branch: ${targetBranch}\n`);
    
    // Get current commit before fetch for comparison
    const beforeCommit = (await git.revparse(['HEAD'])).trim().substring(0, 7);
    
    // Step 1: Fetch latest refs from remote (never causes conflicts)
    await git.fetch('origin', targetBranch);
    res.write(`   ✅ Fetched from origin\n`);
    
    // Step 2: Hard reset to exact remote state (guarantees fresh code)
    await git.reset(['--hard', `origin/${targetBranch}`]);
    res.write(`   ✅ Reset to origin/${targetBranch}\n`);
    
    // Step 3: Clean untracked files and directories (removes stale artifacts)
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
