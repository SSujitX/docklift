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

// Pull latest from repository
export async function pullRepo(projectPath: string, res: Response): Promise<void> {
  const git = simpleGit(projectPath);
  
  res.write(`📥 Pulling latest changes...\n`);
  
  try {
    const result = await git.pull();
    
    if (result.summary.changes > 0) {
      res.write(`   ✅ ${result.summary.changes} file(s) changed\n`);
      res.write(`   ↓ ${result.summary.insertions} insertions\n`);
      res.write(`   ✗ ${result.summary.deletions} deletions\n`);
    } else {
      res.write(`   Already up to date\n`);
    }
    res.write(`\n`);
  } catch (error: any) {
    const errorMsg = error?.message || String(error);
    
    // Check if it's a merge conflict / local changes issue
    if (errorMsg.includes('overwritten by merge') || 
        errorMsg.includes('local changes') || 
        errorMsg.includes('Please commit your changes') ||
        errorMsg.includes('Aborting')) {
      res.write(`   ⚠️ Local changes detected, resetting to remote...\n`);
      
      try {
        // Get current branch
        const branch = await git.revparse(['--abbrev-ref', 'HEAD']);
        const cleanBranch = branch.trim();
        
        // Reset hard to discard local changes
        await git.reset(['--hard', 'HEAD']);
        res.write(`   🔄 Reset local changes\n`);
        
        // Pull again
        const retryResult = await git.pull('origin', cleanBranch);
        
        if (retryResult.summary.changes > 0) {
          res.write(`   ✅ ${retryResult.summary.changes} file(s) changed\n`);
          res.write(`   ↓ ${retryResult.summary.insertions} insertions\n`);
          res.write(`   ✗ ${retryResult.summary.deletions} deletions\n`);
        } else {
          res.write(`   Already up to date\n`);
        }
        res.write(`\n`);
        return;
      } catch (resetError: any) {
        res.write(`   ❌ Reset failed: ${resetError?.message || resetError}\n\n`);
      }
    } else {
      res.write(`   ⚠️ Git pull warning: ${error}\n\n`);
    }
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
