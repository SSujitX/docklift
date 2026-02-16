# Install dependencies with Bun (fast)
FROM oven/bun:1-alpine AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

# Build with Node.js (stable — Bun crashes with SIGILL on some server CPUs)
FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1

# Ensure public folder exists
RUN mkdir -p public

RUN npm run build

# Production runtime
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

# Create public folder if it doesn't exist
RUN mkdir -p public

COPY --from=builder /app/public ./public

RUN mkdir .next
RUN chown nextjs:nodejs .next

COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
