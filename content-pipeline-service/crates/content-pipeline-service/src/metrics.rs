use std::net::SocketAddr;
use std::time::Duration;

use axum::Router;
use axum::extract::State;
use axum::http::{StatusCode, header};
use axum::response::{IntoResponse, Response};
use axum::routing::get;
use prometheus::{
    Encoder, HistogramOpts, HistogramVec, IntCounterVec, Opts, Registry, TextEncoder,
};
use tonic::{Code, Status};
use tracing::info;

const ROUTE_PROCESS_ASSET: &str = "ProcessAsset";
const ROUTE_COMPILE_DRAFTS: &str = "CompileDrafts";
const STATUS_OK: &str = "ok";
const STATUS_ERROR: &str = "error";
const ERROR_NONE: &str = "none";

#[derive(Debug, Clone)]
pub(crate) struct ContentPipelineMetrics {
    registry: Registry,
    requests_total: IntCounterVec,
    duration_seconds: HistogramVec,
    media_input_bytes: HistogramVec,
    media_output_bytes: HistogramVec,
    draft_compile_warnings_total: IntCounterVec,
}

impl ContentPipelineMetrics {
    pub(crate) fn new() -> Result<Self, prometheus::Error> {
        let registry = Registry::new_custom(Some("mpp_content_pipeline".to_string()), None)?;

        let requests_total = IntCounterVec::new(
            Opts::new(
                "requests_total",
                "Count content pipeline requests by route, platform, status, and error class.",
            ),
            &["route", "platform", "status", "error_class"],
        )?;
        let duration_seconds = HistogramVec::new(
            HistogramOpts::new(
                "duration_seconds",
                "Track content pipeline request latency in seconds.",
            )
            .buckets(vec![
                0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0,
            ]),
            &["route", "platform", "status", "error_class"],
        )?;
        let media_input_bytes = HistogramVec::new(
            HistogramOpts::new(
                "media_input_bytes",
                "Track processed media input byte size.",
            )
            .buckets(byte_buckets()),
            &["platform", "usage", "source"],
        )?;
        let media_output_bytes = HistogramVec::new(
            HistogramOpts::new(
                "media_output_bytes",
                "Track processed media output byte size.",
            )
            .buckets(byte_buckets()),
            &["platform", "usage", "source"],
        )?;
        let draft_compile_warnings_total = IntCounterVec::new(
            Opts::new(
                "draft_compile_warnings_total",
                "Track lossy or partial draft compiler transformations.",
            ),
            &["platform", "profile"],
        )?;

        registry.register(Box::new(requests_total.clone()))?;
        registry.register(Box::new(duration_seconds.clone()))?;
        registry.register(Box::new(media_input_bytes.clone()))?;
        registry.register(Box::new(media_output_bytes.clone()))?;
        registry.register(Box::new(draft_compile_warnings_total.clone()))?;

        Ok(Self {
            registry,
            requests_total,
            duration_seconds,
            media_input_bytes,
            media_output_bytes,
            draft_compile_warnings_total,
        })
    }

    pub(crate) fn record_process_asset_success(
        &self,
        platform: &str,
        usage: &str,
        source: MediaSourceKind,
        input_bytes: Option<u64>,
        output_bytes: Option<u64>,
        duration: Duration,
    ) {
        self.record_request(
            ROUTE_PROCESS_ASSET,
            platform,
            STATUS_OK,
            ERROR_NONE,
            duration,
        );
        if let Some(input_bytes) = input_bytes {
            self.media_input_bytes
                .with_label_values(&[label_value(platform), label_value(usage), source.as_label()])
                .observe(input_bytes as f64);
        }
        if let Some(output_bytes) = output_bytes {
            self.media_output_bytes
                .with_label_values(&[label_value(platform), label_value(usage), source.as_label()])
                .observe(output_bytes as f64);
        }
    }

    pub(crate) fn record_process_asset_error(
        &self,
        platform: &str,
        status: &Status,
        duration: Duration,
    ) {
        self.record_request(
            ROUTE_PROCESS_ASSET,
            platform,
            STATUS_ERROR,
            status_error_class(status),
            duration,
        );
    }

