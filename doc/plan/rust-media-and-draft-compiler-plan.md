# Rust Media Asset and Platform Draft Compiler Plan

## 1. Decision

Introduce a Rust-backed content processing layer for two business capabilities:

- Media asset processing and platform asset adaptation.
- Platform draft compilation.

This plan does not propose rewriting the Go backend. The Go backend should continue to own authentication, user scope, project state, publication state, account credentials, queue orchestration, and database transactions. Rust should be introduced only where it provides a stronger fit than Go for the business workload: deterministic content transformation, binary media processing, strict schema validation, and bounded resource execution.

The first Rust service should be a small internal gRPC service named `content-pipeline-service`.

```mermaid
flowchart LR
  Frontend["Frontend Workspace"] --> Backend["Go Backend"]
  Backend --> Pg[("PostgreSQL")]
  Backend --> Redis[("Redis")]
  Backend --> Rust["Rust content-pipeline-service"]
  Rust --> ObjectStore[("Object Storage<br/>future")]
  Backend --> Publisher["Publish Worker / Platform Publishers"]
```

## 2. Why These Capabilities Fit Rust

Most backend modules in MPP are orchestration-heavy and remain a good fit for Go. These two modules are different.

Media processing deals with untrusted external bytes, large memory buffers, image decoding, resizing, compression, MIME detection, and platform-specific asset constraints. Rust gives this workload precise memory ownership, predictable resource cleanup, strong error types, and a mature ecosystem for safe binary processing.

Draft compilation deals with deterministic transformations from source content into platform-specific payloads. It benefits from strong enums, explicit error variants, strict schema versions, exhaustive matching, and highly testable pure transformation pipelines. Rust can make platform rules feel closer to a compiler than scattered helper functions.

## 3. Service Name and Framework Stack

### 3.1 Service Name

Use `content-pipeline-service` as the production service name.

Recommended internal crate layout:

| Crate | Purpose |
| --- | --- |
| `content-pipeline-service` | Binary crate. Owns gRPC server startup, config, observability, and dependency wiring. |
| `content-pipeline-core` | Library crate. Owns pure media processing, draft compilation, platform profiles, and tests. |
| `content-pipeline-proto` | Generated protobuf bindings, if the workspace prefers keeping generated code separate. |

The service name should appear consistently in Compose, logs, metrics labels, gRPC service config, and tracing spans.

### 3.2 Framework Stack

| Layer | Choice | Reason |
| --- | --- | --- |
| gRPC framework | `tonic` | Primary service framework for gRPC server and Go backend integration. |
| Protobuf runtime | `prost`, `prost-types` | Standard protobuf code generation used by `tonic`. |
| Proto build | `tonic-build` and optionally `buf` | Generates Rust bindings and keeps protobuf definitions lintable/versioned. |
| Async runtime | `tokio` | Runtime used by `tonic`, async filesystem work, bounded tasks, and HTTP fetches. |
| Middleware | `tower` layers | Timeouts, concurrency limits, request IDs, and tracing around gRPC calls. |
| HTTP client | `reqwest` with `rustls` | Download external media sources with timeouts, redirect limits, and TLS. |
| Errors | `thiserror` | Stable typed errors mapped into gRPC status codes. |
| Serialization | `serde`, `serde_json` | Platform config and adapted-content payloads. |
| Observability | `tracing`, `tracing-subscriber`, Prometheus exporter | Structured logs and metrics for processing latency and failure classes. |
| Media | `image`, `fast_image_resize`, `infer`, `mime` | Safe image decoding, resizing, MIME sniffing, and platform media constraints. |
| HTML/draft parsing | `html5ever`, `scraper` | Deterministic parsing and extraction for platform draft compilation. |

Business APIs should be gRPC-first. HTTP should only be used for operational endpoints such as Prometheus metrics if the deployment stack requires HTTP scraping.

## 4. Scope

