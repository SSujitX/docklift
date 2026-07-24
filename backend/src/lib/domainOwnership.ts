import fs from 'fs/promises';
import path from 'path';
import prisma from './prisma.js';
import { config } from './config.js';
import { PANEL_CONF_MARKER } from '../services/nginxSsl.js';

export function normalizeDomainList(input: unknown): string[] {
  const parts = Array.isArray(input)
    ? input.map((item) => String(item))
    : String(input ?? '').split(',');
  const seen = new Set<string>();
  const hosts: string[] = [];
  for (const part of parts) {
    const host = part.trim().toLowerCase();
    if (!host || seen.has(host)) continue;
    seen.add(host);
    hosts.push(host);
  }
  return hosts;
}

export function formatDomainField(hosts: string[]): string | null {
  if (hosts.length === 0) return null;
  return hosts.join(', ');
}

async function panelHostnames(): Promise<Set<string>> {
  const hosts = new Set<string>();
  try {
    await fs.mkdir(config.nginxConfPath, { recursive: true });
    const files = await fs.readdir(config.nginxConfPath);
    for (const file of files) {
      if (!file.endsWith('.conf') || file === 'default.conf' || file.startsWith('service-')) {
        continue;
      }
      const content = await fs.readFile(path.join(config.nginxConfPath, file), 'utf-8');
      if (
        !content.includes(PANEL_CONF_MARKER) &&
        !content.includes('host.docker.internal') &&
        !content.includes('docklift-nginx')
      ) {
        continue;
      }
      if (content.includes('service-') && content.includes('Main server block')) continue;
      const match = content.match(/server_name\s+([^;]+);/);
      if (match) {
        for (const name of match[1].trim().split(/\s+/)) {
          if (name) hosts.add(name.toLowerCase());
        }
      } else {
        hosts.add(file.slice(0, -5).toLowerCase());
      }
    }
  } catch {
    // unreadable nginx dir — treat as no panel hosts
  }
  return hosts;
}

export async function assertHostnamesAvailable(
  hosts: string[],
  opts?: { excludeServiceId?: string },
): Promise<void> {
  if (hosts.length === 0) return;

  const requested = new Set(hosts);

  const services = await prisma.service.findMany({
    where: { domain: { not: null } },
    select: { id: true, domain: true },
  });
  for (const service of services) {
    if (opts?.excludeServiceId && service.id === opts.excludeServiceId) continue;
    for (const host of normalizeDomainList(service.domain)) {
      if (requested.has(host)) {
        throw new Error(`Hostname "${host}" is already assigned to another service`);
      }
    }
  }

  const panelHosts = await panelHostnames();
  for (const host of hosts) {
    if (panelHosts.has(host)) {
      throw new Error(`Hostname "${host}" is already used by a panel domain`);
    }
  }
}
