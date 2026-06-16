# MPP Tech Stack

This document explains which technologies MPP introduces, what problems they solve, and what role they play in the overall system.

## ai-service

The `ai-service` module owns prompt construction, LLM invocation, and streaming AI editing responses. It stays separate from the Go backend so model-specific logic does not leak into authentication, persistence, or publishing orchestration.

Technology selection rationale: this module uses the Python AI ecosystem because prompt orchestration, chat model integrations, and streaming response handling are more mature there. FastAPI keeps the service lightweight, while LangChain provides a clear provider boundary so the rest of the system does not depend on a specific LLM implementation.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Python | Provides a mature runtime for AI and prompt tooling. | Runtime for the AI service. |
| FastAPI | Needs lightweight HTTP APIs with clean request validation and streaming support. | Exposes content editing, pre-publish editing, and calibration endpoints. |
| Uvicorn | Needs an ASGI server for local and containerized FastAPI execution. | Runs the AI HTTP service in development and production containers. |
| LangChain | Needs reusable abstractions for message construction and chat model calls. | Builds prompt messages and calls the configured LLM provider. |
| langchain-openai | Needs an OpenAI-compatible provider boundary. | Connects MPP to OpenAI-compatible chat completion APIs. |
| Pydantic | Needs strict request schemas for AI editing inputs. | Validates AI request payloads before prompt execution. |
| python-dotenv | Needs local environment configuration without hardcoding secrets. | Loads LLM provider URL, model, and API key settings in development. |
| uv | Needs fast, reproducible Python dependency management. | Installs and locks Python dependencies in local and Docker workflows. |
| pytest | Needs regression tests around AI routes and provider configuration. | Tests AI route behavior, validation, and LLM client setup. |
| Ruff | Needs consistent Python formatting. | Formats AI service source files in pre-commit workflows. |

## collab-service

The `collab-service` module owns real-time collaborative editing traffic. It keeps collaboration transport and document sync separate from the Go backend so the business API can issue sessions and enforce access rules without also hosting the CRDT WebSocket server.

Technology selection rationale: this module stays in the TypeScript editor ecosystem because the frontend editor already uses TipTap and Yjs. Hocuspocus provides the collaboration server boundary, Fastify keeps the HTTP and WebSocket host lightweight, PostgreSQL persists document state, and Redis can coordinate document sync across replicas.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| TypeScript | Needs a type-safe runtime aligned with the editor stack. | Runtime for the collaborative editing service. |
| Fastify and WebSocket support | Needs lightweight health, internal API, metrics, and WebSocket routes. | Hosts collaboration readiness checks, internal document initialization, and live editor connections. |
| Hocuspocus | Needs a maintained Yjs collaboration server instead of custom CRDT transport. | Handles collaborative document connections, updates, awareness, and lifecycle hooks. |
| Yjs | Needs conflict-free collaborative document state. | Synchronizes editor state across active users. |
| TipTap server packages | Needs a server-side document schema compatible with the frontend editor. | Validates and transforms collaborative editor content. |
| PostgreSQL | Needs durable collaboration document persistence. | Stores collaborative document state and project source sync data. |
| Redis | Needs distributed sync for multi-replica collaboration deployments. | Coordinates collaboration updates between service instances. |
| jose | Needs signed collaboration session tokens. | Verifies collaboration sessions issued by the backend. |
| prom-client | Needs collaboration service metrics. | Exposes connection, document, authorization, and persistence metrics. |

## backend

The backend runtime group includes the main Go API, `publish-worker`, and `browser-worker`. Together they own authentication, durable state, publishing orchestration, remote browser coordination, and platform automation.

Technology selection rationale: this layer favors Go because publishing orchestration needs predictable concurrency, typed service boundaries, and straightforward container deployment. The main API uses Echo and GORM for simple HTTP and persistence layers, while Redis and the browser-worker support asynchronous jobs, short-lived locks, and isolated browser automation for platforms that cannot be handled through stable public APIs.

