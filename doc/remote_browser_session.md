# Remote Browser Session Design

## Goal

Let users connect cookie-based publishing platforms without browser extensions or manual cookie copying.

User signs in inside an isolated remote Chromium session. The backend reads cookies from that controlled browser context through Chrome DevTools Protocol, stores them securely, then reuses them for draft creation or publishing automation.

## Non-Goals

- No browser extension.
- No scraping cookies from the user's real local browser profile.
- No user-entered platform password inside MPP forms.
- No shared browser profile between users.
- No silent publishing when platform UI requires user review.

## Recommended Stack

| Area | Technology | Why |
| --- | --- | --- |
| Browser runtime | Chromium in Docker | Isolated, reproducible, disposable per session. |
| Browser control | Existing Go `chromedp` + remote CDP | Backend already uses chromedp, but remote sessions should connect to the worker-owned browser instead of launching local backend Chromium. |
| Visual remote access | noVNC + websockify + x11vnc, or KasmVNC | Browser UI streams to user's web page without extension. |
| Virtual display | Xvfb | Runs visible Chromium inside headless server/container. |
| Session manager | Separate `browser-worker` service | Creates browser sessions, tracks TTL, stops containers, and owns CDP connectivity. Backend API remains a thin authenticated coordinator. |
| Queue / locking | PostgreSQL first; Redis later if needed | Prevents multiple live sessions per user/platform with a partial unique index. |
| Secret storage | Encrypted cookie store over `PlatformAccount.Cookies` | Existing model has cookie JSON, but encryption/decryption must sit behind a service boundary before MVP stores real cookies. |
| Reverse proxy | Existing frontend API proxy / Nginx / Caddy | Routes session WebSocket and HTTP control endpoints without exposing raw CDP or VNC ports. |

Preferred MVP: **one browser container per connection session**.

## Architecture

![Remote browser session architecture](./assests/remote_browser_session_architecture.svg)

## User Flow

1. User clicks `Connect` on a platform card.
2. Backend creates `remote_browser_sessions` row with `pending` status.
3. Backend asks `browser-worker` to start a browser container with:
   - isolated user data directory
   - adapter network policy, not only an initial allowlist URL
   - CDP endpoint reachable only by `browser-worker` on a private network
   - VNC/WebSocket endpoint reachable only through the authenticated backend stream proxy
4. Frontend opens remote browser modal.
5. Chromium navigates to platform login page.
6. User logs in or scans official QR code.
7. `browser-worker` polls login state through CDP.
8. When required cookies exist, backend stores sanitized encrypted cookie JSON through the cookie store.
9. Backend marks platform account `connected`.
10. Backend stops browser container after success or TTL expiry.

## Session States

| State | Meaning |
| --- | --- |
| `pending` | Session record created. Container not ready. |
| `ready` | Browser stream available. User can interact. |
| `login_detected` | Required cookies or account marker found. |
| `capturing` | Backend reading cookies/profile metadata. |
| `connected` | Cookies saved. Account usable. |
| `expired` | TTL ended before login. |
| `failed` | Container, CDP, or validation error. |
| `revoked` | User disconnected account. Cookies deleted. |

## Data Model

Add table:

```sql
CREATE TABLE remote_browser_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  platform text NOT NULL,
  status text NOT NULL,
  worker_session_ref text NOT NULL DEFAULT '',
  container_id text NOT NULL DEFAULT '',
  cdp_endpoint_ref text NOT NULL DEFAULT '',
  stream_endpoint_ref text NOT NULL DEFAULT '',
  connect_token_hash text NOT NULL,
  error_message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  completed_at timestamptz
);

CREATE INDEX idx_remote_browser_sessions_user_platform
ON remote_browser_sessions (user_id, platform, status);

CREATE UNIQUE INDEX ux_remote_browser_sessions_active_user_platform
ON remote_browser_sessions (user_id, platform)
WHERE status IN ('pending', 'ready', 'login_detected', 'capturing');
```

