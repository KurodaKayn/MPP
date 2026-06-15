FROM node:24-alpine AS base

ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable

FROM base AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json frontend/package.json
COPY extension/package.json extension/package.json
COPY collab-service/package.json collab-service/package.json
RUN pnpm install --frozen-lockfile --filter collab-service...

FROM deps AS dev
WORKDIR /app/collab-service
ENV NODE_ENV=development
ENV CI=true
EXPOSE 8090
CMD ["sh", "-c", "pnpm install --frozen-lockfile --filter collab-service... --config.confirmModulesPurge=false --store-dir /pnpm/store && pnpm dev"]

FROM deps AS builder
WORKDIR /app
COPY . .
WORKDIR /app/collab-service
RUN pnpm build

FROM base AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV HOME=/home/app
RUN addgroup -S app -g 10001 && adduser -S app -G app -u 10001 && chown app:app /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json frontend/package.json
COPY extension/package.json extension/package.json
COPY collab-service/package.json collab-service/package.json
RUN pnpm install --prod --frozen-lockfile --filter collab-service...
WORKDIR /app/collab-service
COPY --from=builder --chown=app:app /app/collab-service/dist ./dist
RUN chown -R app:app /app

EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8090/ready >/dev/null || exit 1
USER app
CMD ["node", "dist/server.js"]
