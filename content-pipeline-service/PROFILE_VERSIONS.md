# Content Pipeline Profile Versioning

Draft profiles are the compatibility boundary for compiled platform payloads.
Each profile name has the form `<platform>@v<integer>`, for example `wechat@v1`.
Media profiles are the compatibility boundary for platform media constraints.
They use the same `<platform>@v<integer>` naming rule.

## Draft Profile Policy

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

## Current Draft Profiles

| Profile | Schema | Format | Notes |
| --- | --- | --- | --- |
| `wechat@v1` | `1` | `html` | Preserves source HTML and emits summary text. |
| `zhihu@v1` | `1` | `markdown` | Converts source HTML into Markdown. |
| `x@v1` | `1` | `text` | Joins title/body and applies weighted length truncation. |
| `douyin@v1` | `1` | `text` | Extracts body text with title/source fallback. |

## Media Profile Policy

- Keep existing media constraints compatible once merged.
- Add a new media profile version when changing byte limits, compression behavior, MIME semantics, or platform-specific usage rules.
- Keep old profile implementations available until callers and any persisted media refs that may request them are retired or migrated.
- Register every supported media profile in `content-pipeline-core/src/media/profiles.rs`.
- Empty or unknown platform keys resolve to `generic@v1`.
- Record every media profile behavior change in the changelog below.

## Current Media Profiles

| Profile | Platform | Max Bytes | Compress to Limit | Notes |
| --- | --- | --- | --- | --- |
| `wechat@v1` | WeChat | 2 MiB | Yes | Allows JPEG, PNG, and GIF outputs; WebP is not emitted for WeChat uploads. |
| `douyin@v1` | Douyin | 10 MiB | Yes | Allows JPEG, PNG, and GIF outputs until WebP upload acceptance is verified. |
| `x@v1` | X | 5 MiB | Yes | Allows JPEG, PNG, GIF, and WebP outputs. |
| `zhihu@v1` | Zhihu | 10 MiB | Yes | Allows JPEG, PNG, and GIF outputs until WebP upload acceptance is verified. |
| `generic@v1` | Generic fallback | 10 MiB | Yes | Allows JPEG, PNG, GIF, and WebP outputs for empty or unknown platform keys. |

## Changelog

### 2026-06-12

- Added compiler schema validation for adapted content emitted by Rust draft profiles.
- Added image asset descriptors to WeChat, Zhihu, and Douyin draft outputs when source HTML contains images.

### 2026-06-11

- Added X and Zhihu media profiles and platform-specific output MIME allowlists.
- Enabled constraint-driven image optimization for Douyin, X, Zhihu, and generic media profiles.
- Updated image optimization to try lossless PNG/JPEG metadata reduction, MozJPEG quality search, platform-allowed WebP/AVIF candidates, and gradual Lanczos3 resizing.

### 2026-06-06

- Added a Rust media profile registry for `wechat@v1`, `douyin@v1`, and `generic@v1`.
- Routed media default byte limits and WeChat compression behavior through registered profile metadata.

### 2026-06-05

- Added a Rust draft profile registry for `wechat@v1`, `zhihu@v1`, `x@v1`, and `douyin@v1`.
- Added compatibility checks that tie registered profiles to representative snapshot fixtures.
