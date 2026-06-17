# MPP High-concurrency and Distributed Architecture Evolution Plan

## 1. Background

MPP already has the shape of a multi-service system:

- `frontend`: Next.js workspace and API proxy entry point.
- `backend`: Go API, user-facing business logic, publishing orchestration, account management, and task coordination.
- `ai-service`: FastAPI AI editing service responsible for prompts, model calls, and streaming responses.
- `browser-worker`: Remote browser sessions, Chromium container creation and teardown, and login-state capture.
- `PostgreSQL`: Persistent business data.
- `Redis`: Publishing queues, distributed locks, OAuth state, and temporary session state.

This means the project does not need to introduce a "microservice architecture" from scratch. A better direction is to keep the Go backend as the business core, while gradually evolving high-risk, resource-intensive, asynchronous, and strongly external-dependency-heavy parts into independent services or workers.

## 2. Goals

This plan aims to improve MPP's concurrency capacity, stability, security, observability, failure recovery, and resource governance.

Every design decision should map to real MPP business scenarios and avoid infrastructure that is too complex to maintain at the current stage.

## 3. Design Principles

- First use the existing PostgreSQL, Redis, Docker Compose, and service boundaries well; then consider heavier infrastructure.
- First solve entry security, queue reliability, idempotency, rate limiting, monitoring, and failure isolation; then consider Kubernetes, service mesh, and heavier data infrastructure.
- Split services by runtime characteristics, not by rigid business nouns.
- Every asynchronous task must have an idempotency key, state machine, retry policy, and traceable logs.
- Every external platform call must support degradation, retry, rate limiting, and auditing.

Progress, checklists, and the target Citus state for data-layer expansion are maintained in [MPP Progressive Plan for Database Read/write Splitting and Horizontal Partitioning](./database-optimization.md).

## 4. Recommended Overall Architecture

![MPP high-concurrency and distributed architecture evolution diagram](../assets/plan/high-concurrency-distributed-architecture.svg)

## 5. Architecture Design Candidate Table

Scoring notes:

- Production value: 1 low, 5 high.
- Cost: 1 low, 5 high.
- Recommended priority: P0 do immediately, P1 prioritize, P2 do after growth, P3 not recommended for now.
- Completion status is based on the current code inventory: `Done` means the main implementation points are clearly in place, `In progress` means partial implementation exists but key capabilities are still missing, `Not started` means no clear implementation has been found, and `Deferred` means the plan explicitly does not recommend prioritizing it now.

