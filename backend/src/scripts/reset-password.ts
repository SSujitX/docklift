// CLI script to reset admin password
// Usage: docker exec -it docklift-backend node dist/scripts/reset-password.js

import { PrismaClient } from '@prisma/client';
import bcrypt from 'bcrypt';
import crypto from 'crypto';

const prisma = new PrismaClient();

async function resetPassword() {
  console.log('\n🔐 Docklift Password Reset\n');

  try {
    // Find the first admin user
    const user = await prisma.user.findFirst({
      where: { role: 'admin' },
    });

    if (!user) {
      console.log('❌ No admin user found. Please run setup first.\n');
      process.exit(1);
    }

    // Generate a random password
    const newPassword = crypto.randomBytes(12).toString('base64').slice(0, 16);

    // Hash the password
    const hashedPassword = await bcrypt.hash(newPassword, 12);

    // Update the user's password
    await prisma.user.update({
      where: { id: user.id },
      data: { password: hashedPassword },
    });

    console.log('✅ Password reset successful!\n');
    console.log(`   User:     ${user.email}`);
    console.log(`   Password: ${newPassword}\n`);
    console.log('⚠️  Please change this password after logging in.\n');

  } catch (error: any) {
    console.error('❌ Error resetting password:', error.message);
    process.exit(1);
  } finally {
    await prisma.$disconnect();
  }
}

resetPassword();
