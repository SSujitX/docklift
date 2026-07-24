// Domains routes - panel/server custom domains + Let's Encrypt SSL
import express, { Request, Response, Router } from 'express';
import fs from 'fs/promises';
import fssync from 'fs';
import path from 'path';
import { config } from '../lib/config.js';
import {
  clearSslMeta,
  getAcmeEmail,
  getCertificateStatus,
  issueCertificate,
  setAcmeEmail,
} from '../services/certs.js';
import { reloadNginx } from '../services/nginx.js';
import {
  PANEL_CONF_MARKER,
  buildHttpHttpsServers,
  buildPanelProxyLocation,
  buildWwwRedirectServers,
} from '../services/nginxSsl.js';
import { certificateFilesExist } from '../services/certs.js';

const router: Router = express.Router();
const NGINX_CONF_PATH = config.nginxConfPath;

const DOMAIN_REGEX =
  /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/;

function panelConfPath(domain: string): string {
  return path.join(NGINX_CONF_PATH, `${domain}.conf`);
}

function parsePanelPort(content: string): number | null {
  // New style: docklift-nginx (dashboard)
  if (content.includes('docklift-nginx') || content.includes('$dashboard')) {
    const m = content.match(/# docklift-panel-port:\s*(\d+)/);
    return m ? parseInt(m[1], 10) : 8080;
  }
  const portMatch = content.match(/proxy_pass\s+http:\/\/host\.docker\.internal:(\d+);/);
  return portMatch ? parseInt(portMatch[1], 10) : null;
}

async function writePanelDomainConfig(domain: string, port: number, enableHttps: boolean) {
  await fs.mkdir(NGINX_CONF_PATH, { recursive: true });
  const proxyLocation = buildPanelProxyLocation(port);
  const body = buildHttpHttpsServers({
    comment: `Panel domain ${domain}`,
    serverNames: domain,
    primaryDomain: domain,
    proxyLocation,
    enableHttps,
  });
  const wwwRedirects =
    !domain.startsWith('www.') && !domain.match(/^[\d.]+$/) && !domain.includes('localhost')
      ? [`www.${domain}`]
      : [];
  const wwwBlock = buildWwwRedirectServers(wwwRedirects, domain, enableHttps && certificateFilesExist(domain));
  const nginxConfig = `${PANEL_CONF_MARKER}
# docklift-panel-port: ${port}
${body}
${wwwBlock}`;
  await fs.writeFile(panelConfPath(domain), nginxConfig);
}

async function provisionPanelSsl(domain: string, port: number, opts?: { force?: boolean }) {
  await writePanelDomainConfig(domain, port, false);
  await reloadNginx();

  // Include www SAN (same pattern as service domains) when not already www.*
  const sans = [domain];
  if (!domain.startsWith('www.') && !domain.match(/^[\d.]+$/) && !domain.includes('localhost')) {
    sans.push(`www.${domain}`);
  }

  const status = await issueCertificate(sans, { force: opts?.force });
  if (
    status.status === 'active' ||
    status.status === 'expiring' ||
    certificateFilesExist(domain)
  ) {
    await writePanelDomainConfig(domain, port, true);
    await reloadNginx();
  }
  return status;
}

// GET /api/domains/ssl/email — ACME account email
router.get('/ssl/email', async (_req: Request, res: Response) => {
  try {
    const email = await getAcmeEmail().catch(() => '');
    res.json({ email });
  } catch (error: any) {
    res.status(500).json({ error: 'Failed to get ACME email' });
  }
});

// PUT /api/domains/ssl/email
router.put('/ssl/email', async (req: Request, res: Response) => {
  try {
    const { email } = req.body || {};
    await setAcmeEmail(email);
    res.json({ success: true, email: email.trim().toLowerCase() });
  } catch (error: any) {
    res.status(400).json({ error: error.message || 'Failed to save ACME email' });
  }
});

// GET /api/domains - List panel domains with SSL status
router.get('/', async (_req: Request, res: Response) => {
  try {
    await fs.mkdir(NGINX_CONF_PATH, { recursive: true });

    const files = await fs.readdir(NGINX_CONF_PATH);
    const configs: Array<{ domain: string; port: number; ssl: Awaited<ReturnType<typeof getCertificateStatus>> }> = [];

    for (const file of files) {
      if (!file.endsWith('.conf') || file === 'default.conf' || file.startsWith('service-')) {
        continue;
      }
      const content = await fs.readFile(path.join(NGINX_CONF_PATH, file), 'utf-8');
      if (!content.includes(PANEL_CONF_MARKER) && !content.includes('host.docker.internal') && !content.includes('docklift-nginx')) {
        continue;
      }
      // Skip if it looks like a leftover non-panel file
      if (content.includes('service-') && content.includes('Main server block')) continue;

      const domainMatch = content.match(/server_name\s+([^;]+);/);
      const port = parsePanelPort(content);
      if (!domainMatch || port == null) continue;

      const domain = domainMatch[1].trim().split(/\s+/)[0];
      const ssl = await getCertificateStatus(domain);
      configs.push({ domain, port, ssl });
    }

    res.json(configs);
  } catch (error: any) {
    console.error('List domains error:', error);
    res.status(500).json({ error: 'Failed to list domains' });
  }
});

// POST /api/domains - Add panel domain + issue SSL
router.post('/', async (req: Request, res: Response) => {
  const { domain, port } = req.body;

  if (!domain || !port || isNaN(parseInt(port))) {
    return res.status(400).json({ error: 'Invalid domain or port' });
  }

  const normalized = String(domain).trim().toLowerCase();
  if (!DOMAIN_REGEX.test(normalized)) {
    return res.status(400).json({ error: 'Invalid domain format' });
  }

  const portNum = parseInt(port, 10);

  try {
    const ssl = await provisionPanelSsl(normalized, portNum);
    res.json({ success: true, domain: normalized, port: portNum, ssl });
  } catch (error: any) {
    console.error('Add domain error:', error);
    res.status(500).json({ error: error.message || 'Failed to add domain' });
  }
});

// POST /api/domains/:domain/ssl/retry
router.post('/:domain/ssl/retry', async (req: Request, res: Response) => {
  const domain = String(req.params.domain || '').trim().toLowerCase();
  if (!DOMAIN_REGEX.test(domain)) {
    return res.status(400).json({ error: 'Invalid domain format' });
  }

  try {
    const content = await fs.readFile(panelConfPath(domain), 'utf-8');
    const port = parsePanelPort(content) ?? 8080;
    await clearSslMeta(domain);
    const ssl = await provisionPanelSsl(domain, port, { force: true });
    res.json({ success: true, domain, ssl });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'Domain configuration not found' });
    }
    console.error('SSL retry error:', error);
    res.status(500).json({ error: error.message || 'SSL retry failed' });
  }
});

// DELETE /api/domains/:domain
router.delete('/:domain', async (req: Request, res: Response) => {
  const domain = String(req.params.domain || '').trim().toLowerCase();

  if (!DOMAIN_REGEX.test(domain)) {
    return res.status(400).json({ error: 'Invalid domain format' });
  }

  try {
    await fs.access(panelConfPath(domain));
    await fs.unlink(panelConfPath(domain));
    await clearSslMeta(domain);
    console.log(`Deleted Nginx config for ${domain}`);
    await reloadNginx();
    res.json({ success: true });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'Domain configuration not found' });
    }
    console.error('Delete domain error:', error);
    res.status(500).json({ error: 'Failed to delete domain' });
  }
});

export default router;