### Backend API

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Go | Needs a fast, concurrent service runtime for API and worker-style orchestration. | Runtime for the backend API, publishing services, and platform adapters. |
| Echo | Needs a lightweight HTTP router and middleware stack. | Hosts dashboard APIs, auth routes, AI proxy routes, publishing routes, and browser-session routes. |
| echo-jwt and golang-jwt | Needs authenticated user-scoped dashboard APIs. | Validates session tokens and protects user operations. |
| GORM | Needs structured persistence without hand-writing every SQL query. | Maps users, projects, platform publications, accounts, and browser sessions to database tables. |
| GORM PostgreSQL driver | Needs durable relational storage for production data. | Connects the backend domain model to PostgreSQL. |
| GORM datatypes | Needs flexible JSON fields for platform-specific configuration and drafts. | Stores dynamic platform data such as `config`, `adapted_content`, cookies, and credentials. |
| go-redis | Needs locks, OAuth state, and short-lived browser-session state. | Coordinates distributed locks, stream tokens, and TTL-based session state. |
| Asynq | Needs reliable Redis-backed background jobs with retry and crash recovery semantics. | Manages publish jobs between backend API and publish-worker without storing browser session URLs or tokens in task payloads. |
| AWS S3-compatible SDK | Needs object storage for uploaded and processed media. | Stores media assets and object references through S3/R2-compatible storage. |
| oapi-codegen | Needs generated Go types from shared API contracts. | Keeps backend and browser-worker request and response models aligned with OpenAPI. |
| gRPC and protobuf | Needs typed internal calls to the content pipeline. | Connects the backend to draft compilation and media processing services. |
| chromedp and cdproto | Needs controlled browser automation for platforms that require web sessions. | Drives Chromium, reads browser state, captures cookies, and supports platform automation. |
| gorilla/websocket | Needs browser stream proxying and bidirectional browser-session traffic. | Supports remote browser session streaming between frontend, backend, and worker paths. |
| golang.org/x/net/html | Needs reliable HTML parsing and conversion. | Powers HTML-to-text and HTML-to-Markdown draft adaptation. |
| google/uuid | Needs stable identifiers across users, projects, publications, and sessions. | Generates and stores UUID-based domain IDs. |
| godotenv | Needs local configuration without hardcoding environment variables. | Loads backend settings during local development. |
| testify, miniredis, and sqlite test driver | Needs isolated backend tests without real external services. | Provides assertions, in-memory Redis behavior, and lightweight database test support. |

### browser-worker and browser runtime

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Go and Echo | Needs a small service dedicated to browser-session lifecycle. | Hosts browser-worker APIs for creating, inspecting, capturing, and stopping browser sessions. |
| Docker SDK for Go | Needs disposable, isolated browser execution environments. | Starts and stops browser runtime containers for remote login and publishing workflows. |
| Kubernetes client | Needs isolated browser runtime Pods in cluster deployments. | Starts and stops browser runtime sessions when Docker is not the runtime driver. |
| Redis | Needs live session state that can expire automatically. | Stores worker session references, status, TTLs, and coordination state. |
| chromedp and cdproto | Needs direct control over remote Chromium sessions. | Configures browser isolation, navigates login pages, detects login state, and captures cookies. |
| Chromium | Needs a real browser for platforms that do not expose stable publishing APIs. | Executes platform login, draft editing, media upload, and publishing steps. |
| Xvfb and Openbox | Needs a visible Linux browser environment inside a container. | Provides a virtual display and window manager for Chromium. |
| x11vnc, noVNC, and websockify | Needs browser UI streaming to the frontend without a local plugin. | Streams the remote browser session to the user through the web application. |

## content-pipeline-service

The `content-pipeline-service` module owns platform draft compilation and media processing. It keeps deterministic content transformation and media optimization behind a typed service boundary instead of scattering those rules across the backend and frontend.

