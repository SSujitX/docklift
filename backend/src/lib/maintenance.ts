// Process-wide maintenance gate used during restore so API handlers do not
// race Prisma while the SQLite file is being replaced.

let maintenance = false;
let reason = '';

export function enterMaintenance(why: string): void {
  maintenance = true;
  reason = why;
}

export function exitMaintenance(): void {
  maintenance = false;
  reason = '';
}

export function isMaintenanceMode(): boolean {
  return maintenance;
}

export function maintenanceReason(): string {
  return reason || 'System maintenance in progress';
}
