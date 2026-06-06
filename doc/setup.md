# Production Setup Guide

This guide covers the production-style Docker Compose deployment path. It runs the full stack behind Traefik and exposes only the gateway HTTP and HTTPS ports by default.

For development workflows, see:

- [Docker development setup](setup-dev.md)
- [Standalone local development setup](setup-standalone.md)

## Quick Start

Start all services with Docker Compose:

```bash
cd docker

# Create the gateway/deploy environment file. Stop if the existing file is a dev env,
# because mixing dev and production settings is easy to miss.
if [ -f .env ] && grep -q '^APP_ENV=development$' .env; then
  echo "docker/.env is currently a dev env. Back it up or remove it before copying .env.deploy.example." >&2
  exit 1
fi
cp -n .env.deploy.example .env

# Start the stack. Without a TLS override, Traefik uses its default self-signed TLS behavior.
# For production, choose one of the real certificate options below.
docker compose up -d

# Follow logs.
docker compose logs -f
```

The default Compose project name is `mpp`. This mode starts Traefik as the gateway and exposes only gateway ports on the host by default:

- Web workspace: [http://localhost](http://localhost)
- HTTPS entrypoint: [https://localhost](https://localhost)

`frontend`, `backend`, `ai-service`, `browser-worker`, `collab-service`, PostgreSQL, and Redis run as internal Compose services. If host ports `80` or `443` are already in use, set `TRAEFIK_HTTP_PORT` or `TRAEFIK_HTTPS_PORT` in `docker/.env`, for example:

```env
TRAEFIK_HTTP_PORT=8088
```

The default HTTPS entrypoint is suitable only as a local smoke test. For production, choose either Let's Encrypt automatic certificates or manual certificates.

## Public URLs

For production or gateway deployments, set public URLs to the real external origin:

```env
FRONTEND_BASE_URL=https://your-domain.example
COLLAB_WEBSOCKET_URL_BASE=wss://your-domain.example
X_OAUTH2_REDIRECT_URL=https://your-domain.example/api/user/dashboard/settings/x/oauth2/callback
```

## Rate Limits

The Traefik gateway enables IP-level rate limiting with:

```env
TRAEFIK_RATE_LIMIT_AVERAGE=100
TRAEFIK_RATE_LIMIT_PERIOD=1s
TRAEFIK_RATE_LIMIT_BURST=200
TRAEFIK_RATE_LIMIT_REDIS_ENDPOINTS=redis:6379
```

The backend also enables Redis-backed application quotas under `/api/user/dashboard`. These quotas cover general user and tenant limits, per-route limits, AI usage, publish jobs, and browser session limits. Application quota behavior is driven by the backend `rate_limits.yml` matrix; environment variables only keep the global enable flag and key prefix.

## HTTPS Certificates

TLS certificate behavior is selected with Compose override files:

- Base stack: `docker-compose.yml`
- Let's Encrypt automatic certificates: `docker-compose.tls-letsencrypt.yml`
- Manual certificates: `docker-compose.tls-manual.yml`

This is configuration-driven deployment rather than a code-level factory pattern. The base Compose file owns the service topology, and each TLS override selects one certificate strategy.

### Option A: Let's Encrypt Automatic Certificates

Use this when a public domain points directly to this server.

Prerequisites:

- The domain A/AAAA record points to the server public IP.
- Public ports `80` and `443` are reachable by Let's Encrypt.
- `docker/.env` contains the real domain, ACME email, and public URLs.

Key environment variables:

```env
TRAEFIK_CERT_DOMAIN=your-domain.example
TRAEFIK_ACME_EMAIL=admin@your-domain.example
TRAEFIK_ACME_CA_SERVER=https://acme-v02.api.letsencrypt.org/directory
FRONTEND_BASE_URL=https://your-domain.example
COLLAB_WEBSOCKET_URL_BASE=wss://your-domain.example
X_OAUTH2_REDIRECT_URL=https://your-domain.example/api/user/dashboard/settings/x/oauth2/callback
```

Start the stack:

```bash
cd docker
docker compose -f docker-compose.yml -f docker-compose.tls-letsencrypt.yml up -d
docker compose -f docker-compose.yml -f docker-compose.tls-letsencrypt.yml logs -f traefik
```

To test the ACME flow first, temporarily use the Let's Encrypt staging directory:

```env
TRAEFIK_ACME_CA_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory
```

### Option B: Manual Certificates

Use this when certificates are issued by an external provider, internal CA, enterprise CA, or CDN/DNS-side process.

Place certificate files here:

```text
docker/traefik/certs/fullchain.pem
docker/traefik/certs/privkey.pem
```

You can also store certificates outside the repository and point Traefik at that directory:

```env
TRAEFIK_MANUAL_CERT_DIR=/absolute/path/to/certs
```

The target directory must contain:

```text
fullchain.pem
privkey.pem
```

Start the stack:

```bash
cd docker
docker compose -f docker-compose.yml -f docker-compose.tls-manual.yml up -d
docker compose -f docker-compose.yml -f docker-compose.tls-manual.yml logs -f traefik
```

After replacing manual certificates, restart Traefik:

```bash
cd docker
docker compose -f docker-compose.yml -f docker-compose.tls-manual.yml restart traefik
```

## Environment Variables

All production-style Compose environment variables are managed in `docker/.env`.

- Deploy template: `docker/.env.deploy.example`
- Default public frontend origin: `https://your-domain.example`
- AI provider key: `LLM_PROVIDER_KEY`
- Database connection settings: `DB_*`
- Backend horizontal scaling: `BACKEND_API_REPLICAS`, defaulting to `2` in the deploy template
- PostgreSQL connection pool controls: `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, and `DB_CONN_MAX_IDLE_TIME`
- Slow query logging threshold: `DB_SLOW_QUERY_THRESHOLD`

For database query plan auditing, see [database-query-governance.md](database-query-governance.md).
