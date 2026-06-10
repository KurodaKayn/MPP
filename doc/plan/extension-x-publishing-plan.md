# Extension X Publishing Plan

## 1. Decision

MPP will add X publishing to the browser extension as a text-only draft preparation flow. The extension will fill the X compose draft and leave final submission to the user. It will not click the platform publish button in the first version.

When a user selects both Douyin and X in the extension workbench, the backend will return one extension handoff containing both platform tasks. The extension will execute those platform tasks through a local serialized queue. This keeps DOM automation predictable and avoids multiple platform tabs competing for focus, login prompts, upload dialogs, or extension state.

The server-side Redis publish queue remains responsible for backend-controlled publishing flows. Extension publishing depends on the user's local browser session, so the browser extension is the executor for this path.

## 2. Scope

### In Scope

- Enable X as an implemented extension platform.
- Generate backend extension handoffs for X.
- Inject text into the X composer.
- Keep X in user-review mode after draft preparation.
- Allow Douyin and X to be selected together.
- Execute selected extension platforms sequentially in the local extension runtime.
- Record per-platform extension events through the existing callback mechanism.

### Out of Scope

- Auto-clicking X publish/post buttons.
- X media upload support.
- X API publishing through OAuth credentials.
- A Redis-backed extension job polling system.
- Fully parallel tab injection.

## 3. Current State

The extension workbench already supports selecting platform cards, but only Douyin is wired as a real handoff platform. X is present as UI-only.

Current backend extension handoff generation only accepts Douyin. The extension handoff service should be generalized from hardcoded Douyin constants into a platform configuration map.

The extension background already opens and injects platforms sequentially in `startPublishingTabs`, but the execution model is implicit. For Douyin plus X, this should become an explicit local platform queue with clear per-platform status.

## 4. Target Flow

1. The user opens the extension workbench.
2. The user selects a pre-publish draft.
3. The user selects Douyin, X, or both.
4. The extension calls the backend extension handoff endpoint with the selected platform list.
5. The backend validates the project, publications, and synced adapted content.
6. The backend returns one `mpp.extension_publish_handoff` with one platform payload per selected platform.
7. The extension stores the execution locally.
8. The extension runner processes platform tasks one at a time.
9. For each platform, the runner opens the platform page, waits for the tab to load, injects the adapter, and records adapter events.
10. The adapter fills the platform draft and emits `user_review`.
11. The user reviews and manually submits on the platform.

## 5. Execution Model

The extension should use local serialized execution:

- One active platform task at a time.
- A platform task starts only after the previous task reaches a terminal preparation state such as `user_review`, `failed`, `cancelled`, or `expired`.
- A new handoff should not overwrite an active execution without an explicit clear/cancel action.
- Events remain per platform and continue to use callback tokens from the backend handoff.

This model can be extended later with limited parallelism, but the first X version should keep concurrency at `1`.

## 6. Backend Changes

### Phase 1: Platform Handoff Configuration

Goal: make backend extension handoff generation support more than Douyin.

Tasks:

- Replace Douyin-only constants in the extension service with a platform config map.
- Add an X config with:
  - `platform`: `x`
  - `adapter_key`: a new extension adapter key, for example `POST_X`
  - `inject_url`: the X compose page URL used by the adapter
  - `content_kind`: `dynamic_post`
  - `target format`: `text`
  - `requires_review`: `true`
  - `auto_publish`: `false`
- Update extension prepublish listing to include both Douyin and X publications.
- Update platform normalization to accept X.
- Preserve callback token creation per platform.
- Add tests for Douyin-only, X-only, and Douyin-plus-X handoffs.

Acceptance criteria:

- X pre-publish drafts appear in the extension workbench when the project has an enabled X publication.
- Creating a handoff with `["x"]` returns one X platform payload.
- Creating a handoff with `["douyin", "x"]` returns one execution with two platform payloads.
- Invalid or unsupported platforms are still rejected.

### Phase 2: Publication Status Integration

Goal: keep extension event callbacks useful for dashboard status.

Tasks:

- Confirm how extension callback events should map to `ProjectPlatformPublication` status.
- Keep `user_review` as the expected successful preparation state for X.
- Ensure failed X adapter events are visible in publication details or extension monitor.
- Keep event callback idempotency through `event_id`.

