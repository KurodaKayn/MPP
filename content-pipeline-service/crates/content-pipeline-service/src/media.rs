mod download;
mod object_ref;

use content_pipeline_core::{DEFAULT_MAX_BYTES, MediaConstraints, MediaProcessor};
use content_pipeline_proto::mpp::contentpipeline::v1::{
    ProcessAssetRequest, ProcessAssetResponse, ProcessedAsset as ProtoProcessedAsset,
    media_asset_processor_server::MediaAssetProcessor, media_source,
};
use reqwest::{Client, Url};
use tonic::{Request, Response, Status};

use download::{fetch_media_url, read_limited_body, response_content_type};
use object_ref::{ObjectRefResolverConfig, resolve_object_ref_url, resolver_http_client};

use crate::media_store::ProcessedMediaObjectStore;
use crate::metrics::{ContentPipelineMetrics, MediaSourceKind};

// Keep this module focused on gRPC orchestration. Network security checks and
// object-ref resolution live behind the two private modules above.
#[derive(Debug, Clone)]
pub(crate) struct MediaAssetProcessorService {
    processor: MediaProcessor,
    metrics: ContentPipelineMetrics,
    object_ref_resolver: Option<ObjectRefResolverConfig>,
    object_ref_resolver_client: Client,
    output_store: ProcessedMediaObjectStore,
}

impl MediaAssetProcessorService {
    pub(crate) fn new(
        metrics: ContentPipelineMetrics,
    ) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        Self::new_with_config(
            metrics,
            ObjectRefResolverConfig::from_env(),
            ProcessedMediaObjectStore::from_env()?,
        )
    }

    #[cfg(test)]
    fn new_with_object_ref_resolver(
        metrics: ContentPipelineMetrics,
        object_ref_resolver: Option<ObjectRefResolverConfig>,
    ) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        Self::new_with_config(
            metrics,
            object_ref_resolver,
            ProcessedMediaObjectStore::test_store()?,
        )
    }

    fn new_with_config(
        metrics: ContentPipelineMetrics,
        object_ref_resolver: Option<ObjectRefResolverConfig>,
        output_store: ProcessedMediaObjectStore,
    ) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        Ok(Self {
            processor: MediaProcessor::new(),
            metrics,
            object_ref_resolver,
            object_ref_resolver_client: resolver_http_client()?,
            output_store,
        })
    }

    async fn process_url(
        &self,
        source_url: &str,
        constraints: &MediaConstraints,
    ) -> Result<content_pipeline_core::ProcessedAsset, Status> {
        let url =
            Url::parse(source_url).map_err(|_| Status::invalid_argument("invalid media URL"))?;
        let response = fetch_media_url(url).await?;
        if !response.status().is_success() {
            return Err(Status::unavailable(format!(
                "media download returned HTTP {}",
                response.status().as_u16()
            )));
        }

        let declared_mime_type = response_content_type(&response);
        let bytes = read_limited_body(response, DEFAULT_MAX_BYTES).await?;

        self.processor
            .process_bytes(bytes, declared_mime_type.as_deref(), constraints)
            .map_err(media_error_to_status)
    }

    async fn process_asset_inner(
        &self,
        request: ProcessAssetRequest,
    ) -> Result<MediaProcessOutcome, Status> {
        let constraints = media_constraints_from_request(&request);
        let source = request
            .source
            .ok_or_else(|| Status::invalid_argument("media source is required"))?;

        let (asset, warnings, source_kind, input_bytes, output_bytes) = match source.value {
            Some(media_source::Value::DataUrl(data_url)) => {
                let processed = self
                    .processor
                    .process_data_url(&data_url, &constraints)
                    .map_err(media_error_to_status)?;
                let input_bytes = processed.input_byte_size;
                let output_bytes = processed.byte_size;
                let (asset, warnings) = self.processed_asset_to_proto(processed).await?;
                (
                    asset,
                    warnings,
                    MediaSourceKind::DataUrl,
                    Some(input_bytes),
                    Some(output_bytes),
                )
            }
            Some(media_source::Value::ObjectRef(object_ref)) => {
                // Object refs are trusted internal handles, but the resolved URL still
                // flows through the same download validation as user-provided URLs.
                let resolver = self.object_ref_resolver.as_ref().ok_or_else(|| {
                    Status::failed_precondition("media object ref resolver is not configured")
                })?;
                let resolved_url =
                    resolve_object_ref_url(&self.object_ref_resolver_client, resolver, &object_ref)
                        .await?;
                let processed = self.process_url(&resolved_url, &constraints).await?;
                let input_bytes = processed.input_byte_size;
                let output_bytes = processed.byte_size;
                let (asset, warnings) = self.processed_asset_to_proto(processed).await?;
                (
                    asset,
                    warnings,
                    MediaSourceKind::ObjectRef,
                    Some(input_bytes),
                    Some(output_bytes),
                )
            }
            Some(media_source::Value::Url(url)) => {
                let processed = self.process_url(&url, &constraints).await?;
                let input_bytes = processed.input_byte_size;
                let output_bytes = processed.byte_size;
                let (asset, warnings) = self.processed_asset_to_proto(processed).await?;
                (
                    asset,
                    warnings,
                    MediaSourceKind::Url,
                    Some(input_bytes),
                    Some(output_bytes),
                )
            }
            None => return Err(Status::invalid_argument("media source value is required")),
        };

        Ok(MediaProcessOutcome {
            response: ProcessAssetResponse {
                asset: Some(asset),
                status: "processed".to_string(),
                warnings,
            },
            source_kind,
            input_bytes,
            output_bytes,
        })
    }

    async fn processed_asset_to_proto(
        &self,
        processed: content_pipeline_core::ProcessedAsset,
    ) -> Result<(ProtoProcessedAsset, Vec<String>), Status> {
        let warnings = processed.warnings.clone();
        let stored = self
            .output_store
            .put_processed_asset(&processed)
            .await
            .map_err(|err| {
                Status::unavailable(format!("failed to store processed media object: {err}"))
            })?;

        Ok((
            ProtoProcessedAsset {
                object_ref: stored.object_ref,
                mime_type: processed.mime_type,
                byte_size: processed.byte_size,
                width: processed.width,
                height: processed.height,
                sha256: processed.sha256,
            },
            warnings,
        ))
    }
}

