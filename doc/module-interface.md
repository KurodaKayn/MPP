# MPP Module External Interfaces and Runtime Parameters

This document describes the external interfaces, authentication methods, key request parameters, and runtime environment parameters for the runnable modules in this repository. It is intended for integration and deployment work. It does not replace these source-of-truth files:

- REST/shared type contract: `contracts/openapi.yaml`
- Content Pipeline gRPC contract: `content-pipeline-service/proto/mpp/contentpipeline/v1/content_pipeline.proto`
- Environment templates: `contracts/env.schema.yaml`, `docker/.env.dev.example`, `docker/.env.deploy.example`
- Module responsibility design: `doc/module-design.md`

## 1. Module Overview

| Module | Runtime | Default address | External interface type | Main dependencies |
| --- | --- | --- | --- | --- |
| `frontend` | Next.js | `http://localhost:3000` | Pages, Next API proxy, browser stream rewrites | `backend` |
| `backend` | Go Echo | `http://localhost:8080` | REST API, internal media resolver, health checks, metrics | PostgreSQL, Redis optional or required, AI, Browser Worker, Collab, Content Pipeline |
| `publish-worker` | Go worker | `http://localhost:8080` health server | Health checks, metrics, Redis queue consumer | PostgreSQL, Redis |
| `ai-service` | FastAPI | `http://localhost:8000` | Internal REST API, streaming text responses, metrics | OpenAI-compatible LLM provider |
| `browser-worker` | Go Echo | `http://localhost:8081` | Internal REST API, browser stream proxy, metrics | Redis, Docker or Kubernetes runtime |
| `collab-service` | Fastify + Hocuspocus | `http://localhost:8090` / `ws://localhost:8090` | Internal REST API, collaborative editing WebSocket, metrics | PostgreSQL, optional Redis pub/sub |
| `content-pipeline-service` | Rust tonic | `localhost:50051` | gRPC, gRPC health/reflection, HTTP metrics | Optional backend media resolver |
| `extension` | WXT browser extension | `http://localhost:3010` dev server | Extension UI, content scripts, backend REST calls | `frontend`, `backend` |

## 2. Common Conventions

### 2.1 Authentication

| Interface scope | Authentication method |
| --- | --- |
| Backend user, workspace, and collab REST APIs | `Authorization: Bearer <web-jwt>` |
| Backend admin API | `Authorization: Bearer <web-jwt>`, with JWT role `admin` |
| Backend internal media resolver | Header `X-MPP-Internal-Token: <CONTENT_PIPELINE_INTERNAL_TOKEN>` |
| AI Service internal API | `Authorization: Bearer <AI_SERVICE_INTERNAL_TOKEN>` |
| Browser Worker internal API | `Authorization: Bearer <BROWSER_WORKER_INTERNAL_TOKEN>` |
| Collab Service internal initialization API | `Authorization: Bearer <COLLAB_TOKEN_SECRET>` |
| Collab WebSocket | query parameter `token=<collab-session-jwt>` |

Exception: backend `/api/user/dashboard/extension/events` is an extension publish callback endpoint. It does not use the web JWT; it authenticates with the `token` field in the request body.

### 2.2 Health Checks And Observability

Most HTTP services expose:

| Path | Meaning |
| --- | --- |
| `GET /health` | Process liveness check |
| `GET /ready` | Dependency readiness check |
| `GET /metrics` | Prometheus metrics |

`content-pipeline-service` uses gRPC for business traffic. It also provides gRPC health/reflection and exposes HTTP `GET /metrics` on `CONTENT_PIPELINE_METRICS_ADDR`.

### 2.3 Trace Headers

The frontend proxy and service observability middleware use:

- `x-request-id`
- `x-trace-id`

If a request does not include these headers, the service generates a new trace id and writes it back to the response headers.

## 3. Frontend

### 3.1 External Interfaces

The frontend is the user-facing entry point and proxies backend requests through Next API routes.