| Capability | Included | Notes |
| --- | --- | --- |
| Image download | Yes | Fetch remote images with size, timeout, MIME, and redirect limits. |
| Data URL decoding | Yes | Decode inline images safely with maximum payload limits. |
| Image validation | Yes | Detect MIME, dimensions, byte size, and unsupported formats. |
| Image resizing/compression | Yes | Generate platform-compliant images, especially for WeChat covers and inline assets. |
| Platform asset adaptation | Yes | Convert source assets into platform-ready asset descriptors. |
| Draft compilation | Yes | Compile source project content into platform draft payloads. |
| Draft schema validation | Yes | Validate input and output against versioned platform draft schemas. |
| Object storage upload | Partial | Rust can optionally write processed media to object storage and return internal object refs; inline bytes remain the default until callers consume the refs. |
| Platform API publishing | No | Publishing execution remains in Go for this phase. |
| Browser automation | No | Browser-based publishing stays in `browser-worker`. |
| User permissions | No | Go backend remains the permission boundary. |
| Database ownership | No | Rust should not directly mutate publication state in this phase. |

## 5. Current Code Touchpoints

The current Go backend already contains the natural seams for these modules:

- `backend/internal/pkg/media` delegates media processing to `content-pipeline-service`.
- `backend/internal/pkg/html/processor.go` scans HTML and replaces image sources.
- `backend/internal/publisher/platforms/wechat/wechat.go` processes inline images and cover images before creating a WeChat draft.
- `backend/internal/publisher/platforms/x/x.go` contains X text extraction, URL weighting, and truncation rules.
- `backend/internal/publisher/platforms/zhihu/zhihu.go` compiles source HTML into Markdown.
- `backend/internal/publisher/platforms/douyin/douyin.go` extracts plain text and prepares image-based publishing input.

These are business-specific transformations. They are not merely infrastructure utilities.

## 6. Module 1: Media Asset Processing and Platform Asset Adaptation

### 6.1 Responsibility

This module turns source media references into platform-ready assets.

Inputs:

- Remote image URL.
- Data URL.
- Existing MPP asset reference.
- Optional platform key.
- Optional usage kind, such as `cover`, `inline_image`, `thumbnail`, or `gallery_image`.

Outputs:

- Normalized asset metadata.
- Processed bytes or temporary object reference.
- MIME type.
- Width and height.
- Byte size.
- Hash.
- Platform compliance status.
- Structured warnings and errors.

### 6.2 Platform Rules

The first platform rules should be deliberately small:

| Platform | Usage | Rule |
| --- | --- | --- |
| WeChat | cover | Must produce an uploadable image under the configured size limit. |
| WeChat | inline image | Must produce uploadable bytes and preserve HTML replacement metadata. |
| Douyin | cover/image post | Must produce a local or object-backed image reference suitable for upload automation. |
| Generic | all | Reject unsupported MIME types, oversized inputs, and unsafe URLs. |

Rules should live in Rust as versioned platform profiles:

```text
wechat@v1
douyin@v1
generic@v1
```

### 6.3 Proposed Interface

Start with an internal gRPC API. The Go backend should generate a typed gRPC client from the same protobuf definition and call the Rust service from the existing media adapter.

```proto
syntax = "proto3";

package mpp.contentpipeline.v1;

service MediaAssetProcessor {
  rpc ProcessAsset(ProcessAssetRequest) returns (ProcessAssetResponse);
}

message ProcessAssetRequest {
  string request_id = 1;
  string platform = 2;
  string usage = 3;
  MediaSource source = 4;
  MediaConstraints constraints = 5;
}

message MediaSource {
  oneof value {
    string url = 1;
    string data_url = 2;
    string object_ref = 3;
  }
}

message MediaConstraints {
  uint64 max_bytes = 1;
  repeated string preferred_mime_types = 2;
}

message ProcessAssetResponse {
  ProcessedAsset asset = 1;
  string status = 2;
  repeated string warnings = 3;
}

message ProcessedAsset {
  oneof content {
    bytes inline_bytes = 1;
    string object_ref = 2;
  }
  string mime_type = 3;
  uint64 byte_size = 4;
  uint32 width = 5;
  uint32 height = 6;
  string sha256 = 7;
}
```

For the first version, returning `inline_bytes` is acceptable for small assets. Once object storage is introduced, the service should prefer returning `object_ref` instead.