#[tonic::async_trait]
impl MediaAssetProcessor for MediaAssetProcessorService {
    async fn process_asset(
        &self,
        request: Request<ProcessAssetRequest>,
    ) -> Result<Response<ProcessAssetResponse>, Status> {
        let started_at = std::time::Instant::now();
        let request = request.into_inner();
        let platform = request.platform.clone();
        let usage = request.usage.clone();

        match self.process_asset_inner(request).await {
            Ok(outcome) => {
                self.metrics.record_process_asset_success(
                    &platform,
                    &usage,
                    outcome.source_kind,
                    outcome.input_bytes,
                    outcome.output_bytes,
                    started_at.elapsed(),
                );
                Ok(Response::new(outcome.response))
            }
            Err(status) => {
                self.metrics
                    .record_process_asset_error(&platform, &status, started_at.elapsed());
                Err(status)
            }
        }
    }
}

struct MediaProcessOutcome {
    response: ProcessAssetResponse,
    source_kind: MediaSourceKind,
    input_bytes: Option<u64>,
    output_bytes: Option<u64>,
}

fn media_constraints_from_request(request: &ProcessAssetRequest) -> MediaConstraints {
    let max_bytes = request
        .constraints
        .as_ref()
        .and_then(|value| (value.max_bytes > 0).then_some(value.max_bytes));
    let preferred_mime_types = request
        .constraints
        .as_ref()
        .map(|value| value.preferred_mime_types.clone())
        .unwrap_or_default();

    MediaConstraints::for_platform(
        &request.platform,
        &request.usage,
        max_bytes,
        preferred_mime_types,
    )
}