Technology selection rationale: this module uses Rust because draft compilation and media work benefit from strong data modeling, explicit error handling, and predictable resource use. A gRPC boundary keeps the service independent from the Go backend while still giving both sides typed contracts.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Rust | Needs a safe and efficient runtime for media and draft transformation. | Runtime for the content pipeline service and core library. |
| tonic and protobuf | Needs typed internal service contracts. | Exposes draft compiler and media processor gRPC services. |
| Tokio | Needs async service execution. | Runs gRPC traffic, media resolver calls, and object storage operations. |
| Axum | Needs a small HTTP surface alongside gRPC. | Exposes service metrics. |
| scraper and HTML parsing utilities | Needs deterministic source-content parsing. | Converts source content into platform draft formats. |
| image, oxipng, mozjpeg, and webp | Needs media optimization and format handling. | Resizes, converts, and optimizes images for publishing workflows. |
| object_store | Needs provider-neutral object storage access. | Writes processed media to S3/R2-compatible storage. |
| tracing | Needs structured diagnostics. | Emits service logs for content transformation and media processing. |

## frontend

The frontend module owns the dashboard experience: content editing, collaborative editing, platform draft review, AI proposal review, account connection, publishing controls, SaaS-style product presentation, and SEO-friendly entry points.

Technology selection rationale: this module uses the React and TypeScript ecosystem because the product is an interactive SaaS dashboard with complex editor state, streaming AI review, real-time collaboration, platform-specific previews, and discoverable public-facing pages. Next.js provides the application framework and SEO primitives, TipTap handles rich content editing, Yjs handles collaborative document state, and Tailwind-based UI tooling keeps the interface consistent without slowing down feature work.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Next.js | Needs a production-ready React application framework with SaaS-ready routing and SEO support. | Provides routing, application shell, API proxying, metadata support, optimized builds, and standalone container output. |
| React | Needs a component model for interactive dashboard workflows. | Builds content pages, account settings, AI review surfaces, and publishing controls. |
| TypeScript | Needs type safety across API clients, state, and UI contracts. | Defines frontend domain types and catches integration mistakes at build time. |
| Tailwind CSS | Needs fast, consistent utility-first styling. | Styles the dashboard, editor, panels, and responsive layouts. |
| shadcn, Base UI, Radix Slot, CVA, clsx, and tailwind-merge | Needs reusable accessible UI patterns and predictable variants. | Provides UI composition, button/card/input variants, class merging, and component structure. |
| lucide-react | Needs consistent iconography for dashboard controls. | Supplies icons for actions, navigation, and controls. |
| sonner | Needs lightweight user feedback. | Shows toast notifications for save, publish, connection, and error states. |
| next-themes | Needs theme support. | Handles light/dark theme switching. |
| TipTap and TipTap extensions | Needs a rich editor that can produce structured content. | Provides source editing, image/link support, alignment, placeholders, and Markdown interoperability. |
| Yjs and Hocuspocus provider | Needs collaborative editor synchronization. | Connects the frontend editor to live collaboration sessions. |
| react-markdown, remark-gfm, and rehype-sanitize | Needs safe Markdown rendering for AI and platform drafts. | Renders AI proposals and Markdown platform previews while sanitizing unsafe output. |
| diff and react-diff-view | Needs user-reviewable AI edits. | Generates and displays content differences before the user accepts changes. |
| Zustand | Needs local workflow state without pushing every edit to the backend. | Stores editor state, selected platforms, pre-publish drafts, and loading flags. |
| SEO metadata utilities | Needs discoverable pages and consistent search/social previews for a SaaS product. | Centralizes page titles, descriptions, and metadata for SEO-friendly presentation. |
| Vitest and jsdom | Needs frontend unit tests without a browser. | Tests API clients, hooks, navigation, and dashboard workflows. |
| oxlint, oxfmt, and TypeScript compiler | Needs fast linting, formatting, and type checks. | Enforces frontend code quality in local and pre-commit workflows. |
| pnpm | Needs deterministic JavaScript dependency management. | Installs and locks frontend dependencies. |

## extension