| No. | Architecture design | Problem solved | Project landing point | Production value | Cost | Priority | Status | Recommendation |
| --- | ------------------- | -------------- | --------------------- | ---------------- | ---- | -------- | ------ | -------------- |
| 1 | Traefik API Gateway | Unified entry point, HTTPS, routing, and internal service hiding | Application entry point exposes only `80/443`, hiding backend, AI, worker, DB, and Redis | 5 | 2 | P0 | In progress | Application entry and service hiding are in place; Let's Encrypt automatic certificates and manual-certificate Compose overrides are in place, while production domain/certificate configuration must be set per environment; Prometheus/Grafana/Loki/Alloy observability ports are still explicitly exposed through environment variables |
| 2 | Gateway and application two-layer rate limiting | Prevent crawlers, malicious requests, AI abuse, and publishing task floods | Traefik handles IP-level rate limits; backend handles user/workspace/API-level quotas | 5 | 2 | P0 | Done | Critical production protection capability |
| 3 | Observability baseline | Enable issue diagnosis and show the real runtime state | Prometheus metrics, Grafana dashboards, Loki logs, Trace ID | 5 | 3 | P0 | Done | backend, publish-worker, ai-service, and browser-worker already have HTTP metrics and structured request logs; this is a baseline, not full distributed tracing |
| 4 | Health checks and graceful shutdown | Support rolling restarts and avoid interrupted requests | Add health/readiness to frontend/backend/ai/browser-worker | 5 | 2 | P0 | Done | Low cost and required for production |
| 5 | Stateless API service and horizontal scaling | Support more concurrent requests | backend does not store local sessions, scales to multiple replicas, and shares Redis/Postgres | 5 | 2 | P1 | Done | API is basically stateless and supports `api/worker/all` roles; production Compose defaults to multiple backend replicas, and data-layer evolution is covered by the [database plan](./database-optimization.md) |
| 6 | Upgrade Redis queues to a reliable task model | Make publishing asynchronous, retryable, and recoverable | Use Redis Streams or Asynq to manage publish jobs | 5 | 3 | P1 | Done | Redis List has been replaced with Asynq; publish jobs have ack/retry/worker crash recovery/archive semantics, and task payloads store only durable IDs without browser session addresses or tokens |
| 7 | Idempotency keys and publication state machine | Prevent repeated clicks, duplicate consumption, and duplicate publishing | publish requests include an idempotency key, and the publication state machine transitions strictly | 5 | 3 | P1 | Done | Publish requests support idempotency key reuse; publication states are aligned to `draft`, `syncing`, `queued`, `publishing`, `succeeded`, `failed`, and `cancelled`, while old state names remain as compatibility aliases |
| 8 | Outbox Pattern | Keep database updates and event delivery consistent | Write outbox after publication state updates, then let worker deliver tasks | 4 | 4 | P1 | Done | The publishing queue path has a transactional Outbox: `EnqueuePublishProject` writes `outbox_events` in the same transaction, dispatches immediately after commit, and the worker periodically flushes failed/stale processing records and retries; currently only `publish.job_requested` is covered, while general multi-consumer CDC follows the database evolution plan |
| 9 | Stronger distributed locks | Avoid concurrent publishing of the same publication | Redis lock with owner, TTL, renewal, and release validation | 5 | 2 | P1 | Done | Redis is already in the project, so the cost is manageable |
| 10 | External-call circuit breaking, retry, and backoff | Prevent third-party platform or LLM failures from dragging down the system | Unified retry/backoff/circuit breaker for AI, WeChat, Zhihu, X, and Douyin calls | 5 | 3 | P1 | Done | backend has a unified resilience layer; HTTP retry covers safe methods by default, non-idempotent POST only uses timeout/circuit breaker; the publishing operation layer breaks circuits by platform but does not retry full publish operations; ai-service LLM client has timeout, max retries, and stream chunk timeout configured |
| 11 | Browser Worker resource pool and quotas | Control Chromium container count and avoid exhausting the host | Per-user/workspace browser session concurrency limits and a global worker pool | 5 | 3 | P1 | Done | User+platform active session locks, user/workspace concurrency quotas, a global worker pool, and container CPU/memory limits are already present |
| 12 | WebSocket/SSE long-connection governance | Handle AI streams and remote browser streams | Gateway timeout, connection limits, stream tokens, and reconnection recovery | 4 | 3 | P1 | Done | AI streams and remote browser long connections are covered by connection-count limits; remote browser streams validate tokens uniformly, HTTP gateway timeout returns 504, and reconnects are supported within token validity |
| 13 | Object storage and signed URLs | Keep images and media out of application containers and the database | S3/R2/OSS stores media, backend generates signed URLs | 5 | 3 | P2 | Done | R2 object storage configuration, signed upload/download URLs, media object references, and temporary signed URLs before publishing are in place; next focus is CDN, archiving, and production configuration |
| 14 | CDN and static asset cache | Reduce frontend asset and image access pressure | Next static assets and media files use CDN | 4 | 2 | P2 | Not started | Do this gradually after public launch |
| 15 | Temporal workflow orchestration | Complex long-running flows, recoverable tasks, and Saga | Multi-platform publishing, browser automation, retry compensation | 4 | 5 | P2 | Not started | Introduce after publishing workflows become more complex |
| 16 | Kubernetes | Service scheduling, elastic scaling, and rolling releases | Migrate from Compose to K8s + Ingress | 3 | 5 | P3 | In progress | Kustomize packages, environment overlays, image pinning, NetworkPolicy, observability resources, and an operations runbook exist; still treat this as an optional path once production complexity grows |
| 17 | Service Mesh | Inter-service governance, mTLS, and traffic control | Istio/Linkerd manages service-to-service calls | 1 | 5 | P3 | Deferred | Not worth it at the current scale |
| 18 | Multi-active and cross-region disaster recovery | Region-level failure recovery | Multi-region deployment, data replication, failover | 1 | 5 | P3 | Deferred | Revisit after the business matures |

