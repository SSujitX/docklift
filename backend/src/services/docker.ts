// Docker service - container operations (status, logs, stats) and compose streaming
import Docker from 'dockerode';
import { spawn, type ChildProcess } from 'child_process';
import { Response } from 'express';
import path from 'path';
import { config } from '../lib/config.js';
import { projectNetworkName } from '../lib/naming.js';

const DEFAULT_PULL_TIMEOUT_MS = 600_000;

const docker = new Docker();

const EDGE_PROXY_CONTAINER = process.env.NGINX_PROXY_CONTAINER || 'docklift-nginx-proxy';

// Ensure Docker network exists
export async function ensureNetwork(): Promise<void> {
  try {
    await docker.getNetwork(config.dockerNetwork).inspect();
  } catch {
    await docker.createNetwork({
      Name: config.dockerNetwork,
      Driver: 'bridge',
      Labels: {
        'com.docklift.managed': 'true',
        'com.docklift.role': 'control-plane',
      },
    });
  }
}

export function isProxyAlreadyConnectedError(err: unknown): boolean {
  const msg = String((err as { message?: string })?.message || err || '');
  return /already exists|already connected/i.test(msg);
}

/**
 * Attach edge proxy to a project network so it can resolve container_name DNS.
 * Throws on failure (callers must mark deploy degraded/failed — never pretend success).
 */
export async function connectProxyToProjectNetwork(projectId: string): Promise<void> {
  const netName = projectNetworkName(projectId);
  const network = docker.getNetwork(netName);
  await network.inspect();
  try {
    await network.connect({ Container: EDGE_PROXY_CONTAINER });
  } catch (err: unknown) {
    if (isProxyAlreadyConnectedError(err)) return;
    console.error(`[docker] Failed to attach ${EDGE_PROXY_CONTAINER} to ${netName}:`, err);
    throw err instanceof Error ? err : new Error(String(err));
  }
}

/** Disconnect edge proxy from a project network so compose down can remove it. */
export async function disconnectProxyFromProjectNetwork(projectId: string): Promise<void> {
  const netName = projectNetworkName(projectId);
  try {
    const network = docker.getNetwork(netName);
    await network.disconnect({ Container: EDGE_PROXY_CONTAINER, Force: true });
  } catch {
    /* not connected or network already gone */
  }
}

/** Detach edge proxy and remove project network (best-effort on delete). */
export async function teardownProjectNetwork(projectId: string): Promise<void> {
  const netName = projectNetworkName(projectId);
  await disconnectProxyFromProjectNetwork(projectId);
  try {
    await docker.getNetwork(netName).remove();
  } catch {
    /* network may already be gone after compose down */
  }
}

/** Attach any container to a project bridge network (managed DB ↔ app linking). */
export async function connectContainerToProjectNetwork(
  projectId: string,
  containerName: string,
): Promise<void> {
  if (!containerName?.trim()) {
    throw new Error('containerName is required');
  }
  const netName = projectNetworkName(projectId);
  const network = docker.getNetwork(netName);
  await network.inspect();
  try {
    await network.connect({ Container: containerName });
  } catch (err: unknown) {
    if (isProxyAlreadyConnectedError(err)) return;
    throw err instanceof Error ? err : new Error(String(err));
  }
}

/** Detach a container from a project network (best-effort on unlink). */
export async function disconnectContainerFromProjectNetwork(
  projectId: string,
  containerName: string,
): Promise<void> {
  if (!containerName?.trim()) return;
  const netName = projectNetworkName(projectId);
  try {
    await docker.getNetwork(netName).disconnect({ Container: containerName, Force: true });
  } catch {
    /* not connected or network gone */
  }
}

// Get container status
export async function getContainerStatus(containerName: string): Promise<{ status: string; running: boolean }> {
  try {
    // Try exact match first
    const container = docker.getContainer(containerName);
    const info = await container.inspect();
    return {
      status: info.State.Status,
      running: info.State.Running,
    };
  } catch {
    // Try partial match using list
    try {
      const containers = await docker.listContainers({ all: true, filters: { name: [containerName] } });
      if (containers.length > 0) {
        return {
          status: containers[0].State,
          running: containers[0].State === 'running',
        };
      }
    } catch {
      // Ignore
    }
    return { status: 'not_found', running: false };
  }
}

// Get container logs
export async function getContainerLogs(containerName: string, tail = 100): Promise<string> {
  try {
    const container = docker.getContainer(containerName);
    const logs = await container.logs({
      stdout: true,
      stderr: true,
      tail,
      timestamps: false,
    });
    return logs.toString();
  } catch {
    return '';
  }
}