Acceptance criteria:

- X adapter failures are recorded with a user-facing error message.
- X draft preparation success records a `user_review` event.
- Duplicate callback events do not create duplicate rows.

## 7. Extension Changes

### Phase 3: Platform Registry and UI Enablement

Goal: make X selectable as a real extension platform.

Tasks:

- Add `x` to extension platform types.
- Add a new X adapter key.
- Add X capability metadata and injectable URL matching.
- Change the X card from `ui_only` to `implemented`.
- Keep unsupported platforms such as WeChat or Zhihu UI-only unless their adapters are ready.
- Update prepublish workbench tests for X selection and multi-select handoff creation.

Acceptance criteria:

- X can be selected when the backend marks the X publication enabled.
- X is included in the handoff platform list sent to the backend.
- UI-only platforms are not sent to the backend.

### Phase 4: X Text Draft Adapter

Goal: prepare an X post draft without publishing it.

Tasks:

- Add an X content script entrypoint.
- Add an X adapter module.
- Detect signed-out or unavailable composer states.
- Find the compose textbox with resilient selectors.
- Fill the text draft using the shared text injection helpers where possible.
- Emit `user_review` after the draft text is prepared.
- Do not click the X post button.
- Add unit tests around text extraction, missing composer handling, and adapter result metadata.

Acceptance criteria:

- The adapter fills the X composer with the prepublish text.
- The adapter does not publish automatically.
- Missing login or missing composer returns a clear failure.
- The final event tells the user to review and publish manually.

### Phase 5: Local Serialized Platform Queue

Goal: make Douyin-plus-X execution explicit and recoverable enough for extension UX.

Tasks:

- Introduce a local execution runner state in extension storage.
- Represent each platform as a task with status:
  - `queued`
  - `opening_tabs`
  - `injecting`
  - `user_review`
  - `failed`
  - `expired`
  - `cancelled`
- Prevent a new handoff from silently replacing an active execution.
- Start the next platform task only after the current platform reaches a terminal preparation state.
- Keep monitor UI compatible with existing event history.
- Add tests for sequential Douyin-plus-X execution.

Acceptance criteria:

- Selecting Douyin and X opens/injects one platform at a time.
- A failed first platform does not block the next platform unless the failure is caused by an invalid handoff.
- The monitor shows per-platform progress clearly.
- Clearing the monitor clears the active local queue state.

## 8. Testing Plan

Backend tests:

- Extension prepublish listing includes X publications.
- X-only handoff generation.
- Douyin-plus-X handoff generation.
- Unsupported platform rejection.
- Callback event recording for X.

Extension tests:

- Platform UI maps X to a handoff platform.
- Handoff request includes X only when selected and enabled.
- X adapter returns failure when not on X or when signed out.
- X adapter fills draft text and returns `user_review`.
- Local runner processes multiple platform tasks sequentially.

Manual tests:

- Login to MPP and X in the same browser profile.
- Create a project with X enabled.
- Sync prepublish drafts.
- Open extension workbench.
- Select the X draft and start publishing.
- Confirm X compose opens with text filled and no auto-submit.
- Repeat with Douyin and X selected together.

## 9. Rollout Order

1. Backend X handoff support.
2. Extension platform registry and UI enablement.
3. X text draft adapter.
4. Local serialized execution queue.
5. End-to-end manual verification.

Each phase should be committed separately to keep review small and to isolate backend, extension UI, adapter, and execution-model changes.

## 10. Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| X DOM changes frequently. | Keep selectors narrow but layered, and return clear adapter failures. |
| User is not logged in to X. | Detect signed-out state and stop with a reviewable error. |
| Extension service worker sleeps. | Store execution state and make task transitions idempotent. |
| Multiple selected platforms confuse the user. | Execute sequentially and show per-platform status. |
| Auto-publish causes accidental posting. | Keep `auto_publish` false and never click the X post button in this phase. |

## 11. Recommended First Implementation Slice

Start with backend Phase 1 and extension Phase 3 together. That creates a complete handoff contract for X before building the DOM adapter. After that, implement the X text adapter, then harden multi-platform execution with the local serialized queue.
