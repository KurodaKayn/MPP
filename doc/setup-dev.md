# Docker Development Setup

This guide covers the Docker-based development workflow with hot reload. It keeps service startup consistent with production Compose while exposing direct local ports for day-to-day development.

For production-style deployment, see [Production setup](setup.md). For running services directly on the host, see [Standalone local development setup](setup-standalone.md).

## Start With Hot Reload

Run from the repository root:

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml watch
```

The dev Compose project name is `mpp-dev`.

This mode starts the frontend, backend, AI service, browser-worker, content pipeline service, collaboration service, PostgreSQL, and Redis. It keeps direct local ports open:

- Frontend: [http://localhost:3000](http://localhost:3000)
- Backend API: [http://localhost:8080/ping](http://localhost:8080/ping)
- AI service: [http://localhost:8000](http://localhost:8000)
- Browser worker: [http://localhost:8081](http://localhost:8081)
- Extension WXT dev server: [http://localhost:3010](http://localhost:3010), only with the `extension` profile
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`

## Reload Behavior

- The frontend runs `pnpm dev`; source changes trigger Next.js hot reload.
- Frontend `.next` is stored in a Docker named volume. Next.js 16 Turbopack filesystem cache is written into `mpp-dev_frontend_next` to speed up rebuilds after container restarts.
- The backend runs with `air`; Go source changes rebuild and restart the API.
- The browser worker runs with the Go dev target and rebuilds when source changes.
- The AI service runs with `uvicorn --reload`; Python source changes reload the server.
- Dependency file changes trigger service rebuilds, including `package.json`, `pnpm-lock.yaml`, `go.mod`, `go.sum`, `pyproject.toml`, and `uv.lock`.

## Start In The Background

If you want dev containers running in the background without Compose watch rebuilds:

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up -d
```

Source hot reload still works, but dependency file changes will not automatically trigger Compose rebuilds.

## Frontend Dev Cache

Inspect or clean the frontend dev cache volume:

```bash
script/docker/dev-cache.sh status
script/docker/dev-cache.sh clean-frontend-next
```

`clean-frontend-next` removes only the dev cache inside `.next` and keeps the volume. To fully recreate the frontend `.next` volume:

```bash
script/docker/dev-cache.sh reset-frontend-next
```

Disable the Next.js dev filesystem cache with:

```env
MPP_FRONTEND_TURBOPACK_FS_CACHE=false
```

## Browser Extension Dev Server

The browser extension dev service is an optional Compose profile. It is not started by the default dev workflow.

Start it with:

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml --profile extension up extension-dev
```

This runs the WXT dev server from the `extension` directory. Docker dev mode disables the container browser runner, so load `extension/.output/chrome-mv3-dev` as an unpacked extension in the host browser.

Change the extension dev port with:

```env
EXTENSION_DEV_PORT=3010
```

## Optional Dev Gateway

If you want to test Traefik while keeping direct dev ports available, start only the dev gateway probe:

```bash
script/docker/dev-traefik.sh up
```

This starts only the `traefik` service. It does not start or rebuild frontend, backend, AI, database, or Redis services, and it does not remove direct local ports.

Default dev gateway entrypoints:

- HTTP gateway: [http://localhost:8088](http://localhost:8088)
- HTTPS gateway: [https://localhost:8443](https://localhost:8443)

Useful commands:

```bash
script/docker/dev-traefik.sh logs
script/docker/dev-traefik.sh restart
script/docker/dev-traefik.sh stop
```

Override gateway ports with `TRAEFIK_HTTP_PORT` or `TRAEFIK_HTTPS_PORT`.

Dev mode does not use the Let's Encrypt or manual certificate overrides unless you explicitly include those Compose files.