// Get container stats
export async function getContainerStats(containerName: string): Promise<Record<string, unknown> | null> {
  try {
    const container = docker.getContainer(containerName);
    const stats = await container.stats({ stream: false });

    const cpuDelta =
      (stats.cpu_stats?.cpu_usage?.total_usage || 0) -
      (stats.precpu_stats?.cpu_usage?.total_usage || 0);
    const systemDelta =
      (stats.cpu_stats?.system_cpu_usage || 0) -
      (stats.precpu_stats?.system_cpu_usage || 0);
    const onlineCpus =
      stats.cpu_stats?.online_cpus ||
      stats.cpu_stats?.cpu_usage?.percpu_usage?.length ||
      1;
    const cpuPercent =
      systemDelta > 0 ? (cpuDelta / systemDelta) * onlineCpus * 100 : 0;

    const memoryUsage = stats.memory_stats?.usage || 0;
    const memoryLimit = stats.memory_stats?.limit || 1;
    const memoryPercent =
      memoryLimit > 0 ? (memoryUsage / memoryLimit) * 100 : 0;

    return {
      cpu_percent: (Number.isFinite(cpuPercent) ? cpuPercent : 0).toFixed(2),
      memory_usage: (memoryUsage / 1024 / 1024).toFixed(2) + ' MB',
      memory_limit: (memoryLimit / 1024 / 1024).toFixed(2) + ' MB',
      memory_percent: (Number.isFinite(memoryPercent) ? memoryPercent : 0).toFixed(2),
    };
  } catch {
    return null;
  }
}

export interface PullImageOptions {
  onChunk?: (text: string) => void;
  /** Register the child immediately so /cancel can kill mid-pull. */
  onSpawn?: (child: ChildProcess) => void;
  /** Kill and reject after this many ms (default 10 minutes). */
  timeoutMs?: number;
  /** When aborted, the pull child is killed and the promise rejects. */
  signal?: AbortSignal;
}

/**
 * Pull an image without blocking the Node event loop.
 * Streams docker pull output so deploy logs stay live.
 * Supports cancel (kill child) and a hard timeout.
 */
export function pullImage(
  image: string,
  onChunkOrOptions?: ((text: string) => void) | PullImageOptions,
): Promise<void> {
  const options: PullImageOptions =
    typeof onChunkOrOptions === 'function'
      ? { onChunk: onChunkOrOptions }
      : onChunkOrOptions || {};
  const timeoutMs = options.timeoutMs ?? DEFAULT_PULL_TIMEOUT_MS;

  return new Promise((resolve, reject) => {
    if (options.signal?.aborted) {
      reject(new Error(`Pull cancelled: ${image}`));
      return;
    }

    const child = spawn('docker', ['pull', image], {
      shell: false,
      windowsHide: true,
      detached: process.platform !== 'win32',
    });
    options.onSpawn?.(child);

    let settled = false;
    const settle = (fn: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      options.signal?.removeEventListener('abort', onAbort);
      fn();
    };

    const killPull = () => {
      if (!child.pid) return;
      try {
        if (process.platform === 'win32') {
          spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], {
            stdio: 'ignore',
            windowsHide: true,
          });
        } else {
          try {
            process.kill(-child.pid, 'SIGTERM');
          } catch {
            child.kill('SIGTERM');
          }
          setTimeout(() => {
            try {
              if (child.pid) {
                try {
                  process.kill(-child.pid, 'SIGKILL');
                } catch {
                  child.kill('SIGKILL');
                }
              }
            } catch {
              /* already dead */
            }
          }, 2000);
        }
      } catch {
        try {
          child.kill('SIGKILL');
        } catch {
          /* ignore */
        }
      }
    };

    const onAbort = () => {
      killPull();
      settle(() => reject(new Error(`Pull cancelled: ${image}`)));
    };
    options.signal?.addEventListener('abort', onAbort, { once: true });

    const timer = setTimeout(() => {
      killPull();
      settle(() =>
        reject(new Error(`Timed out pulling ${image} after ${timeoutMs}ms`)),
      );
    }, timeoutMs);

    child.stdout?.on('data', (data: Buffer) => options.onChunk?.(data.toString()));
    child.stderr?.on('data', (data: Buffer) => options.onChunk?.(data.toString()));
    child.on('error', (err) => settle(() => reject(err)));
    child.on('close', (code) => {
      if (options.signal?.aborted) {
        settle(() => reject(new Error(`Pull cancelled: ${image}`)));
        return;
      }
      if (code === 0) settle(() => resolve());
      else {
        settle(() =>
          reject(new Error(`Failed to pull ${image} (exit ${code ?? 'null'})`)),
        );
      }
    });
  });
}

// Stream docker compose up
export function streamComposeUp(projectPath: string, composeProject: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`🚀 DEPLOYMENT STARTED\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  res.write(`📦 Phase 1: Building Docker Image...\n`);
  res.write(`${'─'.repeat(40)}\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', composeProject, 'up', '-d', '--build'], {
    cwd: projectPath,
    env: { ...globalThis.process.env, DOCKER_BUILDKIT: '1', COMPOSE_DOCKER_CLI_BUILD: '1' },
    shell: false,
  });
  
  childProcess.stdout!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.stderr!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.on('close', (code) => {
    res.write(`\n${'─'.repeat(40)}\n`);
    
    if (code === 0) {
      res.write(`\n${'━'.repeat(50)}\n`);
      res.write(`✅ DEPLOYMENT SUCCESSFUL!\n`);
      res.write(`${'━'.repeat(50)}\n`);
    } else {
      res.write(`\n${'━'.repeat(50)}\n`);
      res.write(`❌ DEPLOYMENT FAILED (code ${code})\n`);
      res.write(`${'━'.repeat(50)}\n`);
    }
    
    res.end();
  });
  
  childProcess.on('error', (err) => {
    res.write(`\n❌ Error: ${err.message}\n`);
    res.end();
  });
}