| Path | Method | Description |
| --- | --- | --- |
| `/{locale}` | `GET` | Home page. Current locales include `zh` and `en` |
| `/{locale}/login` | `GET` | Login page |
| `/{locale}/dashboard` | `GET` | Dashboard home |
| `/{locale}/dashboard/content` | `GET` | Content project list |
| `/{locale}/dashboard/content/{projectId}` | `GET` | Content editor |
| `/{locale}/dashboard/auth` | `GET` | Platform account connection page |
| `/{locale}/dashboard/collab` | `GET` | Collaboration entry |
| `/{locale}/dashboard/settings` | `GET` | Workspace settings |
| `/api/health` | `GET` | Frontend health check |
| `/api/ready` | `GET` | Frontend readiness check |
| `/api/auth/login` | `POST` | Proxies backend `/api/auth/login`; writes auth cookie on success |
| `/api/auth/register` | `POST` | Proxies backend `/api/auth/register`; writes auth cookie on success |
| `/api/auth/session` | `GET` | Checks whether the auth cookie is still valid |
| `/api/auth/session` | `POST` | Creates a web session cookie from body field `token` |
| `/api/auth/session` | `DELETE` | Clears the web session cookie |
| `/api/auth/{path...}` | `POST` | Proxies backend `/api/auth/{path...}` |
| `/api/{path...}` | `GET/POST/PUT/PATCH/DELETE/OPTIONS` | Proxies backend `/api/{path...}` |
| `/api/browser-stream/{path...}` | rewrite | Rewrites to backend `/api/browser-stream/{path...}` |
| `/api/user/dashboard/browser-sessions/{path...}` | rewrite | Rewrites to the matching backend browser-session endpoint |

The frontend proxy adds `Authorization: Bearer <token>` from the auth cookie when possible. For cookie-authenticated write requests, the proxy enforces a same-origin check and returns `403 csrf_failed` when the check fails.

### 3.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `PORT` | No | `3000` in Compose | Next.js listening port |
| `HOSTNAME` | No | `0.0.0.0` in Compose | Next.js listening host |
| `BACKEND_API_BASE_URL` | No | `http://localhost:8080` | Server-side API proxy target |
| `FRONTEND_BASE_URL` | No | None | Public site URL for SEO and dev origin allowlist |
| `NEXT_PUBLIC_SITE_URL` | No | None | Public frontend fallback for `FRONTEND_BASE_URL` |
| `NEXT_PUBLIC_BROWSER_STREAM_BASE_URL` | No | None | Optional browser stream base URL override for local development |
| `MPP_FRONTEND_TURBOPACK_FS_CACHE` | No | `true` | Controls Next dev Turbopack filesystem cache |
| `APP_ENV` / `NODE_ENV` | No | `development` | Controls local vs production behavior |
| `ENABLE_MOCK_LOGIN` | No | `false` | Enables mock login entry in local environments |

## 4. Backend API

The backend is the main business API, authentication boundary, project/publish orchestrator, and cross-service coordination point.

### 4.1 Public Interfaces

| Path | Method | Parameters | Description |
| --- | --- | --- | --- |
| `/ping` | `GET` | None | Returns `{ "message": "pong" }` |
| `/health` | `GET` | None | Liveness check |
| `/ready` | `GET` | None | Checks database and Redis |
| `/metrics` | `GET` | None | Prometheus metrics |

### 4.2 Authentication Interfaces

| Path | Method | Body | Description |
| --- | --- | --- | --- |
| `/api/auth/login` | `POST` | `username`, `password` | Logs in and returns a JWT |
| `/api/auth/register` | `POST` | `username`, `email`, `password`, `code` | Registers a user and returns a JWT |
| `/api/auth/send-code` | `POST` | `email`, `scene` | Sends a verification code. `scene` is `register` or `forgot_password` |
| `/api/auth/reset-password` | `POST` | `email`, `code`, `password` | Resets a password |
| `/api/auth/mock-login` | `POST` | `username` | Enabled only when `ENABLE_MOCK_LOGIN=true` in a local environment |
| `/api/user/dashboard/settings/x/oauth2/callback` | `GET` | X OAuth2 query parameters | X OAuth2 callback |

### 4.3 Admin Interfaces

Prefix: `/api/admin/dashboard`

| Path | Method | Parameters | Description |
| --- | --- | --- | --- |
| `/stats` | `GET` | None | Global statistics |
| `/projects` | `GET` | Query pagination parameters | Project list |
| `/projects/{id}/publications` | `GET` | Path UUID `id` | Publication records for a project |

### 4.4 User Dashboard Interfaces

Prefix: `/api/user/dashboard`