    pub(crate) fn record_compile_drafts_success(
        &self,
        platform: &str,
        profile: &str,
        warning_count: usize,
        duration: Duration,
    ) {
        self.record_request(
            ROUTE_COMPILE_DRAFTS,
            platform,
            STATUS_OK,
            ERROR_NONE,
            duration,
        );
        if warning_count > 0 {
            self.draft_compile_warnings_total
                .with_label_values(&[label_value(platform), label_value(profile)])
                .inc_by(warning_count as u64);
        }
    }

    pub(crate) fn record_compile_drafts_error(
        &self,
        platform: &str,
        error_class: &'static str,
        duration: Duration,
    ) {
        self.record_request(
            ROUTE_COMPILE_DRAFTS,
            platform,
            STATUS_ERROR,
            error_class,
            duration,
        );
    }

    fn record_request(
        &self,
        route: &'static str,
        platform: &str,
        status: &'static str,
        error_class: &'static str,
        duration: Duration,
    ) {
        self.requests_total
            .with_label_values(&[route, label_value(platform), status, error_class])
            .inc();
        self.duration_seconds
            .with_label_values(&[route, label_value(platform), status, error_class])
            .observe(duration.as_secs_f64());
    }

    fn render(&self) -> Result<String, prometheus::Error> {
        let metric_families = self.registry.gather();
        let mut buffer = Vec::new();
        TextEncoder::new().encode(&metric_families, &mut buffer)?;
        String::from_utf8(buffer).map_err(|err| prometheus::Error::Msg(err.to_string()))
    }
}

#[derive(Debug, Clone, Copy)]
pub(crate) enum MediaSourceKind {
    DataUrl,
    ObjectRef,
    Url,
}

impl MediaSourceKind {
    fn as_label(self) -> &'static str {
        match self {
            Self::DataUrl => "data_url",
            Self::ObjectRef => "object_ref",
            Self::Url => "url",
        }
    }
}

pub(crate) fn draft_error_class(err: &content_pipeline_core::DraftCompileError) -> &'static str {
    match err {
        content_pipeline_core::DraftCompileError::EmptySource
        | content_pipeline_core::DraftCompileError::UnsupportedProfile { .. } => "invalid_input",
        content_pipeline_core::DraftCompileError::UnsupportedSourceFormat(_) => {
            "unsupported_format"
        }
        content_pipeline_core::DraftCompileError::UnsupportedPlatform(_) => "unsupported_format",
        content_pipeline_core::DraftCompileError::Encode(_) => "compile_failed",
    }
}

fn status_error_class(status: &Status) -> &'static str {
    match status.code() {
        Code::ResourceExhausted => "resource_limit_exceeded",
        Code::DeadlineExceeded | Code::Unavailable => "transient_failure",
        Code::InvalidArgument => invalid_argument_error_class(status.message()),
        Code::Internal => "compile_failed",
        _ => "invalid_input",
    }
}

fn invalid_argument_error_class(message: &str) -> &'static str {
    if message.contains("unsafe media")
        || message.contains("private address")
        || message.contains("redirect limit")
        || message.contains("redirect missing")
    {
        return "unsafe_source";
    }
    if message.contains("unsupported") || message.contains("decode image") {
        return "unsupported_format";
    }

    "invalid_input"
}

fn label_value(value: &str) -> &str {
    let value = value.trim();
    if value.is_empty() { "unknown" } else { value }
}

fn byte_buckets() -> Vec<f64> {
    vec![
        512.0,
        1_024.0,
        4_096.0,
        16_384.0,
        65_536.0,
        262_144.0,
        1_048_576.0,
        2_097_152.0,
        5_242_880.0,
        10_485_760.0,
    ]
}

