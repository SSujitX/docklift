// Nginx service - manages reverse proxy configurations for custom domains + SSL
import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { config } from '../lib/config.js';
import * as dockerService from './docker.js';
import {
  buildHttpHttpsServers,
  buildServiceProxyLocation,
} from './nginxSsl.js';
import { appendSslEvent, certificateFilesExist, issueCertificate } from './certs.js';

export async function updateServiceDomain(
  service: any,
  opts?: { issueSsl?: boolean; forceSsl?: boolean }
) {
  // Ensure config directory exists
  if (!fs.existsSync(config.nginxConfPath)) {
    fs.mkdirSync(config.nginxConfPath, { recursive: true });
  }

  const confPath = path.join(config.nginxConfPath, `service-${service.id}.conf`);

  // Only generate config if service is running and has a domain
  const shouldExist =
    service.domain &&
    service.container_name &&
    (service.status === 'running' || service.status === 'starting');

  if (!shouldExist) {
    if (fs.existsSync(confPath)) {
      fs.unlinkSync(confPath);
      await reloadNginx();
    }
    return;
  }

  try {
    const containerStatus = await dockerService.getContainerStatus(service.container_name);
    if (!containerStatus.running && service.status !== 'starting') {
      console.warn(
        `Nginx config for ${service.name}: container ${service.container_name} not running yet — config kept for when it starts.`
      );
    }
  } catch {
    console.warn(
      `Nginx config for ${service.name}: could not inspect container ${service.container_name} — config kept.`
    );
  }

  const domainsArray = service.domain
    .split(',')
    .map((d: string) => d.trim().toLowerCase())
    .filter(Boolean);

  if (domainsArray.length === 0) {
    if (fs.existsSync(confPath)) {
      fs.unlinkSync(confPath);
      await reloadNginx();
    }
    return;
  }

  // Configure only hostnames explicitly entered by the user. In particular, do not
  // silently add www: a missing www DNS record would fail the entire certificate order.
  const primaryDomain = domainsArray[0];
  const mainDomainsStr = domainsArray.join(' ');
  const proxyLocation = buildServiceProxyLocation(
    service.id,
    service.container_name,
    service.internal_port
  );

  // 1) Write HTTP(+ACME) config so challenges can succeed
  let content = buildHttpHttpsServers({
    comment: `Main server block for ${service.name}`,
    serverNames: mainDomainsStr,
    primaryDomain,
    proxyLocation,
    enableHttps: certificateFilesExist(primaryDomain),
  });

  try {
    fs.writeFileSync(confPath, content);
    await reloadNginx();
    appendSslEvent(domainsArray, 'info', 'HTTP vhost written and nginx reloaded — ACME challenge path is live.');
  } catch (error) {
    console.error('Failed to write Nginx config:', error);
    appendSslEvent(domainsArray, 'error', 'Failed to write the nginx vhost — check backend logs.');
    // Surface reload/write failure — callers must not report domain save as success
    throw error;
  }

  // 2) Issue certificate (optional — default true on domain updates)
  const shouldIssue = opts?.issueSsl !== false;
  if (shouldIssue) {
    const sans = domainsArray;
    try {
      const status = await issueCertificate(sans, { force: opts?.forceSsl === true });
      // 3) Rewrite with HTTPS if active
      if (
        status.status === 'active' ||
        status.status === 'expiring' ||
        certificateFilesExist(primaryDomain)
      ) {
        content = buildHttpHttpsServers({
          comment: `Main server block for ${service.name}`,
          serverNames: mainDomainsStr,
          primaryDomain,
          proxyLocation,
          enableHttps: true,
        });
        fs.writeFileSync(confPath, content);
        try {
          await reloadNginx();
        } catch (reloadErr) {
          appendSslEvent(
            domainsArray,
            'error',
            'Certificate may exist but nginx reload failed — HTTPS vhost not live.',
          );
          throw reloadErr;
        }
        console.log(
          `Updated Nginx+SSL config for ${service.name} (${mainDomainsStr}) status=${status.status}`
        );
        appendSslEvent(domainsArray, 'success', `HTTPS enabled for ${mainDomainsStr}.`);
      } else {
        console.warn(
          `SSL not active for ${primaryDomain}: ${status.status} ${status.error || ''}`
        );
        appendSslEvent(
          domainsArray,
          'warn',
          'Still serving plain HTTP — HTTPS turns on automatically once a certificate exists.'
        );
      }
    } catch (e: any) {
      console.error(`SSL issue failed for ${primaryDomain}:`, e?.message || e);
      appendSslEvent(domainsArray, 'error', e?.message || 'Certificate issuance failed unexpectedly.');
      // Propagate nginx reload failures; soft-fail only pure certbot issuance errors
      if (String(e?.message || e).includes('Nginx reload failed')) {
        throw e;
      }
    }
  } else {
    console.log(`Updated Nginx config for ${service.name} (${mainDomainsStr})`);
  }
}