## 6. Recommended Rollout Path

### Phase 1: Production Entry Point and Stability Baseline

Goal: give the project a production entry point, basic security boundary, and troubleshooting capability.

Deliverables:

- [x] Introduce Traefik as the unified entry point and expose only application entry `80/443`. Let's Encrypt automatic certificates and manual-certificate Compose overrides are provided; production domain/certificate settings must be configured per environment.
- [x] Make backend, ai-service, browser-worker, PostgreSQL, and Redis internal services.
- [x] Add basic health/readiness endpoints. Completed: frontend already has `/api/health` and `/api/ready`; backend, ai-service, and browser-worker have `/health` and `/ready`; Dockerfiles and production Compose are wired to readiness healthchecks; backend and browser-worker support signal-driven graceful shutdown.
- [x] Add request ID / trace ID to request logs.
- [x] Add basic rate limiting at IP, user, AI API, and browser session levels.
- [x] Organize production environment variables and secret management conventions.

### Phase 2: Asynchronous Publishing and Idempotency Governance

Goal: make the publishing path withstand concurrent requests, repeated clicks, worker restarts, and third-party platform failures.

Deliverables:

- [x] Upgrade the publishing task model to a reliable queue, with Redis Streams or Asynq as the first options. Completed: Asynq + Redis manages publish jobs with ack, failed retry, worker crash recovery, and archive.
- [x] Add idempotency keys to publish requests.
- [x] Clarify the publication state machine: `draft`, `syncing`, `queued`, `publishing`, `succeeded`, `failed`, `cancelled`. Completed: old `pending`, `adapted`, `published`, and `disabled` state names remain as compatibility aliases.
- [x] Add owner, TTL, renewal, and release validation to distributed locks.
- [x] Add retry, backoff, timeout, and circuit breaker to external platform calls. Completed: backend's unified resilience layer covers AI service, WeChat, X, browser-worker, and media-download HTTP calls; HTTP retry is used only for safe methods by default, non-idempotent POST is not replayed automatically; the publishing operation layer breaks circuits by platform but does not retry full publish operations; ai-service LLM client supports timeout, max retries, and stream chunk timeout.
- [x] Write a publish event for each task execution to support auditing and diagnosis.

### Phase 3: Resource Isolation and Horizontal Scaling

Goal: let high-resource modules and normal API modules scale independently.

Deliverables:

- [x] Split backend into two runtime processes: `backend` API service and `publish-worker`.
- [x] Add a global resource pool and user-level concurrency quotas to browser-worker. Completed: backend uses Redis to maintain user/workspace concurrency quotas; browser-worker reserves a global pool slot before starting a Chromium container; containers still keep CPU/memory limits.
- [x] Add user-level concurrency limits and token/cost statistics for AI requests. Completed: normal and streaming AI requests both use `STREAM_GATE_*`/`AI_STREAM_*` concurrency leases; non-streaming AI responses pass through provider token usage and estimated cost based on `LLM_INPUT_COST_PER_1K_TOKENS` and `LLM_OUTPUT_COST_PER_1K_TOKENS`; ai-service exposes `mpp_ai_tokens_total` and `mpp_ai_cost_total` metrics. Streaming AI is still governed mainly by connection concurrency and does not return per-request token details.
- [x] Support multiple backend-api replicas. Completed: API is basically stateless, production Compose defaults to `BACKEND_API_REPLICAS=2`, and dev override fixes it to one replica to avoid port conflicts.
- [x] Apply gateway timeout, connection limits, and token validation uniformly to long-connection APIs. Completed: the remote browser stream entry validates session stream tokens uniformly; WebSocket/websockify long connections use `BROWSER_STREAM_*` connection-count limits; the HTTP reverse proxy's upstream first-byte wait is controlled by `BROWSER_STREAM_GATEWAY_TIMEOUT` and returns 504 on timeout; clients can reconnect while the token is valid.

