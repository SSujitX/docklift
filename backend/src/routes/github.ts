import { Router, Request, Response } from 'express';
import fs from 'fs';
import path from 'path';
import jwt from 'jsonwebtoken';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';

const router = Router();
const GITHUB_API_URL = 'https://api.github.com';

// Helper: Get setting from database
async function getSetting(key: string): Promise<string | null> {
  const setting = await prisma.settings.findUnique({ where: { key } });
  return setting?.value || null;
}

// Helper: Save setting to database
async function saveSetting(key: string, value: string): Promise<void> {
  await prisma.settings.upsert({
    where: { key },
    update: { value },
    create: { key, value },
  });
}

// Helper: Delete setting from database
async function deleteSetting(key: string): Promise<void> {
  await prisma.settings.deleteMany({ where: { key } });
}

// Helper: Get GitHub App private key from database (or fallback to file)
async function getPrivateKey(): Promise<string | null> {
  // First try database (from manifest flow)
  const dbKey = await getSetting('github_private_key');
  if (dbKey) {
    return dbKey;
  }
  
  // Fallback to file (legacy)
  const keyPath = path.resolve(config.githubPrivateKeyPath);
  if (fs.existsSync(keyPath)) {
    return fs.readFileSync(keyPath, 'utf-8');
  }
  return null;
}

// Helper: Get GitHub App ID from database (or fallback to env)
async function getAppId(): Promise<string | null> {
  const dbAppId = await getSetting('github_app_id');
  if (dbAppId) {
    return dbAppId;
  }
  return config.githubAppId || null;
}

// Helper: Create JWT for GitHub App auth
async function createJwtToken(): Promise<string> {
  const privateKey = await getPrivateKey();
  if (!privateKey) {
    throw new Error('GitHub App private key not found. Please create a GitHub App first.');
  }
  
  const appId = await getAppId();
  if (!appId) {
    throw new Error('GitHub App ID not found. Please create a GitHub App first.');
  }
  
  const payload = {
    iat: Math.floor(Date.now() / 1000) - 60,
    exp: Math.floor(Date.now() / 1000) + 600,
    iss: parseInt(appId, 10),
  };
  
  return jwt.sign(payload, privateKey, { algorithm: 'RS256' });
}

// Helper: Get installation access token
async function getInstallationToken(installationId: string): Promise<string> {
  const jwtToken = await createJwtToken();
  
  const response = await fetch(
    `${GITHUB_API_URL}/app/installations/${installationId}/access_tokens`,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${jwtToken}`,
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
      },
    }
  );
  
  if (!response.ok) {
    throw new Error(`Failed to get installation token: ${await response.text()}`);
  }
  
  const data = await response.json() as { token: string };
  return data.token;
}

// ========================================
// GitHub App Manifest Flow
// Allows one-click GitHub App creation
// ========================================

// POST /manifest - Generate manifest and return HTML form for GitHub redirect
router.post('/manifest', async (req: Request, res: Response) => {
  try {
    const { appName, returnUrl } = req.body;
    
    if (!appName || typeof appName !== 'string') {
      return res.status(400).json({ error: 'appName is required' });
    }

    if (returnUrl) {
      await saveSetting('github_return_url', returnUrl);
    }
    
    // Sanitize app name
    const sanitizedName = appName.toLowerCase().replace(/[^a-z0-9-]/g, '-').slice(0, 34);
    
    // Get server URL (from request or config)
    const protocol = req.headers['x-forwarded-proto'] || 'http';
    const host = req.headers['x-forwarded-host'] || req.headers.host || 'localhost:8000';
    const serverUrl = `${protocol}://${host}`;
    
    // Build the manifest
    // Note: hook_attributes removed - webhooks can be configured later in production
    const manifest = {
      name: `docklift-${sanitizedName}`,
      url: 'https://github.com/SSujitX/docklift',
      redirect_url: `${serverUrl}/api/github/manifest/callback`,
      callback_urls: [`${serverUrl}/api/github/manifest/callback`],
      // setup_url is called after app installation - redirects user back to Docklift
      setup_url: `${serverUrl}/api/github/setup`,
      public: false,
      default_permissions: {
        contents: 'read',
        metadata: 'read'
      }
    };
    
    // Store the app name for later
    await saveSetting('github_pending_app_name', sanitizedName);
    
    // Return data for frontend to create form and submit
    res.json({
      manifest: JSON.stringify(manifest),
      action: 'https://github.com/settings/apps/new',
      serverUrl
    });
  } catch (error) {
    console.error('Manifest generation error:', error);
    res.status(500).json({ error: 'Failed to generate manifest' });
  }
});

