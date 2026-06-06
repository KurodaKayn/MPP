# Draft Profile Versioning

Draft profiles are the compatibility boundary for compiled platform payloads.
Each profile name has the form `<platform>@v<integer>`, for example `wechat@v1`.

## Policy

- Keep existing profile output compatible once merged.
- Add a new profile version when changing required fields, output format, schema version, or platform-specific semantics in a way that old publishers may not understand.
- Keep old profile implementations available until all persisted drafts and Go callers that may request them are retired or migrated.
- Register every supported profile in `content-pipeline-core/src/drafts/profiles.rs`.
- Add or update representative snapshot fixtures for every supported profile.
- Record every profile behavior change in the changelog below.

## Compatibility Checks

The Rust core tests assert that:

- Blank target profiles resolve to the registered default profile for the platform.
- Every registered profile has a representative snapshot fixture.
- Snapshot profile, schema version, and adapted content format match the registry.
- Representative compiler output remains stable for WeChat, Zhihu, X, and Douyin.

Run:

```sh
cargo test -p content-pipeline-core
```

## Current Profiles

| Profile | Schema | Format | Notes |
| --- | --- | --- | --- |
| `wechat@v1` | `1` | `html` | Preserves source HTML and emits summary text. |
| `zhihu@v1` | `1` | `markdown` | Converts source HTML into Markdown. |
| `x@v1` | `1` | `text` | Joins title/body and applies weighted length truncation. |
| `douyin@v1` | `1` | `text` | Extracts body text with title/source fallback. |

## Changelog

### 2026-06-05

- Added a Rust draft profile registry for `wechat@v1`, `zhihu@v1`, `x@v1`, and `douyin@v1`.
- Added compatibility checks that tie registered profiles to representative snapshot fixtures.