| Group | Path | Method | Key parameters |
| --- | --- | --- | --- |
| Stats | `/stats` | `GET` | None |
| Extension session | `/extension/session` | `GET` | None |
| Extension prepublish | `/extension/prepublish` | `GET` | None |
| Extension handoff | `/extension/handoffs` | `POST` | `project_id`, `platforms` |
| Extension callback | `/extension/events` | `POST` | No web JWT required. Body includes `token`, `event_id`, `platform`, `status`, `remote_id`, `publish_url`, `error_message`, `metadata` |
| Content templates | `/content-templates` | `GET/POST` | Create with `name`, `title_template`, `source_template`, `default_platforms`; optional `description`, `platform_config`, `tags` |
| Brand profiles | `/brand-profiles` | `GET/POST` | Create with `name`; optional `voice`, `audience`, `banned_words`, `cta`, `link_strategy`, `default_tags` |
| Projects | `/projects` | `GET/POST` | Create with `title`, `source_content`, `platforms`; optional `summary`, `cover_image_url`, `template_id`, `brand_profile_id` |
| Project detail | `/projects/{id}` | `GET/PUT` | Path UUID `id`; update body matches create body |
| Project collaborators | `/projects/{id}/collaborators` | `GET/POST` | Add with `user_id` or `email`, plus `role` |
| Project collaborator | `/projects/{id}/collaborators/{userId}` | `PATCH/DELETE` | Path UUID `userId`; update with `role` |
| Project activity | `/projects/{id}/activity` | `GET` | Path UUID `id` |
| Project comments | `/projects/{id}/comments` | `GET/POST` | Create with `body`; optional `anchor_text`, `metadata` |
| Project comment | `/projects/{id}/comments/{commentId}` | `PATCH` | Update with `status` |
| Project versions | `/projects/{id}/versions` | `GET` | Path UUID `id` |
| Version restore | `/projects/{id}/versions/{versionId}/restore` | `POST` | Path UUID `versionId` |
| Share links | `/projects/{id}/share-links` | `GET/POST` | Create with `role`; optional `expires_at` |
| Share link | `/projects/{id}/share-links/{linkId}` | `DELETE` | Path UUID `linkId` |
| Accept project share | `/project-share-links/{token}/accept` | `POST` | Path `token` |
| Collab session | `/projects/{id}/collab/session` | `POST` | Path UUID `id` |
| Save content | `/projects/{id}/content` | `PATCH` | `title`, `source_content`; optional `summary`, `cover_image_url` |
| Save platforms | `/projects/{id}/platforms` | `PATCH` | `platforms` |
| Media upload | `/projects/{id}/media/uploads` | `POST` | `filename`, `mime_type`, `size_bytes`, `usage`; optional `library_scope`, `tags`, `alt_text`, `source` |
| Complete media upload | `/media/{id}/complete` | `POST` | Path media UUID `id` |
| Resolve media | `/media/resolve` | `POST` | `asset_ids` |
| Delete media | `/media/{id}` | `DELETE` | Path media UUID `id` |
| Publications | `/projects/{id}/publications` | `GET` | Path UUID `id` |
| Sync prepublish | `/projects/{id}/prepublish/sync` | `POST` | `platforms`, `actor.type` |
| Update draft | `/projects/{id}/prepublish/{platform}` | `PUT` | Path `platform`; body `adapted_content` |
| Publish | `/projects/{id}/publish` | `POST` | `platform` or `platforms`; optional `mode`, `idempotency_key` |
| Douyin browser publish | `/projects/{id}/publish-sessions/douyin` | `POST` | Path UUID `id` |
| AI source edit | `/ai/content/edit` | `POST` | `content`, `message`; optional `title`, `conversation` |
| AI source edit stream | `/ai/content/edit/stream` | `POST` | Same as above; response is `text/markdown` stream |
| AI prepublish edit | `/ai/prepublish/edit` | `POST` | `platform`, `adapted_content`, `message`; optional `title`, `conversation` |
| AI prepublish edit stream | `/ai/prepublish/edit/stream` | `POST` | Same as above; response is `text/markdown` stream |
| WeChat account | `/settings/wechat/account` | `GET/PUT` | Save with `app_id`, `app_secret` |
| WeChat test | `/settings/wechat/test` | `POST` | `app_id`, `app_secret` |
| Douyin account | `/settings/douyin/account` | `GET` | None |
| Zhihu account | `/settings/zhihu/account` | `GET` | None |
| X account | `/settings/x/account` | `GET/PUT` | Save with `api_key`, `api_secret`, `access_token`, `access_token_secret`, `username` |
| X test | `/settings/x/test` | `POST` | `api_key`, `api_secret`, `access_token`, `access_token_secret` |
| X OAuth2 | `/settings/x/oauth2/start` | `GET` | None |
| Browser login session | `/settings/platforms/{platform}/browser-session` | `POST` | Path `platform`, such as `douyin` or `zhihu`; optional query `workspace_id`, `platform_account_id`; `workspace_id` may also come from header `X-Workspace-ID` |
| Browser session status | `/browser-sessions/{id}` | `GET` | Path session UUID `id` |
| Browser stream | `/browser-sessions/{id}/stream` | `GET` | Path session UUID `id` |
| Complete browser session | `/browser-sessions/{id}/complete` | `POST` | Path session UUID `id` |
| Cancel browser session | `/browser-sessions/{id}` | `DELETE` | Path session UUID `id` |

Current platform keys include `douyin`, `wechat`, `x`, and `zhihu`.