### 6.4 Why This Should Not Stay in Go Long Term

Go is good enough for the current implementation, but media processing has different pressure points:

- Large byte buffers can increase Go heap pressure.
- Image decoding and resizing benefit from tight memory control.
- Asset processing should be isolated from request handlers and publishing orchestration.
- Platform asset rules will grow independently from publication state rules.
- The module can be tested with golden files and property-style constraints.

Rust gives better leverage here without forcing distributed business transactions.

## 7. Module 2: Platform Draft Compiler

### 7.1 Responsibility

This module turns a canonical MPP source project into platform-specific draft payloads.

Inputs:

- Project title.
- Source content.
- Source content format.
- Platform key.
- Optional platform config.
- Optional asset references.
- Compiler profile version.

Outputs:

- Versioned platform draft payload.
- Human-readable summary.
- Extracted text.
- Platform-specific warnings.
- Validation errors.
- Asset processing requests, if assets must be normalized before publishing.

### 7.2 Draft Compilation Targets

Initial platform targets:

| Platform | Draft Format | Example Rules |
| --- | --- | --- |
| WeChat | HTML | Preserve rich text, normalize images, prepare cover and inline assets. |
| Zhihu | Markdown | Convert source HTML to Markdown, preserve headings and links. |
| X | Text | Extract text, apply weighted length rules, truncate safely. |
| Douyin | Text + image assets | Extract concise text and prepare image publishing inputs. |

### 7.3 Proposed Interface

```proto
syntax = "proto3";

package mpp.contentpipeline.v1;

service PlatformDraftCompiler {
  rpc CompileDrafts(CompileDraftsRequest) returns (CompileDraftsResponse);
}

message CompileDraftsRequest {
  string request_id = 1;
  SourceProject project = 2;
  repeated DraftTarget targets = 3;
}

message SourceProject {
  string id = 1;
  string title = 2;
  string source_format = 3;
  string source_content = 4;
}

message DraftTarget {
  string platform = 1;
  string profile = 2;
  string config_json = 3;
}

message CompileDraftsResponse {
  repeated CompiledDraft drafts = 1;
}

message CompiledDraft {
  string platform = 1;
  string profile = 2;
  string status = 3;
  string adapted_content_json = 4;
  string summary = 5;
  repeated string warnings = 6;
}
```

`adapted_content_json` intentionally stays JSON in the first version because existing MPP platform draft schemas are already JSON-shaped. The protobuf contract provides a typed transport boundary while allowing platform-specific payloads to evolve behind versioned profiles.

### 7.4 Compiler Design

The compiler should be organized around explicit stages:

1. Parse source content into an internal document model.
2. Normalize whitespace, links, headings, images, and unsupported nodes.
3. Apply platform profile rules.
4. Emit versioned adapted content.
5. Validate the emitted payload against the target schema.
6. Return warnings for lossy transformations.

The internal document model does not need to be a full editor schema. It only needs enough structure to produce platform drafts consistently.

### 7.5 Why This Should Not Stay in Go Long Term

The current platform adapters mix draft adaptation and publishing behavior. As platforms grow, that will make publication code harder to reason about. A Rust compiler module can keep platform transformation rules:

- Pure.
- Deterministic.
- Exhaustively tested.
- Versioned by platform profile.
- Independent from publication execution.

This is a better fit for Rust than Go because the core work is not HTTP orchestration. It is schema-driven transformation.

## 8. Go Backend Responsibilities After Extraction

The Go backend remains the business authority.

It should still own:

- User authentication.
- Project ownership checks.
- Publication record creation and updates.
- Platform account credentials.
- Queue enqueueing.
- Publish locks.
- Publication status transitions.
- Persisting `adapted_content` into PostgreSQL.
- Calling platform publishers or publish workers.

The Rust service should not decide whether a user may publish a project. It should only process content it receives from the trusted backend.

## 9. Data Ownership

The first Rust implementation should be stateless.

| Data | Owner |
| --- | --- |
| Users | Go backend / PostgreSQL |
| Projects | Go backend / PostgreSQL |
| Publications | Go backend / PostgreSQL |
| Platform accounts | Go backend / PostgreSQL |
| Draft compiler profiles | Rust service code |
| Media processing profiles | Rust service code |
| Temporary processed media | Rust service memory or temp storage |
| Durable assets | Future object storage |

