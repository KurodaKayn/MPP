# Extension Workbench UI Simplification Plan

## Status

Confirmed direction: Option A, a minimal publishing workbench with account and diagnostics moved into settings.

## Goal

Refocus the extension UI from a developer-oriented monitor into a user-facing publishing workbench.

The primary user flow should be:

1. Open the extension.
2. Select a pre-publish draft.
3. Select one or more available platforms.
4. Start the platform draft preparation flow.

The main screen should not require users to understand extension internals, handoff metadata, trusted origins, execution IDs, callback details, or raw event timelines.

## Current UI Problem

The current publish page mixes three different audiences:

- Normal users who only want to publish prepared content.
- Operators who need to see account and API state.
- Developers who need handoff, event, callback, and trust diagnostics.

This makes the first screen noisy. The `MPP Session` card, active execution summary, platform event cards, event timeline, trusted origins, version text, and internal status messages are useful during development, but they are too prominent for the normal publishing workflow.

## Chosen Direction

Use a minimal workbench as the first screen and move secondary information into a settings panel.

Main screen:

- Draft selection.
- Platform selection.
- Start action.
- Short user-facing errors.
- Compact progress or result state only when a publishing handoff is active.

Settings panel:

- Account state.
- Sign out.
- Refresh session.
- Trusted origins.
- Clear local execution state.
- Extension version.
- Diagnostics and event timeline.

## Information Architecture

### Main Header

Show:

- Product title, such as `MPP Publisher`.
- A settings icon button.

Hide from the default header:

- Extension version.
- Extension ID.
- Schema version.
- Trusted origin summary.

### Main Workbench

The main screen should contain one primary card or section:

```text
Pre-Publish Drafts
```

Inside it:

- Draft list.
- Selected draft summary.
- Available platform list.
- Primary start button.

The user should not need to scan separate cards to understand what to do next.

### Draft List

Each draft row should show only:

- Draft title.
- Last updated time.
- Short preview when available.
- Selected state.

Avoid showing:

- Project IDs.
- Publication IDs.
- Adapter keys.
- Internal status unless it blocks publishing.

### Platform Selection

For the selected draft, show publishable platforms as checkboxes or compact selectable rows.

Each platform row should show:

- Platform name.
- Availability state.
- Short disabled reason when not publishable.

Hide by default:

- Adapter key.
- Content kind.
- Requires-review flag.

If the first supported production target is Douyin, the UI should feel natural for a Douyin-first workflow while still allowing more platforms later.

### Primary Action

Use a button label that reflects the current behavior accurately. Since platform adapters prepare drafts and generally stop at user review, avoid overpromising full automatic publishing.

Recommended English labels:

- `Prepare Platform Draft`
- `Open Platform Editor`

Recommended Chinese labels if the UI is localized later:

- `准备平台草稿`
- `打开平台编辑器`

Avoid using `Publish Now` unless the adapter actually submits content without user review.

## Session Handling

### Logged Out

When no MPP login token is available, the main screen should show a compact login prompt:

```text
Sign in to MPP to load pre-publish drafts.
```

Actions:

- Open MPP.
- Retry.

Do not show the full `MPP Session` card on the main screen.

### Logged In

When authenticated, do not show the session card on the main screen.

The account state should move to settings:

- Username.
- Session status.
- Refresh session.
- Sign out.

### Sign Out

The extension should offer a sign-out action from settings.

Initial scope:

- Clear extension-stored auth token values from `browser.storage.local`.
- Refresh the workbench into the logged-out state.

Optional later scope:

- Also open or call the MPP Web logout flow if product requirements expect a full web session logout.

## Settings Panel

Use a sheet, drawer, or dialog opened from the header settings icon.

Sections:

### Account

Show:

- Current username when authenticated.
- Session status.
- `Refresh Session`.
- `Sign Out`.

### Publishing State

Show:

- Current handoff status only if one exists.
- Clear execution state.

### Diagnostics

Keep collapsed by default.

Include:

- Extension version.
- Trusted origins.
- Execution events.
- Callback failure details.
- Adapter key and inject URL details.