### 4.5 Workspace Interfaces

Prefix: `/api/workspaces`

| Path | Method | Key parameters |
| --- | --- | --- |
| `/` | `GET/POST` | Create with `name`; optional `slug` |
| `/{id}` | `GET/PATCH` | Path UUID `id`; update with `name`; optional `slug` |
| `/{id}/projects` | `GET/POST` | Create project body matches user project creation |
| `/{id}/content-templates` | `GET/POST` | Body matches content template creation |
| `/{id}/brand-profiles` | `GET/POST` | Body matches brand profile creation |
| `/{id}/activity` | `GET` | Path UUID `id` |
| `/{id}/members` | `GET/POST` | Add with `user_id` or `email`, plus `role` |
| `/{id}/members/{userId}` | `PATCH/DELETE` | Update with `role` |
| `/{id}/invites` | `GET/POST` | Create with `email`, `role`; optional `expires_at` |
| `/{id}/invites/{inviteId}` | `DELETE` | Revoke invite |
| `/invites/accept` | `POST` | `token` |

### 4.6 Collab REST Interfaces

Prefix: `/api/collab`

| Path | Method | Description |
| --- | --- | --- |
| `/documents` | `GET` | Lists collaboration documents |
| `/documents/{id}` | `GET` | Gets a collaboration document |
| `/documents` | `POST` | Creates a collaboration document |
| `/documents/{id}` | `PATCH` | Updates a collaboration document |
| `/documents/{id}/session` | `POST` | Creates a collaborative editing WebSocket session token |

### 4.7 Backend Internal Interface

| Path | Method | Authentication | Body |
| --- | --- | --- | --- |
| `/internal/media/resolve` | `POST` | `X-MPP-Internal-Token` | `object_ref` |

This endpoint lets `content-pipeline-service` exchange an object ref for a short-lived downloadable URL.

