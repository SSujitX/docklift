// Auth routes - registration, login, logout, session check
import express, { Request, Response } from 'express';
import bcrypt from 'bcrypt';
import jwt from 'jsonwebtoken';
import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import prisma from '../lib/prisma.js';
import { JWT_SECRET, authMiddleware } from '../lib/authMiddleware.js';
import { config } from '../lib/config.js';
import {
  ensureBootstrapSecret,
  verifyBootstrapSecret,
  consumeBootstrapSecret,
} from '../lib/bootstrap.js';

const router = express.Router();

const JWT_EXPIRES_IN = '7d';

function requireBootstrap(req: Request, res: Response): boolean {
  const header = (req.headers['x-bootstrap-secret'] as string | undefined)?.trim();
  const bodySecret = typeof req.body?.bootstrapSecret === 'string' ? req.body.bootstrapSecret.trim() : undefined;
  const provided = header || bodySecret;
  if (!verifyBootstrapSecret(provided)) {
    res.status(403).json({
      error: 'Bootstrap secret required. Copy it from the backend logs (or data/.bootstrap-secret on the host).',
    });
    return false;
  }
  return true;
}

function signSessionToken(user: { id: string; email: string; role: string }) {
  return jwt.sign(
    { userId: user.id, email: user.email, role: user.role },
    JWT_SECRET,
    { expiresIn: JWT_EXPIRES_IN }
  );
}

