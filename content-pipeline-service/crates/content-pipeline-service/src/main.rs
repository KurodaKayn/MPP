use std::net::SocketAddr;

use content_pipeline_core::{DraftCompiler, DraftTarget, MediaProcessor, SourceProject};
use content_pipeline_proto::mpp::contentpipeline::v1::platform_draft_compiler_server::{
    PlatformDraftCompiler, PlatformDraftCompilerServer,
};
use content_pipeline_proto::mpp::contentpipeline::v1::{
    CompileDraftsRequest, CompileDraftsResponse, CompiledDraft, ProcessAssetRequest,
    ProcessAssetResponse, ProcessedAsset,
    media_asset_processor_server::{MediaAssetProcessor, MediaAssetProcessorServer},
    media_source, processed_asset,
};
use tonic::{Request, Response, Status, transport::Server};
use tracing::info;
use tracing_subscriber::EnvFilter;

#[derive(Debug, Default)]
struct MediaAssetProcessorService {
    processor: MediaProcessor,
}

#[tonic::async_trait]
impl MediaAssetProcessor for MediaAssetProcessorService {
    async fn process_asset(
        &self,
        request: Request<ProcessAssetRequest>,
    ) -> Result<Response<ProcessAssetResponse>, Status> {
        let request = request.into_inner();
        let source = request
            .source
            .ok_or_else(|| Status::invalid_argument("media source is required"))?;

        let asset = match source.value {
            Some(media_source::Value::DataUrl(data_url)) => {
                let max_bytes = request.constraints.as_ref().map(|value| value.max_bytes);
                let processed = self
                    .processor
                    .process_data_url(&data_url, max_bytes)
                    .map_err(media_error_to_status)?;
                ProcessedAsset {
                    content: Some(processed_asset::Content::InlineBytes(processed.bytes)),
                    mime_type: processed.mime_type,
                    byte_size: processed.byte_size,
                    width: 0,
                    height: 0,
                    sha256: String::new(),
                }
            }
            Some(media_source::Value::ObjectRef(object_ref)) => ProcessedAsset {
                content: Some(processed_asset::Content::ObjectRef(object_ref)),
                mime_type: String::new(),
                byte_size: 0,
                width: 0,
                height: 0,
                sha256: String::new(),
            },
            Some(media_source::Value::Url(_)) => {
                return Err(Status::unimplemented(
                    "remote URL media processing is not scaffolded yet",
                ));
            }
            None => return Err(Status::invalid_argument("media source value is required")),
        };

        Ok(Response::new(ProcessAssetResponse {
            asset: Some(asset),
            status: "ready".to_string(),
            warnings: Vec::new(),
        }))
    }
}

#[derive(Debug, Default)]
struct PlatformDraftCompilerService {
    compiler: DraftCompiler,
}

#[tonic::async_trait]
impl PlatformDraftCompiler for PlatformDraftCompilerService {
    async fn compile_drafts(
        &self,
        request: Request<CompileDraftsRequest>,
    ) -> Result<Response<CompileDraftsResponse>, Status> {
        let request = request.into_inner();
        let project = request
            .project
            .ok_or_else(|| Status::invalid_argument("source project is required"))?;
        let project = SourceProject {
            id: project.id,
            title: project.title,
            source_format: project.source_format,
            source_content: project.source_content,
        };

        let mut drafts = Vec::with_capacity(request.targets.len());
        for target in request.targets {
            let target = DraftTarget {
                platform: target.platform,
                profile: target.profile,
                config_json: target.config_json,
            };
            let output = self
                .compiler
                .compile(&project, &target)
                .map_err(draft_error_to_status)?;
            drafts.push(CompiledDraft {
                platform: output.platform,
                profile: output.profile,
                status: output.status,
                adapted_content_json: output.adapted_content_json,
                summary: output.summary,
                warnings: output.warnings,
            });
        }

        Ok(Response::new(CompileDraftsResponse { drafts }))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    init_tracing();

    let addr = service_addr()?;
    let media_service = MediaAssetProcessorServer::new(MediaAssetProcessorService::default());
    let draft_service = PlatformDraftCompilerServer::new(PlatformDraftCompilerService::default());
    let (health_reporter, health_service) = tonic_health::server::health_reporter();
    health_reporter
        .set_serving::<MediaAssetProcessorServer<MediaAssetProcessorService>>()
        .await;
    health_reporter
        .set_serving::<PlatformDraftCompilerServer<PlatformDraftCompilerService>>()
        .await;

    let reflection_service = tonic_reflection::server::Builder::configure()
        .register_encoded_file_descriptor_set(content_pipeline_proto::FILE_DESCRIPTOR_SET)
        .build_v1()?;

    info!(%addr, "starting content-pipeline-service");
    Server::builder()
        .add_service(health_service)
        .add_service(reflection_service)
        .add_service(media_service)
        .add_service(draft_service)
        .serve(addr)
        .await?;

    Ok(())
}

fn init_tracing() {
    let filter = EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| EnvFilter::new("content_pipeline_service=info"));
    tracing_subscriber::fmt().with_env_filter(filter).init();
}

fn service_addr() -> Result<SocketAddr, std::net::AddrParseError> {
    std::env::var("CONTENT_PIPELINE_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50051".to_string())
        .parse()
}

fn media_error_to_status(err: content_pipeline_core::MediaError) -> Status {
    match err {
        content_pipeline_core::MediaError::EmptySource
        | content_pipeline_core::MediaError::InvalidDataUrl
        | content_pipeline_core::MediaError::UnsupportedSource => {
            Status::invalid_argument(err.to_string())
        }
        content_pipeline_core::MediaError::ResourceLimitExceeded { .. } => {
            Status::resource_exhausted(err.to_string())
        }
    }
}

fn draft_error_to_status(err: content_pipeline_core::DraftCompileError) -> Status {
    match err {
        content_pipeline_core::DraftCompileError::EmptySource => {
            Status::invalid_argument(err.to_string())
        }
        content_pipeline_core::DraftCompileError::Encode(_) => Status::internal(err.to_string()),
    }
}
