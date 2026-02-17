# Base stage - Use Bun for faster installs
FROM oven/bun:1-alpine AS base

# Dependencies stage
FROM base AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json bun.lock* ./
COPY prisma ./prisma/
RUN bun install --frozen-lockfile
RUN bunx prisma generate --no-hints

# Production dependencies stage
FROM base AS prod-deps
WORKDIR /app
COPY package.json bun.lock* ./
COPY prisma ./prisma/
RUN bun install --production --frozen-lockfile
RUN bunx prisma generate --no-hints

# Builder stage
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

RUN bun run build

# Runner stage - Use Node for production runtime
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV PORT=8000

# Install Docker CLI, Git, and procps (for accurate ps/top)
RUN apk add --no-cache docker-cli docker-cli-compose git procps bash
RUN npm install -g npm@latest

# Create non-root user
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 docklift

# Create necessary directories
RUN mkdir -p /app/data /deployments /nginx-conf
RUN chown -R docklift:nodejs /app/data /deployments /nginx-conf

# Copy built files
COPY --from=prod-deps --chown=docklift:nodejs /app/node_modules ./node_modules
COPY --from=builder --chown=docklift:nodejs /app/dist ./dist
COPY --from=builder --chown=docklift:nodejs /app/prisma ./prisma
COPY --from=builder --chown=docklift:nodejs /app/package.json ./

# Note: Running as root is required for Docker socket access
# USER docklift

EXPOSE 8000

ENV HOSTNAME="0.0.0.0"
ENV DATABASE_URL="file:/app/data/docklift.db"

# Database auto-creation: prisma db push creates the SQLite file if it doesn't exist
CMD ["sh", "-c", "npx prisma db push --skip-generate && node dist/index.js"]
