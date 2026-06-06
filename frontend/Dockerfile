ARG FRONTEND_BASE_URL=http://127.0.0.1:3000
ARG BACKEND_API_BASE_URL=http://localhost:8080

FROM node:24-alpine AS base

# Install pnpm
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable

FROM base AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM base AS dev
WORKDIR /app
ENV NODE_ENV=development
ENV CI=true
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
EXPOSE 3000
CMD ["sh", "-c", "pnpm install --frozen-lockfile --config.confirmModulesPurge=false --store-dir /pnpm/store && pnpm dev --hostname 0.0.0.0"]

FROM base AS builder
WORKDIR /app
ARG FRONTEND_BASE_URL
ARG BACKEND_API_BASE_URL
ENV FRONTEND_BASE_URL=$FRONTEND_BASE_URL
ENV BACKEND_API_BASE_URL=$BACKEND_API_BASE_URL
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm run build

FROM base AS runner
WORKDIR /app
ARG FRONTEND_BASE_URL
ARG BACKEND_API_BASE_URL
ENV NODE_ENV=production
ENV FRONTEND_BASE_URL=$FRONTEND_BASE_URL
ENV BACKEND_API_BASE_URL=$BACKEND_API_BASE_URL
ENV HOME=/home/app
RUN addgroup -S app -g 10001 && adduser -S app -G app -u 10001 && chown app:app /app
COPY --from=builder --chown=app:app /app/public ./public
COPY --from=builder --chown=app:app /app/.next/standalone ./
COPY --from=builder --chown=app:app /app/.next/static ./.next/static

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:3000/api/ready >/dev/null || exit 1
USER app
CMD ["node", "server.js"]