// Stream docker compose down
export function streamComposeDown(projectPath: string, composeProject: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`⏹️ STOPPING CONTAINERS\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', composeProject, 'down'], {
    cwd: projectPath,
    shell: false,
  });
  
  childProcess.stdout!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.stderr!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.on('close', (code) => {
    res.write(`\n${'━'.repeat(50)}\n`);
    res.write(`✅ CONTAINERS STOPPED\n`);
    res.write(`${'━'.repeat(50)}\n`);
    res.end();
  });
}

// Stream real-time container logs via SSE
export function streamContainerLogs(containerName: string, res: Response, tail = 200): void {

  // Set SSE headers
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('X-Accel-Buffering', 'no');

  const container = docker.getContainer(containerName);

  container.inspect().then((info) => {
    if (!info.State.Running) {
      res.write(`data: ${JSON.stringify({ type: 'status', message: 'Container is not running' })}\n\n`);
      res.end();
      return;
    }

    // Stream logs with follow
    container.logs({
      stdout: true,
      stderr: true,
      follow: true,
      tail,
      timestamps: true,
    }).then((logStream: any) => {
      let closed = false;

      // Safe write helper — guards against write-after-end crashes
      const safeWrite = (data: string) => {
        if (closed) return;
        try {
          res.write(data);
          (res as any).flush?.();
        } catch { /* ignore write errors on closed connections */ }
      };

      safeWrite(`data: ${JSON.stringify({ type: 'connected', container: containerName })}\n\n`);

      // Docker multiplexed stream: each frame has 8-byte header
      // [stream_type(1)][0(3)][size(4)][payload(size)]
      let buffer = Buffer.alloc(0);

      const processBuffer = () => {
        while (buffer.length >= 8) {
          const size = buffer.readUInt32BE(4);
          if (buffer.length < 8 + size) break; // wait for more data

          const payload = buffer.subarray(8, 8 + size).toString('utf-8');
          buffer = buffer.subarray(8 + size);

          if (payload.trim()) {
            safeWrite(`data: ${JSON.stringify({ type: 'log', message: payload })}\n\n`);
          }
        }
      };

      logStream.on('data', (chunk: Buffer) => {
        if (closed) return;
        // Try to detect if this is a multiplexed stream or raw
        // Multiplexed streams have header bytes 0x01 (stdout) or 0x02 (stderr) at position 0
        const firstByte = chunk[0];
        if (firstByte === 0x01 || firstByte === 0x02) {
          buffer = Buffer.concat([buffer, chunk]);
          processBuffer();
        } else {
          // Raw stream (e.g., TTY mode)
          const text = chunk.toString('utf-8');
          if (text.trim()) {
            safeWrite(`data: ${JSON.stringify({ type: 'log', message: text })}\n\n`);
          }
        }
      });

      logStream.on('end', () => {
        if (closed) return;
        safeWrite(`data: ${JSON.stringify({ type: 'end', message: 'Log stream ended' })}\n\n`);
        try { res.end(); } catch { /* ignore */ }
      });

      logStream.on('error', (err: Error) => {
        if (closed) return;
        safeWrite(`data: ${JSON.stringify({ type: 'error', message: err.message })}\n\n`);
        try { res.end(); } catch { /* ignore */ }
      });

      // Cleanup on client disconnect
      res.on('close', () => {
        closed = true;
        try {
          logStream.destroy();
        } catch {
          // Ignore cleanup errors
        }
      });
    }).catch((err: Error) => {
      try {
        res.write(`data: ${JSON.stringify({ type: 'error', message: `Failed to stream logs: ${err.message}` })}\n\n`);
        res.end();
      } catch { /* ignore if already closed */ }
    });
  }).catch((err: Error) => {
    res.write(`data: ${JSON.stringify({ type: 'error', message: `Container not found: ${err.message}` })}\n\n`);
    res.end();
  });
}

// Stream docker compose restart
export function streamComposeRestart(projectPath: string, composeProject: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`🔄 RESTARTING CONTAINERS\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', composeProject, 'restart'], {
    cwd: projectPath,
    shell: false,
  });
  
  childProcess.stdout!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.stderr!.on('data', (data) => {
    res.write(data.toString());
  });
  
  childProcess.on('close', (code) => {
    res.write(`\n${'━'.repeat(50)}\n`);
    res.write(`✅ CONTAINERS RESTARTED\n`);
    res.write(`${'━'.repeat(50)}\n`);
    res.end();
  });
}
