# Authenticated Extension Workbench Plan

## Status

Confirmed planning direction.

## Goal

Move the browser extension from a purely external handoff receiver to an authenticated publishing workbench. The extension should reuse the user's existing MPP Web login state, request pre-publish content from the backend, allow the user to choose target platforms, request a backend-generated extension handoff, and then reuse the current DOM injection adapters to prepare drafts on platform creator pages.

The backend owns authentication, user-scoped data access, pre-publish selection, platform capability mapping, handoff generation, asset URL preparation, and event callback validation. The extension owns the user-facing workbench UI, backend API calls, platform selection, handoff execution, adapter injection, local execution monitoring, and compatibility with the existing bridge handoff flow.

## Current Context

The extension currently accepts handoffs from trusted MPP Web origins through the content-script bridge. The background script validates the handoff, stores it in session storage, opens platform publishing tabs, injects the selected adapter, records execution events, and sends callback events when a callback endpoint is present.

The backend is a Go API built with Echo. User dashboard routes are mounted under `/api/user/dashboard` and protected by JWT middleware. The JWT middleware already supports tokens from an `Authorization` bearer header and from these cookies:

- `sevenoxcloud.auth_token`
- `auth_token`
- `access_token`

The frontend login client stores the token in browser storage and also writes `sevenoxcloud.auth_token` as a cookie. This makes it possible to reuse Web login state if extension requests can carry the cookie to the API layer.

## Chosen Direction

Use option A: the extension requests backend data while reusing the MPP Web login state.

Within option A, the selected architecture is backend-generated handoff. The extension should not reconstruct handoff business data from raw project and publication records. Instead, the extension asks the backend for an executable `ExtensionPublishHandoff`.

This keeps platform-specific business decisions in the backend:

- Which projects are available to the current user.
- Which publications are ready for extension publishing.
- Which platform maps to which adapter key.
- Which inject URL should be used.
- Which content kind should be emitted.
- Which assets should be exposed to the extension.
- Which callback URL and callback token should be used.

The extension remains an execution layer that displays choices, starts handoffs, injects adapters, and reports events.

## Branch Strategy

Use separate branches and pull requests so backend and extension work can be reviewed independently.

Backend branch:

```text
feature/backend-extension-handoff-api
```

Extension branch:

```text
feature/extension-handoff-workbench
```

Recommended sequence:

1. Build and merge the backend API contract first.
2. Build the extension workbench against the merged backend contract.
3. If parallel development is needed, build the extension with a mock API client and switch to the real backend once the contract stabilizes.

## End-to-End Flow

```text
User signs in to MPP Web
        |
        v
Extension opens its workbench UI
        |
        v
Extension checks backend session using Web login state
        |
        v
Extension fetches pre-publishable projects and platform drafts
        |
        v
User selects a project and one or more platforms
        |
        v
Extension requests a backend-generated extension handoff
        |
        v
Backend returns ExtensionPublishHandoff
        |
        v
Extension stores the handoff and starts platform tab injection
        |
        v
Platform adapter prepares the draft and stops at user review
        |
        v
Extension records events and sends callback events to backend
```

## Backend Plan

### Phase 1: Extension Origin and Session Compatibility

Add backend support for extension-origin requests when the API is called directly from the browser extension.

Configuration:

```text
EXTENSION_ALLOWED_ORIGINS=chrome-extension://<id>,http://localhost:3000
```

Requirements:

- Allow configured extension origins only.
- Allow credentials for authenticated extension requests.
- Avoid wildcard CORS when credentials are enabled.
- Keep dev and production extension IDs configurable.
- Keep existing dashboard JWT middleware as the source of user identity.

Important validation:

- Confirm whether the current Web login cookie is available to direct extension requests.
- If direct cookie transport is not reliable for a deployment environment, route extension calls through the frontend proxy or adjust cookie settings intentionally.

### Phase 2: Extension Session API

Add:

```text
GET /api/user/dashboard/extension/session
```

Purpose:

- Let the extension determine whether the current browser has an authenticated MPP session.
- Return minimal user information when authenticated.
- Return `401` when the Web login state is unavailable or expired.

Example response:

```json
{
  "authenticated": true,
  "user": {
    "id": "user-uuid",
    "username": "creator"
  }
}
```