export async function syncNginxConfigs() {
  try {
    if (!fs.existsSync(config.nginxConfPath)) return;

    const files = fs
      .readdirSync(config.nginxConfPath)
      .filter((f) => f.startsWith('service-') && f.endsWith('.conf'));
    const fileServiceIds = new Set(
      files
        .map((f) => {
          const match = f.match(/^service-(.+)\.conf$/);
          return match ? match[1] : null;
        })
        .filter(Boolean) as string[]
    );

    const { default: prisma } = await import('../lib/prisma.js');

    const knownWithDomain = await prisma.service.findMany({
      where: { domain: { not: null } },
      select: { id: true },
    });
    const knownServiceIds = new Set(knownWithDomain.map((s) => s.id));

    const allServices = await prisma.service.findMany({
      where: {
        domain: { not: null },
        container_name: { not: null },
        status: { in: ['running', 'starting'] },
      },
    });

    let changeMade = false;

    for (const service of allServices) {
      if (!fileServiceIds.has(service.id)) {
        console.log(`Restoring missing Nginx config for ${service.name}`);
        // Avoid hammering LE on every restart — only write/reuse existing certs
        await updateServiceDomain(service, { issueSsl: false });
        // If cert missing, attempt issue once
        const primary = (service.domain || '')
          .split(',')
          .map((d) => d.trim())
          .filter(Boolean)[0];
        if (primary && !certificateFilesExist(primary)) {
          await updateServiceDomain(service, { issueSsl: true });
        }
      }
    }

    const filesToDelete = [...fileServiceIds].filter((id) => !knownServiceIds.has(id));

    for (const id of filesToDelete) {
      const file = `service-${id}.conf`;
      console.log(`Removing invalid/orphaned Nginx config: ${file}`);
      fs.unlinkSync(path.join(config.nginxConfPath, file));
      changeMade = true;
    }

    if (changeMade) {
      await reloadNginx();
    } else {
      console.log('Nginx configs are in sync.');
    }
  } catch (error) {
    console.error('Failed to sync Nginx configs:', error);
  }
}

export async function cleanupServiceDomain(serviceId: string) {
  const confPath = path.join(config.nginxConfPath, `service-${serviceId}.conf`);

  if (fs.existsSync(confPath)) {
    try {
      fs.unlinkSync(confPath);
      console.log(`Removed Nginx config for service ${serviceId}`);
      await reloadNginx();
    } catch (error) {
      console.error(`Failed to remove Nginx config for service ${serviceId}:`, error);
    }
  }
}

function attemptSelfHealFromReloadError(stderrBuffer: string): boolean {
  const match = stderrBuffer.match(
    /host not found in upstream .* in (.*\/service-[a-zA-Z0-9-]+\.conf):/
  );
  if (!match?.[1]) return false;

  const filename = path.basename(match[1]);
  const localPath = path.join(config.nginxConfPath, filename);

  if (!fs.existsSync(localPath)) return false;

  console.log(`Self-Healing: Removing bad Nginx config causing crash: ${filename}`);
  try {
    fs.unlinkSync(localPath);
    return true;
  } catch (err) {
    console.error('Failed to remove bad config:', err);
    return false;
  }
}

function runNginxReload(): Promise<{ ok: boolean; code: number | null; stderr: string; spawnError?: Error }> {
  return new Promise((resolve) => {
    console.log('Reloading Nginx proxy...');
    const child = spawn('docker', ['exec', 'docklift-nginx-proxy', 'nginx', '-s', 'reload']);

    let stderrBuffer = '';

    child.stdout.on('data', (data) => console.log(`Nginx stdout: ${data}`));
    child.stderr.on('data', (data) => {
      const str = data.toString();
      stderrBuffer += str;
      console.error(`Nginx stderr: ${str}`);
    });

    child.on('close', (code) => {
      if (code === 0) {
        console.log('Nginx proxy reloaded successfully');
        resolve({ ok: true, code, stderr: stderrBuffer });
      } else {
        console.error(`Nginx reload failed with code ${code}`);
        resolve({ ok: false, code, stderr: stderrBuffer });
      }
    });

    child.on('error', (err) => {
      console.error('Failed to spawn docker exec:', err);
      resolve({ ok: false, code: null, stderr: stderrBuffer, spawnError: err });
    });
  });
}

export async function reloadNginx(): Promise<void> {
  const first = await runNginxReload();
  if (first.ok) return;

  attemptSelfHealFromReloadError(first.stderr);

  const second = await runNginxReload();
  if (second.ok) return;

  const detail =
    second.spawnError?.message ||
    second.stderr.trim() ||
    (second.code != null ? `exit code ${second.code}` : 'unknown error');
  throw new Error(`Nginx reload failed: ${detail}`);
}