Data-layer expansion is not covered in this phase and follows the dedicated database plan.

### Phase 4: Media and Workflow Expansion

Goal: support more users, more media, more platforms, and more complex publishing workflows.

Deliverables:

- [x] Move media files to object storage and use signed URLs.
- [ ] Add CDN support for static assets and media.
- [ ] Evaluate Temporal workflow orchestration based on task complexity.
- [x] Evaluate Kubernetes based on deployment complexity. A production Kubernetes path, validation scripts, and operations runbook now exist; actual adoption still depends on the team's deployment complexity.

## 7. P0/P1 Priority List

The most valuable work now is not broad microservice decomposition, but the following high-value improvements:

| Done | Priority | Improvement | Reason |
| ---- | -------- | ----------- | ------ |
| [x] | P0 | Traefik unified entry | Production deployment security boundary and internal service hiding are in place; Let's Encrypt automatic certificates and manual certificates can be selected through Compose overrides |
| [x] | P0 | Rate limits and quotas | Protect AI, browser sessions, and publishing APIs |
| [x] | P0 | Observability baseline | Enables diagnosis and supports stable iteration |
| [x] | P0 | health/readiness | Supports production restarts, monitoring, and load balancing; frontend/backend/ai-service/browser-worker now have baseline health/readiness |
| [x] | P1 | Publishing idempotency | Prevents duplicate publishing and concurrent write conflicts; current implementation supports idempotency keys, idempotent response reuse, publish locks, and state-machine validation |
| [x] | P1 | Reliable queue | Asynq + Redis has replaced Redis List and provides ack, retry, worker crash recovery, and archive |
| [x] | P1 | Stronger distributed locks | Ensures the same publication is not handled concurrently by multiple workers |
| [x] | P1 | External-call circuit breaking and retry | The unified resilience layer covers AI service, WeChat, X, browser-worker, and media-download HTTP calls; HTTP retry is used only for safe methods by default, full publish operations are not retried; ai-service LLM client has timeout/max retries/stream chunk timeout configured |
| [x] | P1 | browser-worker resource pool | User/workspace concurrency quotas, same user/platform active session locks, and a global worker pool are in place to control Chromium container cost and risk |

## 8. Technologies Not Recommended for Early Adoption

| Technology | Reason to defer | When to reconsider |
| ---------- | ---------------- | ------------------ |
| Kubernetes | An optional production deployment path exists, but making it the default still increases operational complexity | After service count, environment count, release frequency, and replica scale grow significantly |
| Service Mesh | Current service-to-service call chains are not complex | After multiple teams, multiple languages, multiple clusters, mTLS, and canary traffic governance become real requirements |
| One microservice per platform | It would make publishing state, accounts, permissions, and transactions more complex | After platform adapter teams are independent, a single platform has massive traffic, or platform-specific failures frequently affect the whole system |

## 9. Conclusion

Considering cost and value, MPP is best served by a "progressive distributed architecture":

- Keep the Go backend as the business core for now and avoid splitting into many business microservices too early.
- Traefik, rate limits, observability, health checks, idempotency, reliable queues, distributed locks, publish-worker, browser-worker resource pools, and object storage already form the main baseline.
- In the next phase, prioritize CDN, fine-grained cache invalidation, finishing read/write splitting, event retention periods, and archiving. The Outbox should expand from publishing tasks to more business events, and CDC should be evaluated only after multi-consumer needs are clear.
- Continue evaluating Temporal, Service Mesh, and multi-region disaster recovery by trigger conditions. Kubernetes remains an optional production path with a base package and runbook already available. Data-layer expansion follows the [database plan](./database-optimization.md).

This route improves concurrency capacity, stability, and business growth capacity while keeping complexity under control.