Reuse `platform_accounts.cookies` for saved cookies, but do not let publishers or handlers read raw storage directly. Add a cookie-store boundary that decrypts, validates, and returns normalized cookie arrays.

Suggested storage envelope:

```json
{
  "version": 1,
  "alg": "AES-256-GCM",
  "kid": "cookie-encryption-key",
  "nonce": "...",
  "ciphertext": "..."
}
```

Suggested decrypted cookie payload:

```json
[
  {
    "name": "sessionid",
    "value": "...",
    "domain": ".douyin.com",
    "path": "/",
    "expires": 1780000000,
    "secure": true,
    "httpOnly": true,
    "sameSite": "None"
  }
]
```

## API

### Start Session

`POST /api/user/dashboard/settings/platforms/:platform/browser-session`

Response:

```json
{
  "session_id": "uuid",
  "status": "pending",
  "stream_url": "/api/user/dashboard/browser-sessions/uuid/stream?token=...",
  "expires_at": "2026-05-30T12:00:00Z"
}
```

### Get Session

`GET /api/user/dashboard/browser-sessions/:id`

Response:

```json
{
  "session_id": "uuid",
  "platform": "douyin",
  "status": "ready",
  "message": "Waiting for login"
}
```

### Complete Session

`POST /api/user/dashboard/browser-sessions/:id/complete`

Backend validates required cookies. Frontend may call this after user clicks `I have signed in`.

### Cancel Session

`DELETE /api/user/dashboard/browser-sessions/:id`

Stops container and marks session `expired` or `failed`.

## Platform Adapter Contract

Each cookie-based platform needs one adapter:

```go
type RemoteBrowserPlatformAdapter interface {
    Platform() string
    LoginURL() string
    AllowedDomains() []DomainRule
    RequiredCookies() []CookieRequirement
    DetectLogin(ctx context.Context, cdp BrowserCDP) (RemoteLoginState, error)
    ExtractAccount(ctx context.Context, cdp BrowserCDP) (RemoteAccountProfile, error)
}
```

`AllowedDomains` must include real login dependencies such as first-party login hosts, QR-code endpoints, captcha domains, and required static/CDN domains. It is not enough to validate only the initial navigation URL.

Example requirements:

| Platform | Login URL | Required Signal |
| --- | --- | --- |
| Douyin | `https://creator.douyin.com/creator-micro/home` | creator page reachable plus required session cookies present |
| Zhihu | `https://www.zhihu.com/signin` | user avatar/account endpoint reachable plus required session cookies present |

## Browser Container

Container process set:

- `Xvfb :99`
- `openbox` or minimal window manager
- `chromium --remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 --user-data-dir=/tmp/profile --no-first-run --no-default-browser-check`
- `x11vnc -display :99 -localhost -forever`
- `websockify 6080 localhost:5900`
- noVNC static web client

Network:

- Browser can access internet.
- CDP port is never published to the host or frontend. It is reachable only from `browser-worker` through a private Docker or Kubernetes network.
- VNC/WebSocket is reachable only through the authenticated backend stream proxy.
- Container egress blocks internal service ranges, link-local metadata endpoints, and non-adapter domains where practical.
- CDP request interception enforces adapter allowlists during navigation and login. Container network policy is the second line of defense.

Resource limits:

- CPU: `0.5` to `1.0`
- Memory: `768MB` to `1.5GB`
- Session TTL: `10` to `15` minutes
- One active session per user/platform

## Security Requirements

- Require authenticated user for every session endpoint.
- Generate single-use stream token. TTL less than session TTL.
- Never expose raw CDP URL to frontend.
- Never expose raw VNC port publicly.
- Store cookies encrypted at rest in Phase 1.
- Decrypt cookies only inside a trusted backend or browser-worker service boundary. Never log decrypted cookies or return them through API responses.
- Delete browser profile after session end.
- Block clipboard read from remote browser unless explicitly needed.
- Disable file downloads or store them in disposable temp storage.
- Enforce navigation and request allowlists through CDP interception plus container egress policy. Initial URL validation alone is insufficient.
- Audit log: session start, success, failure, cancel, cookie refresh.
- User can disconnect platform and delete cookies.