Avoid giving the Rust service direct database write access in the first phase. This keeps the migration reversible.

## 10. Migration Plan

### Phase 1: Extract Media Processing Behind an Adapter

Goal: replace the current in-process media processing path with an internal service call.

Deliverables:

- Rust `MediaAssetProcessor.ProcessAsset` gRPC endpoint.
- Go client wrapper in the existing media package.
- Golden tests comparing output constraints, not exact bytes.
- Metrics for input size, output size, duration, failures, and platform usage.

Acceptance:

- WeChat cover images remain under the required size limit.
- Inline image replacement still works for WeChat drafts.
- Unsafe or oversized inputs fail with structured errors.
- Go does not keep an in-process media implementation.

### Phase 2: Extract Platform Draft Compilation

Goal: move platform-specific draft adaptation out of publisher implementations.

Deliverables:

- Rust `PlatformDraftCompiler.CompileDrafts` gRPC endpoint.
- Go compiler client used by prepublish sync.
- Platform profiles for WeChat, Zhihu, X, and Douyin.
- Contract tests for each platform profile.
- Snapshot tests for representative source documents.

Acceptance:

- Existing platform draft responses preserve the same public API shape.
- X text still respects weighted length behavior.
- Zhihu Markdown output is stable across repeated compilation.
- WeChat HTML output preserves supported rich content.
- Unsupported source structures return warnings instead of silent corruption.

### Phase 3: Add Object Storage Integration

Goal: prevent large media bytes from moving repeatedly through the Go backend.

Deliverables:

- Object storage abstraction in Rust. Implemented in `content-pipeline-service` behind `CONTENT_PIPELINE_MEDIA_OBJECT_STORE`.
- Signed or internal object references returned to Go. Implemented as `mpp://content-pipeline/media/...` refs when Rust output storage is configured.
- Asset hash and deduplication support. Implemented with deterministic sha256 object keys and same-size existing object reuse.
- Expiration policy for temporary objects. Implemented as retention tags/metadata; bucket lifecycle policy should expire the configured processed-media prefix.

The Rust service owns this storage integration directly. Do not add a Go-side processed-media upload proxy for this phase; that would create an extra migration surface to remove later.

Rust output storage is disabled unless `CONTENT_PIPELINE_MEDIA_OBJECT_STORE` is set. Supported values are `filesystem`, `r2`, and `s3`. The key configuration variables are:

| Variable | Purpose |
| --- | --- |
| `CONTENT_PIPELINE_MEDIA_OBJECT_STORE` | Enables processed-media object output and selects `filesystem`, `r2`, or `s3`. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_ROOT` | Filesystem root for local object-store mode. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_BUCKET`, `CONTENT_PIPELINE_MEDIA_OBJECT_ENDPOINT`, `CONTENT_PIPELINE_MEDIA_OBJECT_REGION` | S3/R2 bucket, endpoint, and region overrides. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_ACCESS_KEY_ID`, `CONTENT_PIPELINE_MEDIA_OBJECT_SECRET_ACCESS_KEY` | S3/R2 credentials; R2 mode can also use existing `R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY`. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_PREFIX` | Object key prefix; defaults to `content-pipeline/processed-media`. Attach lifecycle expiration to this prefix. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX` | Internal ref prefix; defaults to `mpp://content-pipeline/media/`. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_MIN_BYTES` | Minimum processed byte size for object-ref output; defaults to `0` once output storage is enabled. |
| `CONTENT_PIPELINE_MEDIA_OBJECT_RETENTION_DAYS` | Retention metadata/tag value; defaults to `7`. |

Acceptance:

- Large processed images no longer need to be stored in publication JSON.
- Publishing code can fetch or pass object references without exposing private URLs to the frontend.

### Phase 4: Expand Platform Profiles

Goal: make platform changes safer and easier to review.

Deliverables:

- Profile versioning policy.
- Compatibility tests for old adapted content.
- Per-platform fixtures.
- Changelog for platform profile changes.

