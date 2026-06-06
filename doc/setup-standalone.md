# Standalone Local Development Setup

This guide covers running project services directly on the host without Docker Compose-managed application containers.

Use this path when you need direct debugger access, custom local process control, or faster single-service iteration. For Docker-based hot reload, see [Docker development setup](setup-dev.md). For production-style deployment, see [Production setup](setup.md).

## Prerequisites

Install these tools on the host:

- `pnpm`
- Go
- `uv`
- PostgreSQL 17
- Redis

PostgreSQL and Redis must already be running locally before starting application services.

## Environment

Use the dev environment template as the starting point:

```bash
cp -n docker/.env.dev.example docker/.env
```

If you run services outside Compose, make sure host-facing values are used for PostgreSQL, Redis, and service URLs. The dev template defaults are intended for local development and direct ports.

## Frontend

```bash
cd frontend
pnpm install
pnpm dev
```

Default URL:

```text
http://localhost:3000
```

## Backend API

```bash
cd backend
go run ./cmd/api
```

Default URL:

```text
http://localhost:8080
```

## AI Service

```bash
cd ai-service
uv run uvicorn main:app --reload
```

Default URL:

```text
http://localhost:8000
```

## Browser Worker

```bash
cd browser-worker
go run .
```

Default URL:

```text
http://localhost:8081
```

The browser worker uses Docker to create isolated browser runtime containers. Make sure Docker is running and reachable from the host if you use remote browser sessions.

## Collaboration Service

```bash
cd collab-service
pnpm install
pnpm dev
```

Default URL:

```text
http://localhost:8090
```

## Content Pipeline Service

```bash
cd content-pipeline-service
cargo run -p content-pipeline-service
```

Default gRPC address:

```text
localhost:50051
```

## Browser Extension

```bash
cd extension
pnpm install
pnpm dev
```

Default URL:

```text
http://localhost:3010
```

Load `extension/.output/chrome-mv3-dev` as an unpacked extension in the host browser.