## Cookie Handling

Capture with CDP from controlled browser context after login. Include `HttpOnly` cookies. Do not use `document.cookie`.

Normalize before storage:

- keep only required platform domains
- keep only required cookie names when possible
- drop expired cookies
- preserve `domain`, `path`, `expires`, `secure`, `httpOnly`, `sameSite`

Implementation boundary:

- `CookieStore.Save(userID, platform, cookies)` normalizes and encrypts before updating `PlatformAccount.Cookies`.
- `CookieStore.Load(userID, platform)` decrypts and returns validated cookie arrays.
- Existing publishers should receive decrypted cookie arrays through service code. They should not parse encrypted `PlatformAccount.Cookies` directly.

Refresh strategy:

- Try saved cookies before publishing.
- If login page appears, mark account `failed` with reconnect required.
- Do not auto-open remote session during scheduled publishing.

## Publishing Flow After Connection

1. Backend loads saved cookies.
2. Cookie store decrypts and validates cookies.
3. Existing publisher uses decrypted cookies with chromedp.
4. Publisher creates draft.
5. If platform requires review, return `manual_review_required` with remote draft URL.
6. UI shows `Open draft` action.

## Frontend UX

Settings card:

- `Connect`
- `Connected as <account>`
- `Reconnect`
- `Disconnect`
- `Last checked`

Remote modal:

- large browser viewport
- top status bar: platform, session time left, connection state
- actions: `I have signed in`, `Cancel`
- no instructions about cookies

Failure messages:

- `Login not detected`
- `Session expired`
- `Platform changed its login flow`
- `Cookies saved but publishing test failed`

## Implementation Plan

### Phase 1: MVP

- Add `remote_browser_sessions` table.
- Add partial unique index for one active session per user/platform.
- Add encrypted cookie store over `PlatformAccount.Cookies`.
- Add `browser-worker` session manager interface.
- Add Docker image for remote browser runtime.
- Add private CDP connectivity from `browser-worker` to browser containers.
- Add CDP request interception and adapter domain allowlists.
- Add start/get/complete/cancel APIs.
- Add one adapter for Douyin.
- Add settings modal with noVNC iframe.
- Save encrypted cookies into `PlatformAccount.Cookies`.
- Reuse existing Douyin publisher with cookie-store-loaded cookies.

### Phase 2: Hardening

- Add session cleanup worker.
- Add per-session resource limits.
- Add audit logs.
- Add adapter tests with mocked CDP.
- Add reconnect-required status in UI.

### Phase 3: More Platforms

- Add Zhihu adapter.
- Add Xiaohongshu adapter.
- Add reusable draft automation contract.
- Add manual review handoff URLs.

## Open Decisions

| Decision | Recommendation |
| --- | --- |
| noVNC vs KasmVNC | Use noVNC for MVP. Switch to KasmVNC only if performance or mobile input is poor. |
| Browser worker location | Separate `browser-worker` service. Keep backend API thin and reuse the worker for publishing later if local backend Chromium becomes unreliable. |
| Cookie encryption | Application-level AES-GCM using `COOKIE_ENCRYPTION_KEY`, implemented in Phase 1 before storing real platform cookies. |
| Container orchestration | Docker Compose for dev; Kubernetes Jobs/Pods for production. |
| Session persistence | Persist cookies only. Delete browser profile after connect. |

## Acceptance Criteria

- User connects Douyin without copying cookies.
- User sees official login page inside remote browser modal.
- Backend captures required cookies after login.
- Cookies are saved on `PlatformAccount`.
- Remote container stops after success, cancel, or TTL.
- Reconnect is required when cookies expire.
- No browser extension is required.
