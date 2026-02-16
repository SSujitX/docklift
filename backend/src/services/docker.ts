// Docker service - container operations (status, logs, stats) and compose streaming
import Docker from 'dockerode';
import { spawn } from 'child_process';
import { Response } from 'express';
import path from 'path';
import { config } from '../lib/config.js';

const docker = new Docker();

// Ensure Docker network exists
export async function ensureNetwork(): Promise<void> {
  try {
    await docker.getNetwork(config.dockerNetwork).inspect();
  } catch {
    await docker.createNetwork({
      Name: config.dockerNetwork,
      Driver: 'bridge',
    });
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
    
    // Calculate CPU and memory usage
    const cpuDelta = stats.cpu_stats.cpu_usage.total_usage - stats.precpu_stats.cpu_usage.total_usage;
    const systemDelta = stats.cpu_stats.system_cpu_usage - stats.precpu_stats.system_cpu_usage;
    const cpuPercent = systemDelta > 0 ? (cpuDelta / systemDelta) * 100 : 0;
    
    const memoryUsage = stats.memory_stats.usage || 0;
    const memoryLimit = stats.memory_stats.limit || 1;
    const memoryPercent = (memoryUsage / memoryLimit) * 100;
    
    return {
      cpu_percent: cpuPercent.toFixed(2),
      memory_usage: (memoryUsage / 1024 / 1024).toFixed(2) + ' MB',
      memory_limit: (memoryLimit / 1024 / 1024).toFixed(2) + ' MB',
      memory_percent: memoryPercent.toFixed(2),
    };
  } catch {
    return null;
  }
}

// Stream docker compose up
export function streamComposeUp(projectPath: string, projectId: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`🚀 DEPLOYMENT STARTED\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  res.write(`📦 Phase 1: Building Docker Image...\n`);
  res.write(`${'─'.repeat(40)}\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', projectId, 'up', '-d', '--build'], {
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
export function streamComposeDown(projectPath: string, projectId: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`⏹️ STOPPING CONTAINERS\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', projectId, 'down'], {
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
  const docker = new Docker();

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
export function streamComposeRestart(projectPath: string, projectId: string, res: Response): void {
  const timestamp = new Date().toISOString();
  
  res.write(`\n${'━'.repeat(50)}\n`);
  res.write(`🔄 RESTARTING CONTAINERS\n`);
  res.write(`📅 ${timestamp}\n`);
  res.write(`${'━'.repeat(50)}\n\n`);
  
  const childProcess = spawn('docker', ['compose', '-p', projectId, 'restart'], {
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
