import { spawn, ChildProcess, SpawnOptions } from 'child_process';

export interface RunComposeHandlers {
  onStdout?: (data: Buffer) => void;
  onStderr?: (data: Buffer) => void;
  onClose: (code: number | null) => void | Promise<void>;
  onError?: (err: Error) => void | Promise<void>;
}

/** Spawn `docker` with mandatory error + close handlers (never leave spawn unhandled). */
export function runCompose(
  dockerArgs: string[],
  options: SpawnOptions,
  handlers: RunComposeHandlers,
): ChildProcess {
  const child = spawn('docker', dockerArgs, { shell: false, ...options });

  child.stdout?.on('data', (data: Buffer) => handlers.onStdout?.(data));
  child.stderr?.on('data', (data: Buffer) => handlers.onStderr?.(data));

  child.on('close', (code) => {
    void Promise.resolve(handlers.onClose(code)).catch((err) => {
      console.error('runCompose onClose error:', err);
    });
  });

  child.on('error', (err) => {
    if (handlers.onError) {
      void Promise.resolve(handlers.onError(err)).catch((e) => {
        console.error('runCompose onError handler failed:', e);
      });
    } else {
      console.error('docker spawn error:', err);
      void Promise.resolve(handlers.onClose(null)).catch((e) => {
        console.error('runCompose fallback onClose error:', e);
      });
    }
  });

  return child;
}
