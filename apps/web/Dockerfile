# syntax=docker/dockerfile:1.6

# ---- deps stage ----
# Separate dependency installation so `npm ci` is cached as long as
# package.json / package-lock.json are unchanged.
FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

# ---- builder stage ----
FROM node:20-alpine AS builder
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_ENV=production

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# next.config.js uses `output: 'standalone'`, which produces a self-contained
# server.js and pruned node_modules in `.next/standalone/`.
RUN --mount=type=cache,target=/root/.npm \
    npm run build

# ---- runner stage ----
FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# Next.js standalone output already includes a minimal node_modules tree.
# We only need the server entry, the static asset folders, and public/.
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

# Install wget for the compose healthcheck and run as non-root.
RUN apk add --no-cache wget && \
    addgroup -S -g 10001 nextjs && \
    adduser  -S -u 10001 -G nextjs nextjs && \
    chown -R nextjs:nextjs /app

USER nextjs

EXPOSE 3000

# Standalone build exposes a single server.js entrypoint.
CMD ["node", "server.js"]
