// Global restore lock — acquire BEFORE Multer writes any upload bytes.

import fs from 'fs';
import path from 'path';
import { config } from './config.js';

let locked = false;
let reason = '';

function criticalMarkerExists(): boolean {
  try {
    return fs.existsSync(path.join(config.dataPath, '.restore-critical'));
  } catch {
    return false;
  }
}

export function tryAcquireRestoreLock(why: string): boolean {
  // Critical seal permanently blocks new restores until operator clear
  if (criticalMarkerExists()) return false;
  if (locked) return false;
  locked = true;
  reason = why;
  return true;
}

/** Hold the lock without checking prior state (critical recovery seal). */
export function forceRestoreLock(why: string): void {
  locked = true;
  reason = why;
}

export function releaseRestoreLock(): void {
  // Never drop the lock while a critical seal is on disk
  if (criticalMarkerExists()) {
    locked = true;
    reason = reason || 'restore-critical';
    return;
  }
  locked = false;
  reason = '';
}

export function isRestoreLocked(): boolean {
  return locked || criticalMarkerExists();
}

export function restoreLockReason(): string {
  if (criticalMarkerExists()) {
    return 'CRITICAL restore recovery required — clear .restore-critical after manual repair';
  }
  return reason || 'A restore is already in progress';
}
