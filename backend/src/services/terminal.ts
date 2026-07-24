// Terminal service - WebSocket-based interactive shell (zero native dependencies)
import { WebSocketServer, WebSocket } from 'ws';
import { Server as HttpServer, IncomingMessage } from 'http';
import { spawn, ChildProcess } from 'child_process';
import jwt from 'jsonwebtoken';
import bcrypt from 'bcrypt';
import { JWT_SECRET, assertPasswordStillValid, type JwtPayload } from '../lib/authMiddleware.js';
import { PrismaClient } from '@prisma/client';
import { config } from '../lib/config.js';
import { isTrustedOrigin } from '../lib/originCheck.js';

const prisma = new PrismaClient();

interface TerminalSession {
  ws: WebSocket;
  shell: ChildProcess | null;
  authenticated: boolean;
  userId: string | null;
  authAttempts: number;
  idleTimer: ReturnType<typeof setTimeout> | null;
}

const activeSessions = new Set<TerminalSession>();
const MAX_CONCURRENT_SESSIONS = 3;
const MAX_AUTH_ATTEMPTS = 5;
const IDLE_TIMEOUT_MS = 30 * 60 * 1000; // 30 minutes

// Global password brute-force limit (survives reconnect)
const globalAuthFails = new Map<string, { count: number; windowStart: number }>();
const GLOBAL_AUTH_WINDOW_MS = 15 * 60 * 1000;
const GLOBAL_AUTH_MAX_FAILS = 20;

// Message types from client
interface ClientMessage {
  type: 'auth' | 'input' | 'resize';
  password?: string;
  data?: string;
  cols?: number;
  rows?: number;
}

function clientKey(request: IncomingMessage, userId: string): string {
  const ip = request.socket.remoteAddress || 'unknown';
  return `${ip}:${userId}`;
}

function isGloballyRateLimited(key: string): boolean {
  const entry = globalAuthFails.get(key);
  if (!entry) return false;
  if (Date.now() - entry.windowStart > GLOBAL_AUTH_WINDOW_MS) {
    globalAuthFails.delete(key);
    return false;
  }
  return entry.count >= GLOBAL_AUTH_MAX_FAILS;
}

function recordGlobalAuthFail(key: string): void {
  const now = Date.now();
  const entry = globalAuthFails.get(key);
  if (!entry || now - entry.windowStart > GLOBAL_AUTH_WINDOW_MS) {
    globalAuthFails.set(key, { count: 1, windowStart: now });
    return;
  }
  entry.count += 1;
}

function isAllowedOrigin(request: IncomingMessage): boolean {
  const origin = request.headers.origin;
  // Non-browser clients may omit Origin; still require a valid terminal JWT
  if (!origin) {
    // If Origin is missing, allow only when TERMINAL_ALLOW_NO_ORIGIN=true (ops escape hatch)
    return process.env.TERMINAL_ALLOW_NO_ORIGIN === 'true';
  }

  return isTrustedOrigin(origin, request.headers, { allow: [config.frontendUrl] });
}

async function verifyTerminalToken(token: string): Promise<{ userId: string; email: string } | null> {
  try {
    const decoded = jwt.verify(token, JWT_SECRET) as JwtPayload;
    if (decoded.purpose !== 'terminal') {
      return null;
    }
    const pwdErr = await assertPasswordStillValid(decoded);
    if (pwdErr) return null;
    return { userId: decoded.userId, email: decoded.email };
  } catch {
    return null;
  }
}