// GET /manifest/callback - Handle GitHub's response after app creation
router.get('/manifest/callback', async (req: Request, res: Response) => {
  try {
    const code = req.query.code as string;
    
    if (!code) {
      // This might be a setup callback without code
      const installationId = req.query.installation_id as string;
      if (installationId) {
        return res.redirect(`/api/github/setup?installation_id=${installationId}`);
      }
      return res.redirect(`${config.frontendUrl}?github_error=no_code`);
    }
    
    // Exchange code for app credentials
    const response = await fetch(`https://api.github.com/app-manifests/${code}/conversions`, {
      method: 'POST',
      headers: {
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
      }
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      console.error('GitHub manifest conversion failed:', errorText);
      return res.redirect(`${config.frontendUrl}?github_error=conversion_failed`);
    }
    
    const data = await response.json() as {
      id: number;
      slug: string;
      name: string;
      client_id: string;
      client_secret: string;
      pem: string;
      webhook_secret: string;
      owner: { login: string; avatar_url: string };
    };
    
    // Store all credentials in settings
    await saveSetting('github_app_id', String(data.id));
    await saveSetting('github_app_slug', data.slug);
    await saveSetting('github_app_name', data.name);
    await saveSetting('github_client_id', data.client_id);
    await saveSetting('github_client_secret', data.client_secret);
    await saveSetting('github_private_key', data.pem);
    await saveSetting('github_webhook_secret', data.webhook_secret || '');
    await saveSetting('github_username', data.owner?.login || '');
    await saveSetting('github_avatar_url', data.owner?.avatar_url || '');
    
    console.log(`GitHub App created successfully: ${data.name} (ID: ${data.id})`);
    
    // Redirect to install the app
    res.redirect(`https://github.com/apps/${data.slug}/installations/new`);
  } catch (error) {
    console.error('Manifest callback error:', error);
    res.redirect(`${config.frontendUrl}?github_error=callback_failed`);
  }
});

// GET /app-status - Check if GitHub App is configured via manifest
router.get('/app-status', async (req: Request, res: Response) => {
  try {
    const appId = await getSetting('github_app_id');
    const appName = await getSetting('github_app_name');
    const appSlug = await getSetting('github_app_slug');
    const installationId = await getSetting('github_installation_id');
    const username = await getSetting('github_username');
    const avatarUrl = await getSetting('github_avatar_url');
    
    res.json({
      configured: !!appId,
      installed: !!installationId,
      appId,
      appName,
      appSlug,
      installationId,
      username,
      avatarUrl,
      installUrl: appSlug ? `https://github.com/apps/${appSlug}/installations/new` : null
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get app status' });
  }
});

// POST /check-installation - Manually check for installations (for localhost)
router.post('/check-installation', async (req: Request, res: Response) => {
  try {
    const appId = await getSetting('github_app_id');
    if (!appId) {
      return res.status(400).json({ error: 'GitHub App not configured' });
    }
    
    // Get JWT token
    const jwtToken = await createJwtToken();
    
    // Fetch all installations for this app
    const response = await fetch(`${GITHUB_API_URL}/app/installations`, {
      headers: {
        Authorization: `Bearer ${jwtToken}`,
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
      },
    });
    
    if (!response.ok) {
      throw new Error(`Failed to fetch installations: ${await response.text()}`);
    }
    
    const installations = await response.json() as Array<{
      id: number;
      account?: { login?: string; avatar_url?: string };
    }>;
    
    if (installations.length === 0) {
      return res.json({ found: false, message: 'No installations found. Please install the app on GitHub first.' });
    }
    
    // Use the first installation (user's account)
    const installation = installations[0];
    await saveSetting('github_installation_id', installation.id.toString());
    await saveSetting('github_username', installation.account?.login || 'unknown');
    await saveSetting('github_avatar_url', installation.account?.avatar_url || '');
    
    console.log(`Found GitHub App installation: ${installation.id} for ${installation.account?.login}`);
    
    res.json({ 
      found: true, 
      installationId: installation.id,
      username: installation.account?.login 
    });
  } catch (error) {
    console.error('Check installation error:', error);
    res.status(500).json({ error: 'Failed to check installation' });
  }
});

// ========================================
// Existing GitHub App Installation Flow
// ========================================

// GET /install - Redirect to GitHub App installation page
router.get('/install', async (req: Request, res: Response) => {
  try {
    const dbAppId = await getSetting('github_app_id');
    const appId = config.githubAppId || dbAppId;
    
    if (!appId) {
      return res.status(400).json({
        error: 'GitHub App not configured. Please create one first.',
      });
    }
    
    const returnUrl = req.query.return_url as string || `${config.frontendUrl}/settings`;
    await saveSetting('github_return_url', returnUrl);
    
    // Get app slug for installation URL
    const appSlug = await getSetting('github_app_slug') || 'docklift-app';
    
    // Redirect to GitHub App installation
    const installUrl = `https://github.com/apps/${appSlug}/installations/new`;
    res.redirect(installUrl);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to initiate GitHub install' });
  }
});

// GET /setup - Handle GitHub App installation callback
router.get('/setup', async (req: Request, res: Response) => {
  try {
    const installationId = req.query.installation_id as string;
    
    if (!installationId) {
      return res.status(400).json({ error: 'No installation_id provided' });
    }
    
    // Save the installation ID
    await saveSetting('github_installation_id', installationId);
    
    // Get installation details to save username
    try {
      const jwtToken = await createJwtToken();
      
      const response = await fetch(
        `${GITHUB_API_URL}/app/installations/${installationId}`,
        {
          headers: {
            Authorization: `Bearer ${jwtToken}`,
            Accept: 'application/vnd.github+json',
            'X-GitHub-Api-Version': '2022-11-28',
          },
        }
      );
      
      if (response.ok) {
        const data = await response.json() as { account?: { login?: string; avatar_url?: string } };
        const account = data.account || {};
        await saveSetting('github_username', account.login || 'unknown');
        await saveSetting('github_avatar_url', account.avatar_url || '');
      }
    } catch {
      // Continue even if we can't get details
    }
    
    // Redirect back to original page
    const returnUrl = await getSetting('github_return_url') || `${config.frontendUrl}/settings`;
    const separator = returnUrl.includes('?') ? '&' : '?';
    res.redirect(`${returnUrl}${separator}github=connected`);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to complete GitHub setup' });
  }
});

// GET /callback - Handle OAuth callback (alternative flow)
router.get('/callback', async (req: Request, res: Response) => {
  try {
    const installationId = req.query.installation_id as string;
    const code = req.query.code as string;
    
    if (installationId) {
      // This is a setup callback, redirect to setup handler
      return res.redirect(`/api/github/setup?installation_id=${installationId}`);
    }
    
    if (!code) {
      return res.status(400).json({ error: 'No code provided' });
    }
    
    // OAuth flow for user authentication
    const response = await fetch('https://github.com/login/oauth/access_token', {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        client_id: config.githubClientId,
        client_secret: config.githubClientSecret,
        code,
      }),
    });
    
    if (!response.ok) {
      return res.status(400).json({ error: 'Failed to get access token' });
    }
    
    const data = await response.json() as { access_token?: string };
    const accessToken = data.access_token;
    
    if (accessToken) {
      // Get user info
      const userResponse = await fetch(`${GITHUB_API_URL}/user`, {
        headers: {
          Authorization: `token ${accessToken}`,
          Accept: 'application/json',
        },
      });
      
      if (userResponse.ok) {
        const userData = await userResponse.json() as { login?: string; avatar_url?: string };
        await saveSetting('github_username', userData.login || 'unknown');
        await saveSetting('github_avatar_url', userData.avatar_url || '');
      }
    }
    
    res.redirect(`${config.frontendUrl}/settings?github=connected`);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to complete GitHub callback' });
  }
});

// GET /status - Check GitHub App installation status
router.get('/status', async (req: Request, res: Response) => {
  try {
    const installationId = await getSetting('github_installation_id');
    const username = await getSetting('github_username');
    const avatarUrl = await getSetting('github_avatar_url');
    
    if (!installationId) {
      return res.json({ connected: false, username: null });
    }
    
    // Check if private key exists
    const privateKey = getPrivateKey();
    if (!privateKey) {
      return res.json({
        connected: true,
        username,
        avatar_url: avatarUrl,
        installation_id: installationId,
        warning: 'Private key not found',
      });
    }
    
    res.json({
      connected: true,
      username,
      avatar_url: avatarUrl,
      installation_id: installationId,
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get GitHub status' });
  }
});

// GET /repos - List repositories accessible to the GitHub App
router.get('/repos', async (req: Request, res: Response) => {
  try {
    const installationId = await getSetting('github_installation_id');
    
    if (!installationId) {
      return res.status(401).json({ error: 'GitHub not connected' });
    }
    
    const page = parseInt(req.query.page as string) || 1;
    const perPage = Math.min(parseInt(req.query.per_page as string) || 30, 100);
    
    const token = await getInstallationToken(installationId);
    
    const response = await fetch(
      `${GITHUB_API_URL}/installation/repositories?page=${page}&per_page=${perPage}`,
      {
        headers: {
          Authorization: `token ${token}`,
          Accept: 'application/vnd.github+json',
          'X-GitHub-Api-Version': '2022-11-28',
        },
      }
    );
    
    if (!response.ok) {
      return res.status(response.status).json({ error: 'Failed to fetch repos' });
    }
    
    const data = await response.json() as { repositories?: Record<string, unknown>[] };
    const repos = data.repositories || [];
    
    res.json(
      repos.map((repo: Record<string, unknown>) => ({
        id: repo.id,
        name: repo.name,
        full_name: repo.full_name,
        private: repo.private,
        clone_url: repo.clone_url,
        html_url: repo.html_url,
        description: repo.description,
        default_branch: repo.default_branch,
        updated_at: repo.updated_at,
      }))
    );
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to fetch repositories' });
  }
});

// GET /branches - List branches for a repository
router.get('/branches', async (req: Request, res: Response) => {
  try {
    const { repo, type } = req.query;
    
    if (!repo || typeof repo !== 'string') {
      return res.status(400).json({ error: 'Repo parameter is required (owner/name)' });
    }

    let url = `${GITHUB_API_URL}/repos/${repo}/branches`;
    let headers: Record<string, string> = {
      Accept: 'application/vnd.github+json',
      'X-GitHub-Api-Version': '2022-11-28',
      'User-Agent': 'Docklift-App'
    };

    // If private/app repository, use installation token
    if (type === 'private') {
      const installationId = await getSetting('github_installation_id');
      if (!installationId) {
        return res.status(401).json({ error: 'GitHub not connected' });
      }
      const token = await getInstallationToken(installationId);
      headers = { ...headers, Authorization: `token ${token}` };
    }

    const response = await fetch(url, { headers });

    if (!response.ok) {
      if (response.status === 404) return res.status(404).json({ error: 'Repository not found' });
      if (response.status === 403) return res.status(403).json({ error: 'Rate limit exceeded or access denied' });
      throw new Error(`GitHub API Error: ${response.statusText}`);
    }

    const branches = await response.json() as { name: string }[];
    res.json(branches.map(b => b.name));
  } catch (error: any) {
    console.error('Failed to fetch branches:', error);
    res.status(500).json({ error: error.message || 'Failed to fetch branches' });
  }
});

// POST /disconnect - Disconnect GitHub App
router.post('/disconnect', async (req: Request, res: Response) => {
  try {
    await deleteSetting('github_installation_id');
    await deleteSetting('github_username');
    await deleteSetting('github_avatar_url');
    
    res.json({ message: 'GitHub disconnected' });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to disconnect GitHub' });
  }
});

export default router;