The `extension` module owns local browser publishing support for platforms where user-browser context or content-script automation is useful. It complements server-side publishing and remote browser sessions rather than replacing the backend publishing pipeline.

Technology selection rationale: the extension uses WXT because it handles browser-extension build, manifest, entrypoint, and content-script concerns with less custom tooling. React and Tailwind keep the extension UI close to the dashboard's component model, while platform adapters isolate each site's DOM automation rules.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| WXT | Needs a browser extension framework with manifest and entrypoint management. | Builds the background script, side panel, trust-origin page, and content scripts. |
| React and TypeScript | Needs type-safe extension UI and workflow state. | Implements extension screens, adapters, background events, and publishing handoff flows. |
| Tailwind CSS | Needs compact extension styling. | Styles side panel and trust-origin UI. |
| shadcn-style UI primitives | Needs reusable controls inside the extension. | Provides buttons, alerts, cards, badges, and separators. |
| Platform content scripts | Needs site-specific automation for local publishing flows. | Handles supported targets such as Zhihu, X, Douyin, Xiaohongshu, and Bilibili. |
| Vitest and Testing Library | Needs regression tests for extension workflows. | Tests adapters, settings, prepublish behavior, and workbench UI. |

## devops

The DevOps layer wires all services together for local development, self-hosted deployment, and Kubernetes deployment.

Technology selection rationale: the project is polyglot, so DevOps tooling is chosen to keep service boundaries reproducible instead of forcing every module into one runtime. Docker Compose gives the full stack one startup path, PostgreSQL and Redis split durable state from transient coordination, and native package managers keep each module aligned with its ecosystem.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Docker Compose | Needs repeatable multi-service startup. | Runs frontend, backend, publish-worker, browser-worker, browser runtime image, ai-service, content-pipeline-service, collab-service, PostgreSQL, pgBouncer, Redis, and observability services together. |
| Kubernetes | Needs production orchestration with rolling deploys, scheduling, runtime isolation, and managed data service integration. | Deploys long-running app services, runs browser runtime Pods per session, and applies Ingress, RBAC, NetworkPolicy, PDB, and HPA resources. |
| Kustomize | Needs environment-specific Kubernetes configuration without templating secrets into the base package. | Renders app baseline, runtime control, validation, and data-service packages for cluster overlays. |
| Multi-stage Dockerfiles | Needs separate development and production images. | Builds smaller production images while preserving hot-reload development targets. |
| Traefik | Needs an edge router for frontend and collaboration traffic. | Routes public HTTP/TLS traffic and `/collab` WebSocket connections. |
| PostgreSQL | Needs durable, queryable system-of-record storage. | Stores users, projects, platform publications, accounts, credentials, cookies, and browser-session audit records. |
| pgBouncer | Needs database connection pooling for replicated services. | Pools PostgreSQL connections for backend, publish-worker, and collab-service. |
| Redis | Needs fast transient coordination state. | Backs Asynq publish queues, distributed locks, OAuth state, browser session state, stream tokens, and TTL-controlled data. |
| Browser runtime Docker image | Needs reproducible remote browser execution. | Packages Chromium, Xvfb, Openbox, x11vnc, noVNC, websockify, and the CDP proxy into a disposable runtime. |
| Prometheus, Grafana, Loki, and Alloy | Needs metrics, dashboards, and log collection. | Provides baseline observability for local and deployed environments. |
| pnpm, uv, Go modules, and Cargo | Needs module-specific dependency management. | Keeps JavaScript, Python, Go, and Rust dependencies reproducible through their native package managers. |
| Air | Needs live reload for Go services during development. | Rebuilds and restarts the backend API and browser-worker in dev containers. |
| Lefthook | Needs consistent pre-commit formatting across modules. | Runs frontend formatting, Go formatting, and AI-service formatting before commits. |
| `.env` files | Needs environment-specific configuration without committing secrets. | Configures service URLs, database credentials, Redis settings, JWT secrets, LLM provider settings, and OAuth credentials. |