### Phase 3: Extension Pre-Publish List API

Add:

```text
GET /api/user/dashboard/extension/prepublish
```

Purpose:

- Return a compact list of projects and platform drafts that are available for extension publishing.
- Scope all results to the authenticated user.
- Avoid requiring the extension to stitch together multiple project and publication endpoints.

Example response:

```json
{
  "items": [
    {
      "project_id": "project-uuid",
      "title": "Article title",
      "status": "draft",
      "updated_at": "2026-06-03T10:00:00Z",
      "platforms": [
        {
          "publication_id": "publication-uuid",
          "platform": "douyin",
          "adapter_key": "DYNAMIC_DOUYIN",
          "content_kind": "article",
          "status": "pending",
          "enabled": true,
          "preview": "First 80 characters of the adapted draft"
        }
      ]
    }
  ]
}
```

### Phase 4: Extension Handoff API

Add:

```text
POST /api/user/dashboard/extension/handoffs
```

Request:

```json
{
  "project_id": "project-uuid",
  "platforms": ["douyin"]
}
```

Response:

Return the existing extension handoff protocol:

```json
{
  "schema_version": 1,
  "type": "mpp.extension_publish_handoff",
  "execution_id": "execution-uuid",
  "expires_at": "2026-06-03T10:10:00Z",
  "project": {
    "id": "project-uuid",
    "title": "Article title"
  },
  "platforms": [
    {
      "platform": "douyin",
      "adapter_key": "DYNAMIC_DOUYIN",
      "inject_url": "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
      "content_kind": "article",
      "auto_publish": false,
      "requires_review": true,
      "adapted_content": {
        "schema_version": 1,
        "format": "text",
        "text": "Adapted article body"
      },
      "assets": [],
      "callback": {
        "url": "https://mpp.example.com/api/user/dashboard/extension/events",
        "token": "one-time-callback-token"
      }
    }
  ]
}
```

Backend responsibilities:

- Validate that the project belongs to the authenticated user.
- Validate that requested platforms are enabled for the project.
- Validate that each requested publication has usable adapted content.
- Map platform values to adapter keys, content kinds, target formats, and inject URLs.
- Generate `execution_id`.
- Generate `expires_at`.
- Generate a short-lived callback token.
- Return only data needed by the extension.

### Phase 5: Extension Event Callback API

Add:

```text
POST /api/user/dashboard/extension/events
```

Purpose:

- Receive execution events from the extension.
- Authenticate the event with the callback token generated in the handoff.
- Record extension execution state.
- Optionally update publication status or store an execution log.

Request shape should match the current callback payload:

```json
{
  "token": "one-time-callback-token",
  "event_id": "event-uuid",
  "platform": "douyin",
  "status": "user_review",
  "message": "Article draft prepared. Review platform settings.",
  "remote_id": "",
  "publish_url": "",
  "error_message": "",
  "metadata": {}
}
```

Callback behavior:

- Accept known execution tokens only.
- Reject expired or unknown tokens.
- Deduplicate by `event_id` when possible.
- Keep failure events visible to users and maintainers.

## Extension Plan

### Phase 1: Backend API Client

Add a small backend client layer:

```text
extension/src/backend/client.ts
extension/src/backend/config.ts
extension/src/backend/types.ts
```

Responsibilities:

- Store the backend or frontend API base URL.
- Send authenticated requests with `credentials: "include"`.
- Fetch session state.
- Fetch pre-publish list data.
- Request backend-generated handoffs.
- Normalize backend errors for UI display.

### Phase 2: Session State UI

The extension workbench should start by checking:

```text
GET /api/user/dashboard/extension/session
```

States:

- Authenticated: show pre-publish content.
- Unauthenticated: show an action to open the MPP Web login page.
- API unavailable: show a backend connection error.
- Expired session: show a re-login prompt.

The extension should not add a username/password login form in this direction.

### Phase 3: Pre-Publish Workbench UI

Extend the current publish monitor into a workbench.

Suggested sections:

- Session status.
- Pre-publish project list.
- Project details.
- Platform selection.
- Start handoff action.
- Existing execution monitor.
- Existing event timeline.

The UI should make unsupported or disabled platforms visible but non-actionable.

### Phase 4: Start Backend Handoff

Add a background message such as:

```ts
{
  type: "extension.start_handoff",
  handoff: ExtensionPublishHandoff
}
```

Flow:

1. User selects project and platforms.
2. Extension calls `POST /api/user/dashboard/extension/handoffs`.
3. Extension validates the returned handoff with the existing handoff validator.
4. Extension stores the accepted handoff.
5. Extension records accepted events.
6. Extension starts publishing tabs through the existing `startPublishingTabs` flow.

### Phase 5: Preserve Current Bridge Compatibility

Keep the existing bridge flow during the first version:

- `bridge.detect`
- `bridge.request_trust`
- `bridge.publish_handoff`
- trusted origins

This preserves local testing and avoids breaking existing integration while the authenticated workbench is introduced.

## Asset Handling

The backend should return asset URLs that the extension can download without requiring the content script to access private application state.

Preferred approach:

- Backend returns short-lived signed asset URLs in the handoff.
- Extension background downloads assets through the existing `asset.download` message path.
- Content scripts receive `File` objects, not private backend credentials.

Example asset:

```json
{
  "type": "image",
  "source_url": "https://mpp.example.com/api/user/dashboard/extension/assets/signed/asset-token",
  "name": "cover.png",
  "mime_type": "image/png"
}
```

## Testing Plan

### Backend Tests

Cover:

- Extension session endpoint returns `401` without authentication.
- Extension session endpoint returns current user when authenticated.
- Pre-publish endpoint returns only the current user's projects.
- Pre-publish endpoint excludes unsupported or unavailable records as intended.
- Handoff endpoint rejects projects owned by another user.
- Handoff endpoint rejects disabled platforms.
- Handoff endpoint rejects missing adapted content.
- Handoff endpoint returns a valid `ExtensionPublishHandoff`.
- Event endpoint accepts valid callback tokens.
- Event endpoint rejects invalid or expired callback tokens.
- Event endpoint deduplicates repeated `event_id` values when storage supports it.

### Extension Tests

Cover:

- Unauthenticated session state displays login guidance.
- Authenticated session state loads pre-publish items.
- Backend errors are displayed clearly.
- Platform selection sends the expected handoff request.
- Returned handoff starts the existing publishing flow.
- Existing bridge handoff flow remains compatible.
- Douyin adapter tests remain green.

## Risks and Open Decisions

### Cookie Transport

The selected direction depends on the extension being able to make authenticated requests using the existing Web login state. This must be tested early in local and production-like environments.

If direct extension-to-backend cookie transport is unreliable, the fallback is to route extension calls through the frontend API proxy while keeping the same backend API contract.

### CORS Configuration

Extension origins must be configured explicitly. The backend must not use wildcard CORS with credentials.

### Callback Persistence

The backend needs a place to store callback tokens and execution events. Redis is a good fit for short-lived callback tokens. Database storage is better for audit-friendly execution history.

### Cover Images

The current handoff asset model does not distinguish body images from cover images. If Douyin article cover upload becomes required, the handoff asset model should add a usage field such as:

```json
{
  "usage": "cover"
}
```

## Milestones

### Milestone 1: Backend Contract

Branch:

```text
feature/backend-extension-handoff-api
```

Deliverables:

- Extension CORS configuration.
- Session API.
- Pre-publish API.
- Handoff API.
- Event callback API.
- Backend tests.
- API contract documentation.

### Milestone 2: Extension Workbench Shell

Branch:

```text
feature/extension-handoff-workbench
```

Deliverables:

- Backend API client.
- Session status UI.
- Login guidance.
- Pre-publish list UI.
- Platform selection UI.

### Milestone 3: Handoff Execution

Deliverables:

- Request backend-generated handoff.
- Store accepted handoff.
- Start platform tab injection.
- Reuse existing adapter flow.
- Keep current monitor timeline.

### Milestone 4: Event Feedback Loop

Deliverables:

- Send accepted, opening, injecting, review-ready, and failure events to backend.
- Display callback failures.
- Confirm backend and extension status stay aligned.

## Non-Goals for the First Version

- Do not add extension username/password login.
- Do not remove the existing bridge handoff path.
- Do not auto-publish on platform pages.
- Do not redesign every platform adapter.
- Do not solve cover image semantics unless required by the first backend handoff contract.