fn media_error_to_status(err: content_pipeline_core::MediaError) -> Status {
    match err {
        content_pipeline_core::MediaError::EmptySource
        | content_pipeline_core::MediaError::InvalidDataUrl
        | content_pipeline_core::MediaError::UnsupportedSource
        | content_pipeline_core::MediaError::UnsupportedFormat
        | content_pipeline_core::MediaError::UnsupportedMimeType { .. }
        | content_pipeline_core::MediaError::DecodeImage
        | content_pipeline_core::MediaError::EncodeImage => {
            Status::invalid_argument(err.to_string())
        }
        content_pipeline_core::MediaError::ResourceLimitExceeded { .. }
        | content_pipeline_core::MediaError::ImageDimensionsExceeded { .. } => {
            Status::resource_exhausted(err.to_string())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn treats_zero_max_bytes_as_unset() {
        let request = ProcessAssetRequest {
            request_id: "request-1".to_string(),
            platform: "generic".to_string(),
            usage: "inline_image".to_string(),
            source: None,
            constraints: Some(
                content_pipeline_proto::mpp::contentpipeline::v1::MediaConstraints {
                    max_bytes: 0,
                    preferred_mime_types: vec!["image/png".to_string()],
                },
            ),
        };

        let constraints = media_constraints_from_request(&request);

        assert_eq!(constraints.max_bytes, Some(DEFAULT_MAX_BYTES));
        assert_eq!(constraints.preferred_mime_types, vec!["image/png"]);
    }

    #[tokio::test]
    async fn rejects_object_ref_without_resolver_configuration() {
        let service = MediaAssetProcessorService::new_with_object_ref_resolver(
            ContentPipelineMetrics::new().expect("metrics should initialize"),
            None,
        )
        .expect("service should initialize");
        let request = ProcessAssetRequest {
            request_id: "request-1".to_string(),
            platform: "wechat".to_string(),
            usage: "cover".to_string(),
            source: Some(
                content_pipeline_proto::mpp::contentpipeline::v1::MediaSource {
                    value: Some(media_source::Value::ObjectRef(
                        "mpp://media/11111111-1111-4111-8111-111111111111".to_string(),
                    )),
                },
            ),
            constraints: None,
        };

        let Err(err) = service.process_asset_inner(request).await else {
            panic!("object_ref should require resolver configuration");
        };

        assert_eq!(err.code(), tonic::Code::FailedPrecondition);
    }

    #[tokio::test]
    async fn returns_object_ref_when_output_store_is_configured() {
        use std::fs;
        use std::sync::Arc;

        use crate::media_store::ProcessedMediaObjectStore;
        use object_store::ObjectStoreExt;
        use object_store::local::LocalFileSystem;
        use object_store::path::Path as ObjectPath;

        const TINY_PNG_DATA_URL: &str = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=";
        const OBJECT_REF_PREFIX: &str = "mpp://content-pipeline/media/";

        let temp_dir = std::env::temp_dir().join(format!(
            "mpp-content-pipeline-service-output-{}",
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&temp_dir);
        fs::create_dir_all(&temp_dir).expect("temp object root should be created");
        let local_store = Arc::new(
            LocalFileSystem::new_with_prefix(&temp_dir)
                .expect("local object store should initialize"),
        );
        let output_store = ProcessedMediaObjectStore::new(
            local_store.clone(),
            "processed".to_string(),
            OBJECT_REF_PREFIX.to_string(),
            7,
        )
        .expect("media output store should initialize");
        let service = MediaAssetProcessorService::new_with_config(
            ContentPipelineMetrics::new().expect("metrics should initialize"),
            None,
            output_store,
        )
        .expect("service should initialize");

        let outcome = service
            .process_asset_inner(ProcessAssetRequest {
                request_id: "request-1".to_string(),
                platform: "generic".to_string(),
                usage: "inline_image".to_string(),
                source: Some(
                    content_pipeline_proto::mpp::contentpipeline::v1::MediaSource {
                        value: Some(media_source::Value::DataUrl(TINY_PNG_DATA_URL.to_string())),
                    },
                ),
                constraints: None,
            })
            .await
            .expect("asset should process");

        let asset = outcome
            .response
            .asset
            .expect("processed asset should be returned");
        let object_ref = asset.object_ref;
        assert!(object_ref.starts_with(OBJECT_REF_PREFIX));
        assert!(object_ref.ends_with(".png"));

        let key = object_ref
            .strip_prefix(OBJECT_REF_PREFIX)
            .expect("object ref should include configured prefix");
        let stored = local_store
            .get(&ObjectPath::from(key))
            .await
            .expect("stored object should exist")
            .bytes()
            .await
            .expect("stored object should be readable");
        assert!(!stored.is_empty());

        let _ = fs::remove_dir_all(temp_dir);
    }
}
