// Prisma database client singleton - prevents multiple instances in development.
// Uses a Proxy so reconnectPrisma() swaps the underlying client without breaking
// existing `import prisma from ...` bindings.
import { PrismaClient } from '@prisma/client';

const globalForPrisma = globalThis as unknown as {
  prismaClient: PrismaClient | undefined;
};

function createClient(): PrismaClient {
  return new PrismaClient();
}

let client: PrismaClient = globalForPrisma.prismaClient ?? createClient();

if (process.env.NODE_ENV !== 'production') {
  globalForPrisma.prismaClient = client;
}

export const prisma: PrismaClient = new Proxy({} as PrismaClient, {
  get(_target, prop, _receiver) {
    const value = Reflect.get(client, prop, client);
    return typeof value === 'function' ? value.bind(client) : value;
  },
});

/** Drop the open connection and open a fresh client (required after replacing the SQLite file). */
export async function reconnectPrisma(): Promise<PrismaClient> {
  try {
    await client.$disconnect();
  } catch {
    /* already disconnected */
  }
  client = createClient();
  if (process.env.NODE_ENV !== 'production') {
    globalForPrisma.prismaClient = client;
  }
  await client.$connect();
  return client;
}

export default prisma;
