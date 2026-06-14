# New Publishing Platform Development Guide

This guide explains how to add a new publishing platform to MPP. A platform is not a single config entry. It is a cross-module contract: project selection, draft compilation, account connection, publishing execution, frontend display, and extension or remote-browser capability must all use the same `platform` key.

## Goal

After a new platform is integrated:

- Users can select the platform on a project.
- Pre-publish sync can generate platform-specific `AdaptedContent`.
- The publishing flow can resolve the platform publisher, or clearly enter a manual or remote-browser flow.
- Account connection status is visible to both UI and backend.
- API contract, generated code, tests, and docs are updated together.

## Decide Publishing Mode First

Choose the platform integration mode before writing code. This keeps platform rules in the right module.

| Question                                                                      | Decision                      | Main integration points                                                  |
| ----------------------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------ |
| Platform has a stable official publishing API                                 | Prefer API publishing         | Go `PlatformPublisher`, account credentials, official SDK or HTTP client |
| Platform has no stable API, but supports server-side browser login/publishing | Remote-browser publishing     | `RemoteBrowserPlatformAdapter`, `browser-worker`, cookie storage         |
| Platform is better controlled from user's local browser DOM                   | Extension publishing          | WXT content script, handoff, extension callback                          |
| Platform only needs draft preview, not auto-publishing                        | Pre-publish/manual publishing | Content Pipeline, frontend preview, status copy                          |

Check [platform.md](platform.md) for the current publishing-method classification. Platform capabilities change; re-check official docs, account qualification, review requirements, and rate limits before implementation.

## Platform Key Rules

- Use a stable lowercase key, such as `mastodon` or `bilibili`.
- Do not use display names, brand variants, or region labels as keys unless they represent different APIs or publishing semantics.
- The same key must be used across `contracts`, Go, Rust, frontend, extension, and test fixtures.
- After a key is persisted, renaming it requires migrating `project_platform_publications`, `platform_accounts`, `remote_browser_sessions`, and related data.

## Integration Overview

| Module                     | Required work                                                                                                            |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `contracts`                | Update `PublishPlatform` enum; add account/browser/publish schemas and paths if needed; regenerate code                  |
| `content-pipeline-service` | Register draft profile; implement platform draft compilation; register media profile; add snapshot and version docs      |
| `backend`                  | Add platform allowlist entry; register publisher; connect credentials or cookies; wire publishing service, routes, tests |
| `browser-worker`           | If remote-browser script is needed, add internal publish route, request schema, DOM automation                           |
| `frontend`                 | Add platform tab, icon, i18n, account card, pre-publish preview, publish-button behavior                                 |
| `extension`                | If extension publishing is needed, add capability, adapter key, content script, host permission, UI config               |
| `deploy/docs`              | Add OAuth/env/secrets, runtime params, platform docs, operations notes                                                   |

## 1. Update API Contract

Update contract first, then regenerate typed code for all affected services.

Required:

- Add the new key to `PublishPlatform` in `contracts/components/content.yaml`.
- If the platform has account configuration APIs, add account response, save request, and test response schemas in `contracts/components/account.yaml`.
- If the platform needs a `browser-worker` script, add request schema and internal path in `contracts/components/browser-worker.yaml` and `contracts/paths/browser-worker.yaml`.
- If adding user-facing APIs, expose them through the relevant `contracts/paths/*.yaml` and `contracts/views/*.openapi.yaml`.

Generate:

```sh
sh contracts/generate.sh
```

Do not edit generated files manually:

- `backend/internal/contracts/openapi.gen.go`
- `frontend/src/lib/dashboard/api/generated.ts`
- `browser-worker/internal/contracts/openapi.gen.go`
- `ai-service/contract_schemas.py`

See [openAPI-code-generation-guide.md](openAPI-code-generation-guide.md).

## 2. Integrate Content Pipeline

Draft profiles are the compatibility boundary for platform content formats. The frontend editor should not understand platform-specific formats directly.

Drafts:

- Register `<platform>@v1` in `content-pipeline-service/crates/content-pipeline-core/src/drafts/profiles.rs`.
- Add the compile branch in `content-pipeline-service/crates/content-pipeline-core/src/drafts.rs`, returning `AdaptedContent` JSON.
- Use an existing format: `html`, `markdown`, or `text`, unless `DraftFormat` is extended first in `contracts/components/content.yaml`.
- Add a snapshot fixture: `content-pipeline-service/crates/content-pipeline-core/tests/fixtures/draft_snapshots/<platform>.json`.
- Update Current Draft Profiles and Changelog in `content-pipeline-service/PROFILE_VERSIONS.md`.

Media:

- Register media profile in `content-pipeline-service/crates/content-pipeline-core/src/media/profiles.rs`.
- Define `max_bytes`, whether output should compress to the limit, and allowed output MIME types.
- When a publisher uploads images, use `media.DownloadAndProcessForPlatform(source, "<platform>", "<usage>")` instead of hard-coded generic compression rules.
- Update Current Media Profiles and Changelog in `PROFILE_VERSIONS.md`.

Verify:

```sh
cd content-pipeline-service
cargo test -p content-pipeline-core
```

## 3. Integrate Backend Project Selection

Backend allowlist controls whether a project can select a platform.

Required:

- Add the key to `allowedProjectPlatforms` in `backend/internal/services/project/service.go`.
- Confirm `NormalizeProjectPlatforms` still handles duplicates, empty values, and invalid keys correctly.
- Add or update tests under `backend/internal/services/project` for create, update, and save-platform flows.

