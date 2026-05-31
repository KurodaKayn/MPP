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

## backend

The backend includes the main Go API and the `browser-worker`. Together they own authentication, durable state, publishing orchestration, remote browser coordination, and platform automation.

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
| go-redis | Needs queues, locks, OAuth state, and short-lived browser-session state. | Coordinates asynchronous publishing, distributed locks, stream tokens, and TTL-based session state. |
| chromedp and cdproto | Needs controlled browser automation for platforms that require web sessions. | Drives Chromium, reads browser state, captures cookies, and supports platform automation. |
| gorilla/websocket | Needs browser stream proxying and bidirectional browser-session traffic. | Supports remote browser session streaming between frontend, backend, and worker paths. |
| golang.org/x/net/html | Needs reliable HTML parsing and conversion. | Powers HTML-to-text and HTML-to-Markdown draft adaptation. |
| golang.org/x/image | Needs image decoding and compression for platform media upload. | Processes images before upload to platforms such as WeChat. |
| google/uuid | Needs stable identifiers across users, projects, publications, and sessions. | Generates and stores UUID-based domain IDs. |
| godotenv | Needs local configuration without hardcoding environment variables. | Loads backend settings during local development. |
| testify, miniredis, and sqlite test driver | Needs isolated backend tests without real external services. | Provides assertions, in-memory Redis behavior, and lightweight database test support. |

### browser-worker and browser runtime

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Go and Echo | Needs a small service dedicated to browser-session lifecycle. | Hosts browser-worker APIs for creating, inspecting, capturing, and stopping browser sessions. |
| Docker SDK for Go | Needs disposable, isolated browser execution environments. | Starts and stops browser runtime containers for remote login and publishing workflows. |
| Redis | Needs live session state that can expire automatically. | Stores worker session references, status, TTLs, and coordination state. |
| chromedp and cdproto | Needs direct control over remote Chromium sessions. | Configures browser isolation, navigates login pages, detects login state, and captures cookies. |
| Chromium | Needs a real browser for platforms that do not expose stable publishing APIs. | Executes platform login, draft editing, media upload, and publishing steps. |
| Xvfb and Openbox | Needs a visible Linux browser environment inside a container. | Provides a virtual display and window manager for Chromium. |
| x11vnc, noVNC, and websockify | Needs browser UI streaming to the frontend without a local plugin. | Streams the remote browser session to the user through the web application. |

## frontend

The frontend module owns the dashboard experience: content editing, platform draft review, AI proposal review, account connection, publishing controls, SaaS-style product presentation, and SEO-friendly entry points.

Technology selection rationale: this module uses the React and TypeScript ecosystem because the product is an interactive SaaS dashboard with complex editor state, streaming AI review, platform-specific previews, and discoverable public-facing pages. Next.js provides the application framework and SEO primitives, TipTap handles rich content editing, and Tailwind-based UI tooling keeps the interface consistent without slowing down feature work.

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
| react-markdown, remark-gfm, and rehype-sanitize | Needs safe Markdown rendering for AI and platform drafts. | Renders AI proposals and Markdown platform previews while sanitizing unsafe output. |
| diff and react-diff-view | Needs user-reviewable AI edits. | Generates and displays content differences before the user accepts changes. |
| Zustand | Needs local workflow state without pushing every edit to the backend. | Stores editor state, selected platforms, pre-publish drafts, and loading flags. |
| SEO metadata utilities | Needs discoverable pages and consistent search/social previews for a SaaS product. | Centralizes page titles, descriptions, and metadata for SEO-friendly presentation. |
| Vitest and jsdom | Needs frontend unit tests without a browser. | Tests API clients, hooks, navigation, and dashboard workflows. |
| oxlint, oxfmt, and TypeScript compiler | Needs fast linting, formatting, and type checks. | Enforces frontend code quality in local and pre-commit workflows. |
| pnpm | Needs deterministic JavaScript dependency management. | Installs and locks frontend dependencies. |

## devops

The DevOps layer wires all services together for local development and containerized deployment.

Technology selection rationale: the project is polyglot, so DevOps tooling is chosen to keep service boundaries reproducible instead of forcing every module into one runtime. Docker Compose gives the full stack one startup path, PostgreSQL and Redis split durable state from transient coordination, and native package managers keep each module aligned with its ecosystem.

| Technology | Problem solved | Role in MPP |
| --- | --- | --- |
| Docker Compose | Needs repeatable multi-service startup. | Runs frontend, backend, browser-worker, browser runtime image, ai-service, PostgreSQL, and Redis together. |
| Multi-stage Dockerfiles | Needs separate development and production images. | Builds smaller production images while preserving hot-reload development targets. |
| PostgreSQL | Needs durable, queryable system-of-record storage. | Stores users, projects, platform publications, accounts, credentials, cookies, and browser-session audit records. |
| Redis | Needs fast transient coordination state. | Stores publish queues, locks, OAuth state, browser session state, stream tokens, and TTL-controlled data. |
| Browser runtime Docker image | Needs reproducible remote browser execution. | Packages Chromium, Xvfb, Openbox, x11vnc, noVNC, websockify, and the CDP proxy into a disposable runtime. |
| pnpm, uv, and Go modules | Needs module-specific dependency management. | Keeps JavaScript, Python, and Go dependencies reproducible through their native package managers. |
| Air | Needs live reload for Go services during development. | Rebuilds and restarts the backend API and browser-worker in dev containers. |
| Lefthook | Needs consistent pre-commit formatting across modules. | Runs frontend formatting, Go formatting, and AI-service formatting before commits. |
| `.env` files | Needs environment-specific configuration without committing secrets. | Configures service URLs, database credentials, Redis settings, JWT secrets, LLM provider settings, and OAuth credentials. |
