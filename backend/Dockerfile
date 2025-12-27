# Base stage
FROM node:22-alpine AS base

# Dependencies stage
FROM base AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json package-lock.json* ./
COPY prisma ./prisma/
RUN npm ci
RUN npx prisma generate

# Production dependencies stage
FROM base AS prod-deps
WORKDIR /app
COPY package.json package-lock.json* ./
COPY prisma ./prisma/
RUN npm ci --omit=dev
RUN npx prisma generate

# Builder stage
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

RUN npm run build

# Runner stage
FROM base AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV PORT=8000

# Install Docker CLI and Git for managing deployments
RUN apk add --no-cache docker-cli docker-cli-compose git

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