// Check if setup is complete (any users exist)
router.get('/status', async (req: Request, res: Response) => {
  try {
    const userCount = await prisma.user.count();
    // Ensure secret exists for fresh installs (does not expose it)
    if (userCount === 0) {
      ensureBootstrapSecret();
    }
    res.json({
      setupComplete: userCount > 0,
      userCount,
      bootstrapRequired: userCount === 0,
    });
  } catch (error: any) {
    // If database doesn't exist or table missing, setup is not complete
    if (error.message?.includes('does not exist') || error.code === 'P2021') {
      ensureBootstrapSecret();
      return res.json({
        setupComplete: false,
        userCount: 0,
        needsRestore: true,
        bootstrapRequired: true,
      });
    }
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Register first user (only if no users exist) — requires bootstrap secret
router.post('/register', async (req: Request, res: Response) => {
  try {
    const { name, email, password } = req.body;

    if (!name || !email || !password) {
      return res.status(400).json({ error: 'Name, email, and password are required' });
    }

    if (password.length < 8) {
      return res.status(400).json({ error: 'Password must be at least 8 characters' });
    }

    const userCount = await prisma.user.count();
    if (userCount > 0) {
      return res.status(403).json({ error: 'Setup already complete. Use login instead.' });
    }

    if (!requireBootstrap(req, res)) return;

    const hashedPassword = await bcrypt.hash(password, 12);
    const now = new Date();

    const user = await prisma.user.create({
      data: {
        name,
        email: email.toLowerCase(),
        password: hashedPassword,
        role: 'admin',
        passwordChangedAt: now,
      },
    });

    consumeBootstrapSecret();

    const token = signSessionToken(user);

    res.status(201).json({
      message: 'Registration successful',
      token,
      user: {
        id: user.id,
        name: user.name,
        email: user.email,
        role: user.role,
      },
    });
  } catch (error: any) {
    if (error.code === 'P2002') {
      return res.status(400).json({ error: 'Email already exists' });
    }
    console.error('Registration error:', error);
    res.status(500).json({ error: 'Registration failed' });
  }
});

// Login
router.post('/login', async (req: Request, res: Response) => {
  try {
    const { email, password } = req.body;

    if (!email || !password) {
      return res.status(400).json({ error: 'Email and password are required' });
    }

    const user = await prisma.user.findUnique({
      where: { email: email.toLowerCase() },
    });

    if (!user) {
      return res.status(401).json({ error: 'Invalid email or password' });
    }

    const isValidPassword = await bcrypt.compare(password, user.password);
    if (!isValidPassword) {
      return res.status(401).json({ error: 'Invalid email or password' });
    }

    const token = signSessionToken(user);

    res.json({
      message: 'Login successful',
      token,
      user: {
        id: user.id,
        name: user.name,
        email: user.email,
        role: user.role,
      },
    });
  } catch (error: any) {
    console.error('Login error:', error);
    res.status(500).json({ error: 'Login failed' });
  }
});

// Get current user (requires auth)
router.get('/me', authMiddleware, async (req: Request, res: Response) => {
  try {
    const authUser = (req as any).user;

    const user = await prisma.user.findUnique({
      where: { id: authUser.userId },
      select: {
        id: true,
        name: true,
        email: true,
        role: true,
        created_at: true,
      },
    });

    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    res.json({ user });
  } catch (error: any) {
    console.error('Get user error:', error);
    res.status(500).json({ error: 'Failed to get user' });
  }
});

// Update profile (name, email)
router.patch('/profile', authMiddleware, async (req: Request, res: Response) => {
  try {
    const { name, email } = req.body;
    const authUser = (req as any).user;

    if (!name || !email) {
      return res.status(400).json({ error: 'Name and email are required' });
    }

    const updatedUser = await prisma.user.update({
      where: { id: authUser.userId },
      data: {
        name,
        email: email.toLowerCase(),
      },
      select: {
        id: true,
        name: true,
        email: true,
        role: true,
      },
    });

    res.json({ message: 'Profile updated', user: updatedUser });
  } catch (error: any) {
    if (error.code === 'P2002') {
      return res.status(400).json({ error: 'Email already exists' });
    }
    console.error('Profile update error:', error);
    res.status(500).json({ error: 'Failed to update profile' });
  }
});

// Change password — invalidates existing JWTs via passwordChangedAt
router.post('/change-password', authMiddleware, async (req: Request, res: Response) => {
  try {
    const { currentPassword, newPassword } = req.body;
    const authUser = (req as any).user;

    if (!currentPassword || !newPassword) {
      return res.status(400).json({ error: 'Current and new passwords are required' });
    }

    if (newPassword.length < 8) {
      return res.status(400).json({ error: 'New password must be at least 8 characters' });
    }

    const user = await prisma.user.findUnique({
      where: { id: authUser.userId },
    });

    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    const isValid = await bcrypt.compare(currentPassword, user.password);
    if (!isValid) {
      return res.status(400).json({ error: 'Invalid current password' });
    }

    const hashedNewPassword = await bcrypt.hash(newPassword, 12);
    const now = new Date();
    const updated = await prisma.user.update({
      where: { id: user.id },
      data: { password: hashedNewPassword, passwordChangedAt: now },
    });

    // Issue a fresh session so the current client stays logged in
    const token = signSessionToken(updated);

    res.json({ message: 'Password changed successfully', token });
  } catch (error: any) {
    console.error('Password change error:', error);
    res.status(500).json({ error: 'Failed to change password' });
  }
});

// ========================================
// Setup Token (for restore-upload on fresh install)
// ========================================

// GET /api/auth/setup-token — requires bootstrap secret (not a public dump)
router.get('/setup-token', async (req: Request, res: Response) => {
  try {
    const userCount = await prisma.user.count();
    if (userCount > 0) {
      return res.status(403).json({ error: 'Setup already complete. Setup tokens are only available before first user registration.' });
    }

    if (!requireBootstrap(req, res)) return;

    const dataDir = config.dataPath || './data';
    const tokenPath = path.join(dataDir, '.setup-token');

    if (!fs.existsSync(tokenPath)) {
      fs.mkdirSync(dataDir, { recursive: true });
      const token = crypto.randomBytes(32).toString('hex');
      fs.writeFileSync(tokenPath, token, { mode: 0o600 });
    }

    const token = fs.readFileSync(tokenPath, 'utf8').trim();
    res.json({ setupToken: token });
  } catch (error: any) {
    res.status(500).json({ error: 'Failed to generate setup token' });
  }
});

// ========================================
// Short-lived purpose tokens (SSE / terminal query params)
// ========================================

router.post('/sse-token', authMiddleware, async (req: Request, res: Response) => {
  try {
    const user = (req as any).user;

    const sseToken = jwt.sign(
      {
        userId: user?.userId,
        email: user?.email,
        role: user?.role,
        purpose: 'sse',
      },
      JWT_SECRET,
      { expiresIn: '5m' }
    );

    res.json({ token: sseToken });
  } catch (error: any) {
    res.status(500).json({ error: 'Failed to generate SSE token' });
  }
});

// POST /api/auth/terminal-token — short-lived WS upgrade token (not the 7d session JWT)
router.post('/terminal-token', authMiddleware, async (req: Request, res: Response) => {
  try {
    const user = (req as any).user;

    const terminalToken = jwt.sign(
      {
        userId: user?.userId,
        email: user?.email,
        role: user?.role,
        purpose: 'terminal',
      },
      JWT_SECRET,
      { expiresIn: '5m' }
    );

    res.json({ token: terminalToken });
  } catch (error: any) {
    res.status(500).json({ error: 'Failed to generate terminal token' });
  }
});

export default router;
