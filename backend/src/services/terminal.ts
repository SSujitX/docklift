// Terminal service - WebSocket-based interactive shell (zero native dependencies)
import { WebSocketServer, WebSocket } from 'ws';
import { Server as HttpServer, IncomingMessage } from 'http';
import { spawn, ChildProcess } from 'child_process';
import jwt from 'jsonwebtoken';
import bcrypt from 'bcrypt';
import { JWT_SECRET } from '../lib/authMiddleware.js';
import { PrismaClient } from '@prisma/client';

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

// Message types from client
interface ClientMessage {
  type: 'auth' | 'input' | 'resize';
  password?: string;
  data?: string;
  cols?: number;
  rows?: number;
}

function verifyToken(token: string): { userId: string; email: string } | null {
  try {
    const decoded = jwt.verify(token, JWT_SECRET) as any;
    return { userId: decoded.userId || decoded.id, email: decoded.email };
  } catch {
    return null;
  }
}

export function setupTerminalWebSocket(server: HttpServer) {
  const wss = new WebSocketServer({ noServer: true });

  // Handle HTTP upgrade requests for /ws/terminal
  server.on('upgrade', (request: IncomingMessage, socket, head) => {
    const url = new URL(request.url || '', `http://${request.headers.host}`);
    
    if (url.pathname !== '/ws/terminal') {
      socket.destroy();
      return;
    }

    // Extract JWT from query string
    const token = url.searchParams.get('token');
    if (!token) {
      socket.write('HTTP/1.1 401 Unauthorized\r\n\r\n');
      socket.destroy();
      return;
    }

    const user = verifyToken(token);
    if (!user) {
      socket.write('HTTP/1.1 401 Unauthorized\r\n\r\n');
      socket.destroy();
      return;
    }

    // Upgrade connection
    wss.handleUpgrade(request, socket, head, (ws) => {
      wss.emit('connection', ws, request, user);
    });
  });

  wss.on('connection', (ws: WebSocket, _request: IncomingMessage, user: { userId: string; email: string }) => {
    // Enforce max concurrent sessions
    if (activeSessions.size >= MAX_CONCURRENT_SESSIONS) {
      console.warn(`[TERMINAL] Max concurrent sessions (${MAX_CONCURRENT_SESSIONS}) reached — rejecting user: ${user.email}`);
      ws.send(JSON.stringify({ type: 'auth_error', message: 'Too many active terminal sessions. Close one first.' }));
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

    // Send welcome — client must send password to authenticate
    ws.send(JSON.stringify({ type: 'auth_required' }));

    ws.on('message', async (rawData) => {
      try {
        const msg: ClientMessage = JSON.parse(rawData.toString());

        // Handle authentication (password verification)
        if (msg.type === 'auth') {
          if (session.authenticated) return;

          // Brute-force protection
          if (session.authAttempts >= MAX_AUTH_ATTEMPTS) {
            console.warn(`[TERMINAL] Too many failed auth attempts for user: ${user.email}`);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Too many failed attempts. Reconnect to try again.' }));
            ws.close();
            return;
          }
          
          if (!msg.password) {
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Password required' }));
            return;
          }

          const dbUser = await prisma.user.findFirst();
          if (!dbUser) {
            ws.send(JSON.stringify({ type: 'auth_error', message: 'No user account found' }));
            return;
          }

          const valid = await bcrypt.compare(msg.password, dbUser.password);
          if (!valid) {
            session.authAttempts++;
            console.warn(`[TERMINAL] Failed auth attempt ${session.authAttempts}/${MAX_AUTH_ATTEMPTS} for user: ${user.email}`);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Invalid password' }));
            return;
          }

          session.authenticated = true;
          console.log(`[TERMINAL] User "${user.email}" authenticated for interactive terminal`);

          // Spawn shell using 'script' for PTY emulation (works on Alpine, no native deps)
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
                // Colorful prompt: [green]root@docklift[reset]:[blue]~[reset]#
                PS1: '\\[\\e[1;32m\\]\\u@\\h\\[\\e[0m\\]:\\[\\e[1;34m\\]\\w\\[\\e[0m\\]# ',
              },
              cwd: '/root',
              stdio: ['pipe', 'pipe', 'pipe'],
            });

            session.shell = shell;

            // Stream shell output to WebSocket
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

            // Start idle timeout — auto-kill after 30min of no input
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

            // Attach idle reset to session for input handler
            (session as any)._resetIdleTimer = resetIdleTimer;
          } catch (err) {
            console.error('[TERMINAL] Failed to spawn shell:', err);
            ws.send(JSON.stringify({ type: 'auth_error', message: 'Failed to start terminal session' }));
          }
          return;
        }

        // All other messages require authentication
        if (!session.authenticated || !session.shell) return;

        // Handle terminal input
        if (msg.type === 'input' && msg.data) {
          session.shell.stdin?.write(msg.data);
          // Reset idle timer on input
          if ((session as any)._resetIdleTimer) (session as any)._resetIdleTimer();
        }

        // Handle terminal resize (send SIGWINCH-style resize via stty)
        if (msg.type === 'resize' && msg.cols && msg.rows) {
          // SECURITY: Validate cols/rows are safe integers to prevent command injection
          const cols = Number(msg.cols);
          const rows = Number(msg.rows);
          if (Number.isInteger(cols) && Number.isInteger(rows)
              && cols > 0 && cols < 500 && rows > 0 && rows < 500) {
            session.shell.stdin?.write(`stty cols ${cols} rows ${rows}\n`);
          }
        }
      } catch (err) {
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

// Cleanup all sessions (called on server shutdown)
export function cleanupAllSessions() {
  for (const session of activeSessions) {
    killShell(session);
    if (session.ws.readyState === WebSocket.OPEN) {
      session.ws.close();
    }
  }
  activeSessions.clear();
}
