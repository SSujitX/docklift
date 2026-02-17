// Auth routes - registration, login, logout, session check
import express, { Request, Response, NextFunction } from 'express';
import bcrypt from 'bcrypt';
import jwt from 'jsonwebtoken';
import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import prisma from '../lib/prisma.js';
import { JWT_SECRET, authMiddleware } from '../lib/authMiddleware.js';
import { config } from '../lib/config.js';
const router = express.Router();

const JWT_EXPIRES_IN = '7d';

// Check if setup is complete (any users exist)
router.get('/status', async (req: Request, res: Response) => {
  try {
    const userCount = await prisma.user.count();
    res.json({
      setupComplete: userCount > 0,
      userCount
    });
  } catch (error: any) {
    // If database doesn't exist or table missing, setup is not complete
    // This allows restore from backup on fresh install
    if (error.message?.includes('does not exist') || error.code === 'P2021') {
      return res.json({
        setupComplete: false,
        userCount: 0,
        needsRestore: true
      });
    }
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Register first user (only if no users exist)
router.post('/register', async (req: Request, res: Response) => {
  try {
    const { name, email, password } = req.body;

    // Validate input
    if (!name || !email || !password) {
      return res.status(400).json({ error: 'Name, email, and password are required' });
    }

    // Password validation
    if (password.length < 8) {
      return res.status(400).json({ error: 'Password must be at least 8 characters' });
    }

    // Check if any users already exist
    const userCount = await prisma.user.count();
    if (userCount > 0) {
      return res.status(403).json({ error: 'Setup already complete. Use login instead.' });
    }

    // Hash password
    const hashedPassword = await bcrypt.hash(password, 12);

    // Create user
    const user = await prisma.user.create({
      data: {
        name,
        email: email.toLowerCase(),
        password: hashedPassword,
        role: 'admin',
      },
    });

    // Generate token
    const token = jwt.sign(
      { userId: user.id, email: user.email, role: user.role },
      JWT_SECRET,
      { expiresIn: JWT_EXPIRES_IN }
    );

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

    // Find user
    const user = await prisma.user.findUnique({
      where: { email: email.toLowerCase() },
    });

    if (!user) {
      return res.status(401).json({ error: 'Invalid email or password' });
    }

    // Verify password
    const isValidPassword = await bcrypt.compare(password, user.password);
    if (!isValidPassword) {
      return res.status(401).json({ error: 'Invalid email or password' });
    }

    // Generate token
    const token = jwt.sign(
      { userId: user.id, email: user.email, role: user.role },
      JWT_SECRET,
      { expiresIn: JWT_EXPIRES_IN }
    );

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

// Change password
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

    // Find user to verify current password
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

    // Hash and update new password
    const hashedNewPassword = await bcrypt.hash(newPassword, 12);
    await prisma.user.update({
      where: { id: user.id },
      data: { password: hashedNewPassword },
    });

    res.json({ message: 'Password changed successfully' });
  } catch (error: any) {
    console.error('Password change error:', error);
    res.status(500).json({ error: 'Failed to change password' });
  }
});

// ========================================
// Setup Token (for restore-upload on fresh install)
// ========================================

// GET /api/auth/setup-token - Get or generate a one-time setup token
// Only works when no users exist (fresh install scenario)
router.get('/setup-token', async (req: Request, res: Response) => {
  try {
    const userCount = await prisma.user.count();
    if (userCount > 0) {
      return res.status(403).json({ error: 'Setup already complete. Setup tokens are only available before first user registration.' });
    }

    const dataDir = config.dataPath || './data';
    const tokenPath = path.join(dataDir, '.setup-token');

    // Generate token if it doesn't exist
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
// SSE Token (short-lived tokens for SSE connections)
// ========================================

// POST /api/auth/sse-token - Issue a short-lived JWT for SSE/streaming connections
// This prevents the main JWT from appearing in URLs (server logs, browser history, etc.)
router.post('/sse-token', authMiddleware, async (req: Request, res: Response) => {
  try {
    const jwtSecret = await JWT_SECRET;
    const user = (req as any).user;

    const sseToken = jwt.sign(
      { userId: user?.userId || 'system', purpose: 'sse' },
      jwtSecret,
      { expiresIn: '5m' }
    );

    res.json({ token: sseToken });
  } catch (error: any) {
    res.status(500).json({ error: 'Failed to generate SSE token' });
  }
});

export default router;
