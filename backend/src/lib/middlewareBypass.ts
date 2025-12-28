// Middleware bypass utility - patches Next.js middleware to allow localhost/IP access
import fs from 'fs';
import path from 'path';
import os from 'os';

/**
 * Scans for Next.js middleware files (middleware.ts or middleware.js)
 * and patches the allowedHosts array to include additional hosts.
 */

// Regex to detect allowedHosts pattern:
// const allowedHosts = ["domain.com", "www.domain.com"];
const ALLOWED_HOSTS_REGEX = /const\s+allowedHosts\s*=\s*\[([\s\S]*?)\]/;

interface MiddlewareBypassOptions {
  projectPath: string;
  port: number;
  domains: string[];
  serverIP?: string;
}

interface MiddlewareBypassResult {
  found: boolean;
  patched: boolean;
  filePath?: string;
  originalHosts?: string[];
  addedHosts?: string[];
  error?: string;
}

/**
 * Get the server's external/local IP address
 */
function getServerIP(): string {
  try {
    const interfaces = os.networkInterfaces();
    
    for (const name of Object.keys(interfaces)) {
      for (const iface of interfaces[name] || []) {
        // Skip internal (i.e., 127.0.0.1) and non-IPv4 addresses
        if (iface.family === 'IPv4' && !iface.internal) {
          return iface.address;
        }
      }
    }
  } catch (e) {
    console.error('Failed to get server IP:', e);
  }
  
  return 'localhost';
}

/**
 * Recursively search for middleware files in project directories
 */
function findMiddlewareFiles(dir: string, depth: number = 0, maxDepth: number = 3): string[] {
  const results: string[] = [];
  
  if (depth > maxDepth) return results;
  
  try {
    const entries = fs.readdirSync(dir, { withFileTypes: true });
    
    for (const entry of entries) {
      const fullPath = path.join(dir, entry.name);
      
      // Skip node_modules, .git, and other common directories
      if (entry.isDirectory()) {
        const skipDirs = ['node_modules', '.git', '.next', 'dist', 'build', '.cache'];
        if (!skipDirs.includes(entry.name)) {
          results.push(...findMiddlewareFiles(fullPath, depth + 1, maxDepth));
        }
      } else if (entry.name === 'middleware.ts' || entry.name === 'middleware.js') {
        results.push(fullPath);
      }
    }
  } catch (e) {
    // Ignore permission errors
  }
  
  return results;
}

/**
 * Parse existing hosts from the allowedHosts array
 */
function parseExistingHosts(hostsContent: string): string[] {
  // Extract quoted strings from the array content
  const hostMatches = hostsContent.match(/["']([^"']+)["']/g);
  
  if (!hostMatches) return [];
  
  return hostMatches.map(h => h.replace(/["']/g, ''));
}

/**
 * Patch middleware file to add additional allowed hosts
 */
export async function patchMiddlewareHosts(options: MiddlewareBypassOptions): Promise<MiddlewareBypassResult> {
  const { projectPath, port, domains } = options;
  const serverIP = options.serverIP || getServerIP();
  
  // Find middleware files
  const middlewareFiles = findMiddlewareFiles(projectPath);
  
  if (middlewareFiles.length === 0) {
    return { found: false, patched: false };
  }
  
  // Process each middleware file
  for (const filePath of middlewareFiles) {
    try {
      const content = fs.readFileSync(filePath, 'utf-8');
      
      // Check if file has allowedHosts pattern
      const match = content.match(ALLOWED_HOSTS_REGEX);
      
      if (!match) {
        continue; // This middleware doesn't have host restrictions
      }
      
      const originalHosts = parseExistingHosts(match[1]);
      
      // Build list of hosts to add
      const hostsToAdd: string[] = [
        'localhost',
        `localhost:${port}`,
        serverIP,
        `${serverIP}:${port}`,
      ];
      
      // Add configured domains
      for (const domain of domains) {
        if (domain && domain.trim()) {
          hostsToAdd.push(domain.trim());
          // Also add www variant if not a subdomain
          if (!domain.includes('.') || domain.split('.').length === 2) {
            if (!domain.startsWith('www.')) {
              hostsToAdd.push(`www.${domain.trim()}`);
            }
          }
        }
      }
      
      // Filter out hosts that already exist
      const newHosts = hostsToAdd.filter(h => !originalHosts.includes(h));
      
      if (newHosts.length === 0) {
        return {
          found: true,
          patched: false,
          filePath,
          originalHosts,
          addedHosts: [],
        };
      }
      
      // Create backup
      const backupPath = `${filePath}.bak`;
      fs.writeFileSync(backupPath, content, 'utf-8');
      
      // Build new allowedHosts array content
      const allHosts = [...originalHosts, ...newHosts];
      const newHostsString = allHosts.map(h => `"${h}"`).join(', ');
      const newArrayContent = `const allowedHosts = [${newHostsString}]`;
      
      // Replace in content
      const newContent = content.replace(ALLOWED_HOSTS_REGEX, newArrayContent);
      
      // Write patched file
      fs.writeFileSync(filePath, newContent, 'utf-8');
      
      return {
        found: true,
        patched: true,
        filePath,
        originalHosts,
        addedHosts: newHosts,
      };
      
    } catch (e: any) {
      return {
        found: true,
        patched: false,
        filePath,
        error: e.message,
      };
    }
  }
  
  return { found: false, patched: false };
}

/**
 * Log middleware bypass results to the deployment stream
 */
export function logMiddlewareBypassResult(
  result: MiddlewareBypassResult,
  writeLog: (msg: string) => void
): void {
  if (!result.found) {
    return; // No middleware found, nothing to log
  }
  
  writeLog(`\n🔐 Middleware Host Check Detected\n`);
  writeLog(`   File: ${result.filePath}\n`);
  
  if (result.error) {
    writeLog(`   ⚠️  Error patching: ${result.error}\n`);
    return;
  }
  
  if (!result.patched) {
    writeLog(`   ✓ Already has required hosts\n`);
    return;
  }
  
  writeLog(`   📋 Original hosts: ${result.originalHosts?.join(', ')}\n`);
  writeLog(`   ➕ Added hosts: ${result.addedHosts?.join(', ')}\n`);
  writeLog(`   ✅ Middleware patched for local access\n`);
  writeLog(`   💾 Backup created: ${result.filePath}.bak\n`);
}