### 4.8 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `PORT` | No | `8080` | HTTP listening port |
| `APP_ENV` | No | None | Environment label such as `development` or `production` |
| `JWT_SECRET` | Yes | None | Web JWT signing secret |
| `BACKEND_PROCESS_ROLE` | No | `all` | One of `all`, `api`, `worker` |
| `BACKEND_REQUIRE_REDIS` | No | `false` | If `true`, startup fails when Redis is not configured |
| `EXTENSION_ALLOWED_ORIGINS` | No | Empty | Comma-separated extension origins allowed to call backend; wildcards are not allowed |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | Yes | Provided by Compose templates | PostgreSQL connection |
| `DB_SSLMODE`, `DB_SSLROOTCERT` | No | `disable`, empty | PostgreSQL TLS |
| `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME` | No | `10`, `5`, `30m`, `5m` | Database pool settings |
| `DB_READER_HOST` and other `DB_READER_*` values | No | Empty | Optional read replica |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_TLS` | Depends on feature | Redis DB defaults to `0` | Queues, locks, verification codes, rate limits, browser sessions |
| `REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`, `REDIS_MAX_IDLE_CONNS`, `REDIS_CONN_MAX_IDLE_TIME`, `REDIS_CONN_MAX_LIFETIME` | No | go-redis defaults or template values | Redis pool settings |
| `AI_SERVICE_URL` | No | Empty | AI features are unavailable when empty |
| `AI_SERVICE_INTERNAL_TOKEN` | Yes when AI is enabled | Empty | Bearer token for calling AI Service |
| `BROWSER_WORKER_URL` | No | Empty | Empty value uses the backend mock worker |
| `BROWSER_WORKER_INTERNAL_TOKEN` | Yes when Browser Worker is enabled | Empty | Bearer token for calling Browser Worker |
| `BROWSER_SESSION_USER_CONCURRENCY_LIMIT`, `BROWSER_SESSION_TENANT_CONCURRENCY_LIMIT` | No | `2`, `10` | Remote browser login session concurrency limits |
| `BROWSER_STREAM_GATEWAY_TIMEOUT` | No | `15s` | Backend timeout for browser stream proxy connections/responses |
| `STREAM_GATE_ENABLED`, `STREAM_GATE_KEY_PREFIX` | No | `true`, `mpp:stream` | AI/browser stream concurrency gate |
| `AI_STREAM_*`, `BROWSER_STREAM_*` | No | Code defaults | Stream connection limits and TTLs, such as user/tenant/IP/global limits |
| `CONTENT_PIPELINE_HOST`, `CONTENT_PIPELINE_PORT` | No | host empty, port `50051` | Content Pipeline gRPC address |
| `CONTENT_PIPELINE_MEDIA_ENABLED`, `CONTENT_PIPELINE_DRAFTS_ENABLED` | No | `false` | Controls whether Rust content pipeline capabilities are enabled |
| `CONTENT_PIPELINE_MEDIA_RESOLVER_URL` | Yes when media object refs are enabled | `http://backend:8080/internal/media/resolve` in Compose | Object ref resolver URL |
| `CONTENT_PIPELINE_INTERNAL_TOKEN` | Yes when internal resolver is enabled | Empty | Internal media resolver token |
| `COLLAB_TOKEN_SECRET` | Yes when collaboration is enabled | Falls back to `JWT_SECRET` | Signs collab session tokens and authorizes collab internal API calls |
| `COLLAB_INTERNAL_URL` | No | `http://localhost:8090` | Backend-to-Collab Service URL |
| `COLLAB_WEBSOCKET_URL_BASE` | No | `ws://localhost:8090` | WebSocket base URL returned to the frontend |
| `OBJECT_STORAGE_PROVIDER` | No | disabled | Set to `r2` to enable media uploads and archive exports |
| `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET` | Yes when R2 is enabled | None | R2 object storage settings |
| `R2_ENDPOINT`, `R2_REGION` | Partially required when R2 is enabled | endpoint can be inferred from account id; region defaults to `auto` | R2 endpoint and region |
| `MEDIA_UPLOAD_URL_TTL`, `MEDIA_DOWNLOAD_URL_TTL` | No | `10m`, `5m` | Signed upload/download URL TTLs |
| `EVENT_ARCHIVE_ENABLED` | No | `false` | Enables the cold event archive worker when R2/S3 object storage is configured |
| `EVENT_ARCHIVE_INTERVAL`, `EVENT_ARCHIVE_BATCH_SIZE`, `EVENT_ARCHIVE_OBJECT_PREFIX` | No | `24h`, `500`, `archives/database` | Archive worker cadence, per-table batch size, and object key prefix |
| `PUBLISH_EVENT_RETENTION_DAYS`, `EXTENSION_EXECUTION_EVENT_RETENTION_DAYS` | No | `180`, `180` | Hot-table retention for publish and extension execution events |
| `PROJECT_ACTIVITY_RETENTION_DAYS`, `WORKSPACE_ACTIVITY_RETENTION_DAYS`, `BROWSER_SESSION_HISTORY_RETENTION_DAYS` | No | `365`, `365`, `90` | Hot-table retention for activity history and terminal browser sessions |
| `COOKIE_ENCRYPTION_KEY` | Yes when remote browser accounts are enabled | None | Cookie encryption key; the code expects 32 bytes |
| `APP_RATE_LIMIT_ENABLED`, `APP_RATE_LIMIT_KEY_PREFIX` | No | Enabled in templates | Application-level rate limiting |
| `X_OAUTH2_CLIENT_ID`, `X_OAUTH2_CLIENT_SECRET`, `X_OAUTH2_REDIRECT_URL` | Yes when X OAuth2 is enabled | None | X OAuth2 settings |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, `SMTP_PASSWORD` | No | Mock email service when host is empty | Verification email delivery |

## 5. Publish Worker

`publish-worker` is built from the backend codebase, but it only runs queue consumers and a health-check HTTP server. It does not expose business write APIs.

### 5.1 External Interfaces

| Path | Method | Description |
| --- | --- | --- |
| `/health` | `GET` | Liveness check |
| `/ready` | `GET` | Checks database and Redis |
| `/metrics` | `GET` | Prometheus metrics |

### 5.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `PORT` | No | `8080` | Health server port |
| `REDIS_ADDR` | Yes | None | Required for publish and email queues |
| `DB_*` | Yes | Provided by Compose templates | Persistent publication state |
| `BROWSER_WORKER_URL`, `BROWSER_WORKER_INTERNAL_TOKEN` | Yes when browser publishing is enabled | Empty | Browser publishing runtime |
| `CONTENT_PIPELINE_*` | Yes when content adaptation is enabled | Same as backend | Media and draft pipeline |
| `SMTP_*` | No | Mock email service when host is empty | Async email worker |

## 6. AI Service

AI Service should only be called by the backend. All endpoints except `/health`, `/ready`, and `/metrics` require a bearer token.

### 6.1 External Interfaces

| Path | Method | Body | Response |
| --- | --- | --- | --- |
| `/health` | `GET` | None | `{ "status": "healthy" }` |
| `/ready` | `GET` | None | `{ "status": "ready" }` or 503 |
| `/metrics` | `GET` | None | Prometheus metrics |
| `/content/edit` | `POST` | `content`, `message`, optional `title`, `conversation[]` | JSON `{ channel, content, usage }` |
| `/content/edit/stream` | `POST` | Same as `/content/edit` | `text/markdown; charset=utf-8` stream |
| `/prepublish/edit` | `POST` | `platform`, `adapted_content`, `message`, optional `title`, `conversation[]` | JSON `{ channel, platform, adapted_content, content, usage }` |
| `/prepublish/edit/stream` | `POST` | Same as `/prepublish/edit` | `text/markdown; charset=utf-8` stream |
| `/calibrate` | `POST` | `content`, `platform` | JSON `{ platform, calibrated_content }` |