export function setupTerminalWebSocket(server: HttpServer) {
  const wss = new WebSocketServer({ noServer: true });

  server.on('upgrade', async (request: IncomingMessage, socket, head) => {
    const url = new URL(request.url || '', `http://${request.headers.host}`);

    if (url.pathname !== '/ws/terminal') {
      socket.destroy();
      return;
    }

    if (!isAllowedOrigin(request)) {
      console.warn('[TERMINAL] Rejected upgrade — Origin not allowed:', request.headers.origin);
      socket.write('HTTP/1.1 403 Forbidden\r\n\r\n');
      socket.destroy();
      return;
    }

    const token = url.searchParams.get('token');
    if (!token) {
      socket.write('HTTP/1.1 401 Unauthorized\r\n\r\n');
      socket.destroy();
      return;
    }

    const user = await verifyTerminalToken(token);
    if (!user) {
      socket.write('HTTP/1.1 401 Unauthorized\r\n\r\n');
      socket.destroy();
      return;
    }

    wss.handleUpgrade(request, socket, head, (ws) => {
      wss.emit('connection', ws, request, user);
    });
  });

  wss.on('connection', (ws: WebSocket, request: IncomingMessage, user: { userId: string; email: string }) => {
    if (activeSessions.size >= MAX_CONCURRENT_SESSIONS) {
      console.warn(`[TERMINAL] Max concurrent sessions (${MAX_CONCURRENT_SESSIONS}) reached — rejecting user: ${user.email}`);
      ws.send(JSON.stringify({ type: 'auth_error', message: 'Too many active terminal sessions. Close one first.' }));
      ws.close();
      return;
    }

    const rateKey = clientKey(request, user.userId);
    if (isGloballyRateLimited(rateKey)) {
      ws.send(JSON.stringify({ type: 'auth_error', message: 'Too many failed attempts. Try again later.' }));
      ws.close();
      return;
    }

    const session: TerminalSession = {
      ws,
      shell: null,
      authenticated: false,
      userId: user.userId,
      authAttempts: 0,
      idleTimer: null,
    };

    activeSessions.add(session);
    console.log(`[TERMINAL] WebSocket connected for user: ${user.email}`);

    ws.send(JSON.stringify({ type: 'auth_required' }));

    ws.on('message', async (rawData) => {
      try {
        const msg: ClientMessage = JSON.parse(rawData.toString());

        if (msg.type === 'auth') {
          if (session.authenticated) return;

          if (isGloballyRateLimited(rateKey) || session.authAttempts >= MAX_AUTH_ATTEMPTS) {
            console.warn(`[TERMINAL] Too many failed auth attempts for user: ${user.email}`);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Too many failed attempts. Reconnect later.' }));
            ws.close();
            return;
          }

          if (!msg.password) {
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Password required' }));
            return;
          }

          // Verify password for the JWT user — not an arbitrary first user
          const dbUser = await prisma.user.findUnique({ where: { id: user.userId } });
          if (!dbUser) {
            ws.send(JSON.stringify({ type: 'auth_error', message: 'No user account found' }));
            return;
          }

          const valid = await bcrypt.compare(msg.password, dbUser.password);
          if (!valid) {
            session.authAttempts++;
            recordGlobalAuthFail(rateKey);
            console.warn(`[TERMINAL] Failed auth attempt ${session.authAttempts}/${MAX_AUTH_ATTEMPTS} for user: ${user.email}`);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Invalid password' }));
            return;
          }

          session.authenticated = true;
          console.log(`[TERMINAL] User "${user.email}" authenticated for interactive terminal`);

          try {
            const cols = msg.cols || 80;
            const rows = msg.rows || 24;

            const shell = spawn('script', ['-q', '-c', '/bin/bash', '/dev/null'], {
              env: {
                ...process.env,
                TERM: 'xterm-256color',
                COLORTERM: 'truecolor',
                LANG: 'C.UTF-8',
                COLUMNS: String(cols),
                LINES: String(rows),
                PS1: '\\[\\e[1;32m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[1;34m\\]\\w\\[\\e[0m\\]# ',
              },
              cwd: '/root',
              stdio: ['pipe', 'pipe', 'pipe'],
            });

            session.shell = shell;

            shell.stdout?.on('data', (data: Buffer) => {
              if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'output', data: data.toString() }));
              }
            });

            shell.stderr?.on('data', (data: Buffer) => {
              if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'output', data: data.toString() }));
              }
            });

            shell.on('exit', (code) => {
              console.log(`[TERMINAL] Shell exited with code ${code} for user "${user.email}"`);
              if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'exit', code: code || 0 }));
                ws.close();
              }
            });

            shell.on('error', (err) => {
              console.error(`[TERMINAL] Shell error for user "${user.email}":`, err.message);
              if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'output', data: `\r\nShell error: ${err.message}\r\n` }));
              }
            });

            ws.send(JSON.stringify({ type: 'auth_success' }));

            const resetIdleTimer = () => {
              if (session.idleTimer) clearTimeout(session.idleTimer);
              session.idleTimer = setTimeout(() => {
                console.log(`[TERMINAL] Idle timeout for user: ${user.email}`);
                killShell(session);
                if (ws.readyState === WebSocket.OPEN) {
                  ws.send(JSON.stringify({ type: 'exit', code: -1, reason: 'idle_timeout' }));
                  ws.close();
                }
              }, IDLE_TIMEOUT_MS);
            };
            resetIdleTimer();
            (session as any)._resetIdleTimer = resetIdleTimer;
          } catch (err) {
            console.error('[TERMINAL] Failed to spawn shell:', err);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Failed to start terminal session' }));
          }
          return;
        }

        if (!session.authenticated || !session.shell) return;

        if (msg.type === 'input' && msg.data) {
          session.shell.stdin?.write(msg.data);
          if ((session as any)._resetIdleTimer) (session as any)._resetIdleTimer();
        }

        if (msg.type === 'resize' && msg.cols && msg.rows) {
          const cols = Number(msg.cols);
          const rows = Number(msg.rows);
          if (Number.isInteger(cols) && Number.isInteger(rows)
              && cols > 0 && cols < 500 && rows > 0 && rows < 500) {
            session.shell.stdin?.write(`stty cols ${cols} rows ${rows}\n`);
          }
        }
      } catch {
        // Silently ignore malformed messages
      }
    });

    ws.on('close', () => {
      console.log(`[TERMINAL] WebSocket disconnected for user: ${user.email}`);
      if (session.idleTimer) clearTimeout(session.idleTimer);
      killShell(session);
      activeSessions.delete(session);
    });

    ws.on('error', (err) => {
      console.error(`[TERMINAL] WebSocket error for user ${user.email}:`, err.message);
      if (session.idleTimer) clearTimeout(session.idleTimer);
      killShell(session);
      activeSessions.delete(session);
    });
  });

  console.log('🖥️  Terminal WebSocket server ready at /ws/terminal');
  return wss;
}

function killShell(session: TerminalSession) {
  if (session.shell) {
    try {
      session.shell.stdin?.end();
      session.shell.kill('SIGKILL');
    } catch {
      // Process may already be dead
    }
    session.shell = null;
  }
}

export function cleanupAllSessions() {
  for (const session of activeSessions) {
    killShell(session);
    if (session.ws.readyState === WebSocket.OPEN) {
      session.ws.close();
    }
  }
  activeSessions.clear();
}