If only frontend tabs are updated, but the allowlist is not, backend returns `ErrInvalidProject`.

## 4. Integrate Backend Publisher

Every auto-publishing platform must implement `PlatformPublisher`:

```go
type PlatformPublisher interface {
	ValidateConfig(config []byte) error
	Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error)
}
```

Recommended:

- Add `backend/internal/publisher/platforms/<platform>/`.
- Parse `pub.AdaptedContent` in the publisher, accepting only fields emitted by Content Pipeline `AdaptedContent`.
- Keep `ValidateConfig` limited to platform publish config validation; it should not call external services.
- Make `Publish` return remote content ID and accessible URL; failure errors should include platform name and actionable cause.
- Register in `backend/internal/publisher/factory.go`: `Factory.Register("<platform>", &<Platform>Publisher{})`.
- Test parsing, validation, and publishing error branches.

Publishing service resolves publishers through `publisher.Factory.GetPublisher(platform)` in `backend/internal/services/publish/service.go`. Missing registration returns `no publisher registered for platform: <platform>`.

## 5. Integrate Account Model

Choose the smallest account implementation that matches the platform auth mode.

Official API/OAuth:

- Add platform account service file under `backend/internal/services/platform_account/`.
- Define credentials JSON with only fields needed for publishing.
- Provide `GetWorkspace<Platform>Account`, `UpsertWorkspace<Platform>Account`, `TestWorkspace<Platform>Account`, or OAuth start/callback methods.
- Merge saved credentials into publication config in `ApplySavedCredentialsToPublication`, or let the publisher read `account.Credentials` directly.
- Add routes in `backend/internal/handlers/user-dashboard/accounts.go` and `backend/cmd/api/server.go`.
- Update DTOs, contract, and frontend account API.

Cookie/QR-code login:

- Implement `backend/internal/publisher/browser.RemoteBrowserPlatformAdapter`.
- Define `LoginURL`, `AllowedDomains`, `RequiredCookies`, `DetectLogin`, and `ExtractAccount`.
- Register adapter in `backend/internal/services/browser_session/service.go`.
- If publishing must load stored cookies, add the key to `usesStoredBrowserCookies` in `backend/internal/services/publish/service.go`.
- Test required cookies, domain suffixes, and account-extraction fallback.

## 6. Integrate Remote-Browser Publishing

Only do this when the platform needs server-side browser form filling.

Backend:

- Add or generalize `Start<Platform>PublishSession`; use `backend/internal/services/publish/browser_session.go` as reference.
- Build the `browser-worker` request body from `ProjectPlatformPublication.AdaptedContent`.
- Prepare media with platform media profile.
- Add user route in `backend/internal/handlers/user-dashboard/publications.go` and `backend/cmd/api/server.go`.

Browser Worker:

- Add platform script under `browser-worker/internal/publish/`.
- Register internal route in `browser-worker/internal/server/server.go`.
- Declare request schema in `contracts/paths/browser-worker.yaml` and `contracts/components/browser-worker.yaml`.
- Do not duplicate business validation in worker; worker should only perform DOM actions inside current browser session.

## 7. Integrate Frontend

Platform selection and display should be metadata-driven.

Required:

- Add `PLATFORM_TABS` entry in `frontend/src/lib/content/platforms.ts`.
- Add icon at `frontend/public/icons/platforms/<platform>.svg`.
- Update i18n: `frontend/public/locales/zh/common.json` and `frontend/public/locales/en/common.json`.
- If account connection UI is needed, add or extend `frontend/src/app/[locale]/dashboard/auth/_components/*` and `use-auth-page-controller.ts`.
- If platform cannot auto-publish, check `AUTO_PUBLISH_PLATFORM_TABS` and special branches in `use-content-publish-workflow.ts`.
- If platform needs custom preview, update `platform-preview.tsx` and related tests.

Verify:

```sh
cd frontend
pnpm test
pnpm type-check
pnpm lint
```

## 8. Integrate Extension

Only do this when local browser extension publishing is needed.

Required:

- Extend `PlatformKey` and `AdapterKey` in `extension/src/types/platform.ts`.
- Add `PLATFORM_CAPABILITIES` and `ADAPTER_SCRIPT_FILES` entries in `extension/src/platforms/capabilities.ts`.
- Add platform host permission in `extension/wxt.config.ts`.
- Add `extension/entrypoints/<platform>.content.ts` and call `registerAdapterRunner`.
- Add `extension/src/adapters/<platform>.ts`; keep it limited to target-page DOM fill/click/status reporting.
- Update `extension/src/publish/platform-ui.ts` to decide whether handoff is enabled.
- Update tests for capability, tab injection, and adapter event callback.

Verify:

```sh
cd extension
pnpm test:run
pnpm compile
pnpm lint
```

## 9. Update Config And Deployment

Platform integrations often need new env vars or secrets.

- Put OAuth/API keys in `contracts/env.schema.yaml`, then regenerate example env files.
- If Docker/Kubernetes needs new values, update `deploy/docker/*.example`, `deploy/kubernetes/**/app-config.yaml`, or secret materializer.
- For new external domains used by remote browser, prefer platform adapter `AllowedDomains`; do not broaden global network policy.
- Reuse existing `platform` metrics label; avoid adding high-cardinality labels.