`conversation[]` item format:

```json
{
  "role": "user",
  "content": "Make it shorter"
}
```

`role` supports `user` or `assistant`.

### 6.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `AI_SERVICE_INTERNAL_TOKEN` | Yes | None | Internal API bearer token |
| `LLM_PROVIDER_URL` | Yes | None | OpenAI-compatible base URL |
| `LLM_MODEL` | Yes | None | Model name |
| `LLM_PROVIDER_KEY` | Yes | None | Provider API key |
| `LLM_REQUEST_TIMEOUT_SECONDS` | No | `90` | LLM request timeout |
| `LLM_MAX_RETRIES` | No | `2` | LLM retry count |
| `LLM_STREAM_CHUNK_TIMEOUT_SECONDS` | No | `30` | Streaming chunk timeout |
| `LLM_INPUT_COST_PER_1K_TOKENS` | No | `0` | Cost accounting |
| `LLM_OUTPUT_COST_PER_1K_TOKENS` | No | `0` | Cost accounting |
| `LLM_COST_CURRENCY` | No | `USD` | Cost currency |

## 7. Browser Worker

Browser Worker creates isolated browser runtimes, proxies the browser stream, captures cookies/profile data, and runs selected browser publishing actions. All `/internal/browser-sessions` endpoints require `Authorization: Bearer <BROWSER_WORKER_INTERNAL_TOKEN>`.

### 7.1 External Interfaces

| Path | Method | Body/parameters | Description |
| --- | --- | --- | --- |
| `/health` | `GET` | None | Liveness check |
| `/ready` | `GET` | None | Checks Redis |
| `/metrics` | `GET` | None | Prometheus metrics |
| `/internal/browser-sessions` | `POST` | `session_id`, `user_id`, `platform`, `login_url`, `allowed_domains`, `required_cookies`, `initial_cookies`, `ttl_seconds`, `viewport` | Creates a browser session |
| `/internal/browser-sessions/{ref}` | `GET` | Path `ref` | Gets worker session state |
| `/internal/browser-sessions/{ref}/stream` | any | Path `ref` | Proxies browser stream |
| `/internal/browser-sessions/{ref}/stream/{path...}` | any | Path `ref` | Proxies browser stream subpaths |
| `/internal/browser-sessions/{ref}/capture` | `POST` | Path `ref` | Captures cookies and account profile |
| `/internal/browser-sessions/{ref}/publish/douyin` | `POST` | `title`, `content`, `cover_image_base64`, `cover_image_name` | Starts the Douyin publish script |
| `/internal/browser-sessions/{ref}` | `DELETE` | Path `ref` | Deletes the session and cleans up the runtime |

Key body shape:

```json
{
  "session_id": "uuid",
  "user_id": "uuid",
  "platform": "douyin",
  "login_url": "https://...",
  "ttl_seconds": 900,
  "allowed_domains": [
    {
      "host": "douyin.com",
      "match": "suffix",
      "schemes": ["https"],
      "purpose": "login"
    }
  ],
  "required_cookies": [
    {
      "name": "sessionid",
      "domain_suffixes": [".douyin.com"],
      "required": true,
      "preserve": true
    }
  ],
  "initial_cookies": [],
  "viewport": {
    "width": 1366,
    "height": 768
  }
}
```

### 7.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `BROWSER_WORKER_INTERNAL_TOKEN` | Yes | None | Internal API bearer token |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_TLS` | No | Empty disables Redis client, but `/ready` can still pass with an empty store | Session state storage |
| `BROWSER_WORKER_POOL_SIZE` | No | `4` | Concurrent browser session limit; `0` means unlimited |
| `BROWSER_RUNTIME_DRIVER` | No | `docker` | `docker` or `kubernetes` |
| `BROWSER_RUNTIME_IMAGE` | Docker driver | `mpp-browser-runtime` | Browser runtime image |
| `BROWSER_RUNTIME_NETWORK` | No | Empty | Docker network. Compose uses `mpp-browser-runtime` |
| `BROWSER_RUNTIME_BIND_IP` | No | `127.0.0.1` | Port binding IP when no runtime network is used |
| `BROWSER_RUNTIME_HOST` | No | `127.0.0.1` | Host used by worker to access the runtime |
| `DOCKER_HOST` | Docker driver | Docker default | Docker daemon endpoint |
| Kubernetes runtime variables | Kubernetes driver | None | Read by `browser-worker/internal/kubernetes`, such as kubeconfig, namespace, and image settings |