Diagnostics are for support and development. They should not compete with the normal publishing workflow.

## Execution Feedback

After the user starts a handoff, show a compact status area on the main screen.

States:

- Preparing platform page.
- Opening platform editor.
- Draft ready for review.
- Failed with a short recovery hint.

Keep the detailed event timeline in settings diagnostics.

Failure messages should be user-facing:

- Good: `Could not find the Douyin editor. Open Douyin Creator Center and try again.`
- Avoid: `adapter.run failed with metadata adapter_key=DYNAMIC_DOUYIN`.

## Component-Level Plan

### Phase 1: Separate Workbench and Diagnostics

Refactor the publish page into clearer sections:

- Workbench shell.
- Draft and platform selector.
- Account settings.
- Diagnostics settings.
- Compact execution status.

Expected files:

- `extension/entrypoints/publish/main.tsx`
- `extension/src/publish/prepublish.tsx`
- `extension/src/publish/session.tsx`
- New settings component if needed.

### Phase 2: Simplify Main Screen

Remove or hide these from the default main screen:

- `MPP Session` card when authenticated.
- Extension version text.
- `Active Execution` card when no handoff is active.
- Full platform event list.
- Full event timeline.
- Trusted origins.

Keep visible:

- Draft list.
- Platform selector.
- Primary action.
- Compact loading/error/empty states.

### Phase 3: Add Settings Panel

Add a settings entry point from the header.

Move these controls into settings:

- Account state.
- Login refresh.
- Sign out.
- Trusted origins.
- Clear execution state.
- Detailed execution events.

### Phase 4: Add Sign Out Support

Add extension-side auth clearing.

The sign-out action should:

1. Remove stored auth token keys from extension local storage.
2. Clear authenticated UI state.
3. Return the workbench to the login prompt.

Do not clear unrelated execution state unless the user explicitly chooses that action.

### Phase 5: Polish Copy and Empty States

Review all visible text.

Main screen copy should be short and task-oriented.

Examples:

- `No pre-publish drafts yet.`
- `Select at least one platform.`
- `Draft ready for review in Douyin.`
- `Sign in to MPP to load drafts.`

Avoid exposing internal terms in the main workflow:

- Handoff.
- Adapter.
- Callback.
- Trusted origin.
- Execution ID.

## Testing Plan

### Unit and Component Tests

Cover:

- Logged-out state shows compact login prompt.
- Authenticated state hides the main session card.
- Draft list renders available drafts.
- Selecting a draft updates platform choices.
- Disabled platforms cannot be selected.
- Start action is disabled without a selected platform.
- Start action calls the existing handoff flow with selected project and platforms.
- Sign out clears extension-stored auth tokens.
- Settings panel shows account and diagnostics.

### Manual Test Flow

1. Run the extension locally.
2. Load the unpacked extension.
3. Open extension workbench while logged out.
4. Confirm the main screen shows only login guidance.
5. Log in to MPP Web.
6. Refresh or reopen the extension.
7. Confirm the main screen shows pre-publish drafts, not a session card.
8. Select a draft.
9. Select Douyin.
10. Start platform draft preparation.
11. Confirm compact progress is visible.
12. Open settings and confirm detailed diagnostics are still available.
13. Sign out from settings.
14. Confirm the workbench returns to the login prompt.

## Non-Goals

- Do not redesign the platform DOM adapters.
- Do not change backend handoff contracts.
- Do not remove the existing MPP Web bridge flow.
- Do not add username/password login inside the extension.
- Do not auto-submit platform posts.
- Do not remove diagnostics entirely; move them out of the default path.

## Open Decisions

### Button Label

Decision: use `Start Publishing` as the primary action label.

### Settings Presentation

Preferred implementation:

- A right-side settings sheet for desktop-like extension pages.

Acceptable fallback:

- A modal dialog if the local component set does not include a sheet yet.

### Localization

The current extension UI is mostly English. This plan keeps English labels for implementation consistency. A later localization pass can map the simplified UI into Chinese copy.