pub(crate) async fn serve(
    metrics: ContentPipelineMetrics,
    addr: SocketAddr,
) -> std::io::Result<()> {
    let listener = tokio::net::TcpListener::bind(addr).await?;
    info!(%addr, "starting content-pipeline-service metrics endpoint");

    axum::serve(
        listener,
        Router::new()
            .route("/metrics", get(metrics_handler))
            .with_state(metrics),
    )
    .await
}

async fn metrics_handler(State(metrics): State<ContentPipelineMetrics>) -> Response {
    match metrics.render() {
        Ok(body) => (
            StatusCode::OK,
            [(header::CONTENT_TYPE, TextEncoder::new().format_type())],
            body,
        )
            .into_response(),
        Err(err) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            [(header::CONTENT_TYPE, "text/plain; charset=utf-8")],
            err.to_string(),
        )
            .into_response(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renders_recorded_process_and_draft_metrics() {
        let metrics = ContentPipelineMetrics::new().expect("metrics should initialize");

        metrics.record_process_asset_success(
            "wechat",
            "cover",
            MediaSourceKind::DataUrl,
            Some(2_048),
            Some(1_024),
            Duration::from_millis(12),
        );
        metrics.record_compile_drafts_success("x", "x@v1", 2, Duration::from_millis(8));

        let rendered = metrics.render().expect("metrics should render");

        assert!(rendered.contains("mpp_content_pipeline_requests_total"));
        assert!(rendered.contains(r#"route="ProcessAsset""#));
        assert!(rendered.contains(r#"platform="wechat""#));
        assert!(rendered.contains(r#"status="ok""#));
        assert!(rendered.contains("mpp_content_pipeline_media_input_bytes_bucket"));
        assert!(rendered.contains("mpp_content_pipeline_media_output_bytes_bucket"));
        assert!(rendered.contains(r#"source="data_url""#));
        assert!(rendered.contains("mpp_content_pipeline_draft_compile_warnings_total"));
        assert!(rendered.contains(r#"platform="x""#));
        assert!(rendered.contains(r#"profile="x@v1""#));
    }

    #[test]
    fn records_unknown_labels_for_empty_values() {
        let metrics = ContentPipelineMetrics::new().expect("metrics should initialize");

        metrics.record_process_asset_success(
            "",
            " ",
            MediaSourceKind::Url,
            Some(128),
            Some(128),
            Duration::from_millis(1),
        );

        let rendered = metrics.render().expect("metrics should render");

        assert!(rendered.contains(r#"platform="unknown""#));
        assert!(rendered.contains(r#"usage="unknown""#));
    }

    #[test]
    fn classifies_status_errors_for_metrics() {
        assert_eq!(
            status_error_class(&Status::resource_exhausted("media exceeds max bytes")),
            "resource_limit_exceeded"
        );
        assert_eq!(
            status_error_class(&Status::deadline_exceeded("media download timed out")),
            "transient_failure"
        );
        assert_eq!(
            status_error_class(&Status::invalid_argument("unsafe media URL")),
            "unsafe_source"
        );
        assert_eq!(
            status_error_class(&Status::invalid_argument("unsupported image format")),
            "unsupported_format"
        );
        assert_eq!(
            status_error_class(&Status::invalid_argument("media source is required")),
            "invalid_input"
        );
    }

    #[test]
    fn classifies_draft_errors_for_metrics() {
        assert_eq!(
            draft_error_class(&content_pipeline_core::DraftCompileError::EmptySource),
            "invalid_input"
        );
        assert_eq!(
            draft_error_class(
                &content_pipeline_core::DraftCompileError::UnsupportedSourceFormat(
                    "markdown".to_string(),
                ),
            ),
            "unsupported_format"
        );
        assert_eq!(
            draft_error_class(
                &content_pipeline_core::DraftCompileError::UnsupportedPlatform(
                    "mastodon".to_string(),
                )
            ),
            "unsupported_format"
        );
        assert_eq!(
            draft_error_class(
                &content_pipeline_core::DraftCompileError::UnsupportedProfile {
                    platform: "x".to_string(),
                    profile: "x@v2".to_string(),
                },
            ),
            "invalid_input"
        );
    }
}