The service currently listens on `:8081`.

## 8. Collab Service

Collab Service provides the collaborative editing WebSocket and persists Yjs document state to PostgreSQL. Backend signs session tokens, and frontend uses those tokens to connect to the WebSocket.

### 8.1 External Interfaces

| Path | Method | Authentication/parameters | Description |
| --- | --- | --- | --- |
| `/health` | `GET` | None | Liveness check |
| `/ready` | `GET` | None | Checks database and reports Redis sync state |
| `/metrics` | `GET` | None | Prometheus metrics |
| `/internal/collab/documents/{documentId}/project-state` | `POST` | `Authorization: Bearer <COLLAB_TOKEN_SECRET>` | Initializes project collaboration state |
| `/internal/collab/documents/{documentId}/project-source-content` | `POST` | `Authorization: Bearer <COLLAB_TOKEN_SECRET>` | Syncs project source content into the collaboration document |
| `/collab/documents/{documentId}?token={jwt}` | `GET` WebSocket | Collab session JWT | Hocuspocus/Yjs collaborative editing connection |

Collab session JWTs must include:

| Claim | Description |
| --- | --- |
| `user_id` | User UUID |
| `document_id` | Document UUID |
| `role` | `editor` or `viewer` |
| `purpose` | Fixed value `collab-session` |
| issuer | `mpp-backend` |
| audience | `mpp-collab-service` |

### 8.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `COLLAB_HOST` | No | `0.0.0.0` | Listening host |
| `COLLAB_PORT` | No | `8090` | Listening port |
| `LOG_LEVEL` | No | `info` | Fastify logger level |
| `COLLAB_WS_PATH` | No | `/collab/documents/:documentId` | WebSocket path template |
| `COLLAB_TOKEN_SECRET` | Yes | None | Internal API and WebSocket JWT verification secret |
| `DATABASE_URL` | No | None | PostgreSQL URL. When set, it can replace split DB parameters |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`, `DB_SSLROOTCERT` | Yes | Template values | PostgreSQL connection |
| `DB_MAX_OPEN_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME` | No | `10`, `30m`, `5m` | Database pool settings |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_TLS` | Yes when Redis sync is enabled | `redis:6379`, empty, `0`, `false` | Multi-instance collaboration sync |
| `COLLAB_REDIS_SYNC_ENABLED` | No | `true` | Enables Redis pub/sub sync |
| `COLLAB_REDIS_CHANNEL_PREFIX` | No | `mpp:collab:doc` | Redis channel prefix |
| `COLLAB_HEARTBEAT_SECONDS` | No | `30` | Hocuspocus timeout |
| `COLLAB_UPDATE_FLUSH_MS` | No | `300` | Yjs update debounce |
| `COLLAB_UPDATE_FLUSH_MAX_MS` | No | `2000` | Maximum debounce |
| `COLLAB_UPDATE_FLUSH_MAX_COUNT` | No | `32` | Batch flush threshold |
| `COLLAB_UPDATE_FLUSH_RETRY_MAX_ATTEMPTS` | No | `5` | Flush retry count |
| `COLLAB_UPDATE_FLUSH_RETRY_MAX_MS` | No | `30000` | Maximum flush retry duration |
| `COLLAB_UPDATE_RETENTION_DAYS` | No | `30` | Update retention days |
| `BACKEND_INTERNAL_URL` | No | `http://backend:8080` | Reserved backend internal integration URL |

## 9. Content Pipeline Service

Content Pipeline Service is a Rust gRPC service that processes media and compiles platform drafts.

### 9.1 gRPC Interfaces

Package: `mpp.contentpipeline.v1`

| Service | RPC | Request | Response | Description |
| --- | --- | --- | --- | --- |
| `MediaAssetProcessor` | `ProcessAsset` | `ProcessAssetRequest` | `ProcessAssetResponse` | Processes URL, data URL, or object-ref media |
| `PlatformDraftCompiler` | `CompileDrafts` | `CompileDraftsRequest` | `CompileDraftsResponse` | Compiles a source project into platform drafts |

Key `ProcessAssetRequest` fields:

| Field | Description |
| --- | --- |
| `request_id` | Caller-generated request ID |
| `platform` | Target platform, such as `wechat`, `douyin`, `x`, `zhihu` |
| `usage` | Media usage, such as cover or inline |
| `source.url` | Remote media URL |
| `source.data_url` | Data URL |
| `source.object_ref` | Backend media object ref. Requires resolver configuration |
| `constraints.max_bytes` | Maximum output bytes |
| `constraints.preferred_mime_types` | Preferred MIME types |

