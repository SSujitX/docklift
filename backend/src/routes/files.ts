// Files routes - API endpoints for project file browsing and editing
import { Router, Request, Response } from 'express';
import fs from 'fs';
import path from 'path';
import { config } from '../lib/config.js';

const router = Router();

// Validate project ID to prevent path traversal (only allow UUIDs and alphanumeric-dash strings)
const VALID_PROJECT_ID = /^[a-zA-Z0-9_-]+$/;

// List files in project
router.get('/:projectId', (req: Request, res: Response) => {
  try {
    if (!VALID_PROJECT_ID.test(req.params.projectId)) {
      return res.status(400).json({ error: 'Invalid project ID' });
    }
    const projectPath = path.join(config.deploymentsPath, req.params.projectId);
    
    if (!fs.existsSync(projectPath)) {
      return res.status(404).json({ error: 'Project files not found' });
    }
    
    const files = listFilesRecursive(projectPath, projectPath);
    res.json(files);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list files' });
  }
});

// Get file content - use query param ?path=...
router.get('/:projectId/content', (req: Request, res: Response) => {
  try {
    if (!VALID_PROJECT_ID.test(req.params.projectId)) {
      return res.status(400).json({ error: 'Invalid project ID' });
    }
    const projectPath = path.join(config.deploymentsPath, req.params.projectId);
    const relativePath = req.query.path as string || '';
    const filePath = path.resolve(projectPath, relativePath);

    // Security check - prevent path traversal using resolved absolute paths
    const resolvedProject = path.resolve(projectPath);
    if (!filePath.startsWith(resolvedProject)) {
      return res.status(403).json({ error: 'Access denied' });
    }

    if (!fs.existsSync(filePath)) {
      return res.status(404).json({ error: 'File not found' });
    }

    // Security check - resolve symlinks and verify real path is within project
    const realProjectPath = fs.realpathSync(projectPath);
    const realFilePath = fs.realpathSync(filePath);
    if (!realFilePath.startsWith(realProjectPath)) {
      return res.status(403).json({ error: 'Access denied - symlink outside project' });
    }

    const stat = fs.statSync(filePath);
    if (stat.isDirectory()) {
      return res.status(400).json({ error: 'Path is a directory' });
    }

    const content = fs.readFileSync(filePath, 'utf-8');
    res.json({ name: path.basename(filePath), content });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to read file' });
  }
});

// Update file content - use query param ?path=...
router.put('/:projectId/content', (req: Request, res: Response) => {
  try {
    if (!VALID_PROJECT_ID.test(req.params.projectId)) {
      return res.status(400).json({ error: 'Invalid project ID' });
    }
    const projectPath = path.join(config.deploymentsPath, req.params.projectId);
    const relativePath = req.query.path as string || '';
    const filePath = path.resolve(projectPath, relativePath);

    // Security check - prevent path traversal using resolved absolute paths
    const resolvedProject = path.resolve(projectPath);
    if (!filePath.startsWith(resolvedProject)) {
      return res.status(403).json({ error: 'Access denied' });
    }

    if (!fs.existsSync(filePath)) {
      return res.status(404).json({ error: 'File not found' });
    }

    // Security check - resolve symlinks and verify real path is within project
    const realProjectPath = fs.realpathSync(projectPath);
    const realFilePath = fs.realpathSync(filePath);
    if (!realFilePath.startsWith(realProjectPath)) {
      return res.status(403).json({ error: 'Access denied - symlink outside project' });
    }

    const { content } = req.body;
    fs.writeFileSync(filePath, content, 'utf-8');

    res.json({ status: 'updated' });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update file' });
  }
});

// Helper function to list files recursively
function listFilesRecursive(basePath: string, currentPath: string, depth = 0): Array<{
  name: string;
  path: string;
  type: 'file' | 'folder';
  size?: number;
  editable?: boolean;
  children?: Array<unknown>;
}> {
  if (depth > 10) return []; // Limit depth
  
  const items = fs.readdirSync(currentPath);
  const files: Array<{
    name: string;
    path: string;
    type: 'file' | 'folder';
    size?: number;
    editable?: boolean;
    children?: Array<unknown>;
  }> = [];
  
  // Skip certain directories
  const skipDirs = new Set(['node_modules', '.git', '.next', 'dist', '__pycache__', '.venv', 'venv']);
  
  // Files are editable by default unless they are known binaries
  const binaryExtensions = new Set([
    'png', 'jpg', 'jpeg', 'gif', 'bmp', 'ico', 'svg', 'webp',
    'zip', 'tar', 'gz', '7z', 'rar',
    'exe', 'dll', 'so', 'dylib', 'bin',
    'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx',
    'mp3', 'mp4', 'wav', 'avi', 'mov', 'webm',
    'sqlite', 'db'
  ]);

  for (const item of items) {
    if (skipDirs.has(item)) continue;
    
    const fullPath = path.join(currentPath, item);
    const relativePath = path.relative(basePath, fullPath);
    
    // Skip symlinks to prevent traversal attacks
    const lstat = fs.lstatSync(fullPath);
    if (lstat.isSymbolicLink()) continue;
    
    if (lstat.isDirectory()) {
      files.push({
        name: item,
        path: relativePath,
        type: 'folder',
        children: listFilesRecursive(basePath, fullPath, depth + 1),
      });
    } else {
      const ext = item.split('.').pop()?.toLowerCase() || '';
      // Allow editing if NOT in binary list
      const isEditable = !binaryExtensions.has(ext);

      files.push({
        name: item,
        path: relativePath,
        type: 'file',
        size: lstat.size,
        editable: isEditable,
      });
    }
  }
  
  // Sort: directories first, then by name
  files.sort((a, b) => {
    if (a.type !== b.type) {
      return a.type === 'folder' ? -1 : 1;
    }
    return a.name.localeCompare(b.name);
  });
  
  return files;
}

export default router;