Acceptance:

- Adding a new platform does not require editing existing platform compilers except shared utilities.
- Profile changes are visible in tests and release notes.

## 11. Failure Model

The Rust service must return structured errors.

Recommended error classes:

| Error | Meaning |
| --- | --- |
| `invalid_input` | The backend sent malformed content or config. |
| `unsupported_format` | The source or output format is not supported. |
| `unsafe_source` | URL, redirect, MIME, or payload failed safety checks. |
| `resource_limit_exceeded` | Size, dimensions, timeout, or memory guard rejected the request. |
| `compile_failed` | Draft compilation failed for a platform-specific reason. |
| `transient_failure` | External fetch or temporary infrastructure failure. |

The Go backend should translate these into existing user-facing errors and persist enough context for debugging.

## 12. Observability

The Rust service should expose:

- gRPC health checks through the standard gRPC health checking protocol.
- Optional gRPC reflection in local and staging environments.
- `/metrics` over HTTP if Prometheus scraping requires an HTTP endpoint.

Metrics:

| Metric | Purpose |
| --- | --- |
| `mpp_content_pipeline_requests_total` | Count requests by route, platform, status, and error class. |
| `mpp_content_pipeline_duration_seconds` | Track processing and compilation latency. |
| `mpp_media_input_bytes` | Track media input size. |
| `mpp_media_output_bytes` | Track processed media size. |
| `mpp_draft_compile_warnings_total` | Track lossy or partial platform transformations. |

Logs should include request ID, platform, profile, usage, duration, and error class. Do not log raw content, raw image bytes, credentials, cookies, or signed URLs.

## 13. Security Requirements

Media processing must treat all external sources as untrusted.

Required controls:

- Maximum response size.
- Maximum decoded image size.
- Download timeout.
- Redirect limit.
- Allowed schemes: `https` and controlled `data` URLs only.
- Private network and localhost blocking for remote URLs.
- MIME sniffing, not only trusting response headers.
- Temporary file cleanup.
- No raw source URL logging when it may contain secrets.

Draft compilation must treat source content as untrusted HTML.

Required controls:

- Avoid script execution.
- Normalize or drop unsupported nodes.
- Preserve only supported attributes.
- Return warnings for lossy transformations.
- Keep platform compiler output schema-validated.

## 14. Non-goals

This plan does not include:

- Rewriting Go backend APIs in Rust.
- Moving publication state transitions to Rust.
- Moving queue workers to Rust.
- Adding REST business endpoints for content processing.
- Rewriting `browser-worker`.
- Rewriting `collab-service`.
- Replacing the AI service.
- Introducing Kafka, Kubernetes, or service mesh.
- Giving Rust direct ownership of user credentials or platform cookies.

## 15. Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| The service boundary adds latency. | Batch draft compilation by project and targets; keep media processing async where possible. |
| Byte payloads become expensive over gRPC. | Use object references after Phase 3; keep inline bytes only for small initial cases. |
| Draft output changes unexpectedly. | Use fixtures, golden tests, and profile versioning. |
| Platform rules duplicate Go logic during migration. | Keep platform media and draft rules in Rust profiles and leave Go as the orchestration boundary. |
| Rust service failure blocks prepublish sync. | Treat `content-pipeline-service` as a required dependency and surface structured service errors. |

## 16. Success Criteria

The extraction is successful when:

- Media processing can be scaled independently from Go API replicas.
- Platform draft compilation is deterministic and covered by platform fixtures.
- Go publisher implementations no longer contain most draft transformation logic.
- WeChat and X rules are easier to change without touching queue or database code.
- Large media processing does not increase Go backend memory pressure.
- The Rust service can be disabled without corrupting publication state.

## 17. Recommended First Milestone

Start with media processing, not draft compilation.

Reason:

- It has the clearest Rust advantage over Go.
- It has a small input/output contract.
- It is easy to feature-flag.
- It does not require changing publication state semantics.
- It creates immediate isolation for the most resource-sensitive content path.

After that, move X and WeChat draft compilation into Rust because they are the easiest profiles to validate with deterministic fixtures.