Key `CompileDraftsRequest` fields:

| Field | Description |
| --- | --- |
| `request_id` | Caller-generated request ID |
| `project.id` | Source project ID |
| `project.title` | Title |
| `project.source_format` | Source format |
| `project.source_content` | Source content |
| `targets[].platform` | Platform key |
| `targets[].profile` | Profile, such as a platform version profile |
| `targets[].config_json` | Platform config JSON string |

### 9.2 HTTP Operations Interface

| Address | Path | Description |
| --- | --- | --- |
| `CONTENT_PIPELINE_METRICS_ADDR` | `/metrics` | Prometheus metrics |

The business service listens on `CONTENT_PIPELINE_ADDR`, which defaults to `0.0.0.0:50051`. The service also registers gRPC health and reflection.

### 9.3 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `CONTENT_PIPELINE_ADDR` | No | `0.0.0.0:50051` | gRPC listening address |
| `CONTENT_PIPELINE_METRICS_ADDR` | No | `0.0.0.0:9090` | Metrics HTTP listening address |
| `CONTENT_PIPELINE_MEDIA_RESOLVER_URL` | Yes when object-ref media is enabled | None; Compose points it to the backend internal endpoint | Object ref resolver URL |
| `CONTENT_PIPELINE_INTERNAL_TOKEN` | Yes when object-ref media is enabled | None | Token for calling backend resolver |
| `RUST_LOG` | No | `content_pipeline_service=info` | Tracing filter |

## 10. Extension

The Extension module receives handoffs from the browser side, fills platform pages, and reports publishing status back to the system.

### 10.1 External Interfaces

The extension does not expose a server-side HTTP API. It interacts with the system through:

| Interface | Description |
| --- | --- |
| Backend REST | Calls backend APIs through `WXT_MPP_API_BASE_URL` |
| Web login | Builds login URLs through `WXT_MPP_WEB_BASE_URL` |
| Content scripts | Injected into Zhihu, Xiaohongshu, Douyin, Bilibili, X/Twitter, and related platform pages |
| Extension handoff callback | Reports publish results through the callback URL and token returned by backend |

### 10.2 Runtime Parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `WXT_MPP_API_BASE_URL` | No | `http://localhost:8080` | Backend API base URL |
| `WXT_MPP_WEB_BASE_URL` | No | `http://localhost:3000` | Web app base URL |
| `WXT_MANUAL_RUNNER` | No | `false` | Set to `true` in Docker dev to disable WXT automatic browser runner |
| `EXTENSION_DEV_PORT` | No | `3010` | Docker dev extension profile port |

## 11. Data And Infrastructure Modules

### 11.1 PostgreSQL

| Parameter | Default | Description |
| --- | --- | --- |
| `DB_HOST` | `db` in Compose | Primary database host |
| `DB_PORT` | `5432` | Primary database port |
| `DB_USER` | `postgres` | User |
| `DB_PASSWORD` | `postgres` in dev template | Password |
| `DB_NAME` | `poster_db` | Database |
| `DB_SSLMODE` | `disable` | TLS mode |
| `DB_SSLROOTCERT` | Empty | CA certificate path |

Compose also includes `pgbouncer`. In Compose, backend and collab-service connect to `pgbouncer:5432` by default.

### 11.2 Redis

| Parameter | Default | Description |
| --- | --- | --- |
| `REDIS_ADDR` | `redis:6379` in Compose | Redis address |
| `REDIS_PASSWORD` | Empty | Password |
| `REDIS_DB` | `0` | DB index |
| `REDIS_TLS` | `false` | TLS |

Redis is used for publish queues, locks, verification codes, rate limits, OAuth state, browser session state, and Collab multi-instance sync.

### 11.3 Traefik Gateway

| Parameter | Default | Description |
| --- | --- | --- |
| `TRAEFIK_HTTP_PORT` | `80`; dev gateway uses `8088` | HTTP entrypoint |
| `TRAEFIK_HTTPS_PORT` | `443`; dev gateway uses `8443` | HTTPS entrypoint |
| `TRAEFIK_LOG_LEVEL` | `INFO` | Log level |
| `TRAEFIK_RATE_LIMIT_*` | Template values | Gateway rate limiting |

Traefik routing rules:

| Path prefix | Target service |
| --- | --- |
| `/` | `frontend:3000` |
| `/collab` | `collab-service:8090` |

### 11.4 Observability

| Service | Default port | Description |
| --- | --- | --- |
| Prometheus | `9090` | Scrapes metrics |
| Loki | `3100` | Logs |
| Alloy | `12345` | Collection |
| Grafana | `3001` | Dashboard |
