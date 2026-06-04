mod drafts;
mod media;

use std::net::SocketAddr;

use content_pipeline_proto::mpp::contentpipeline::v1::media_asset_processor_server::MediaAssetProcessorServer;
use content_pipeline_proto::mpp::contentpipeline::v1::platform_draft_compiler_server::PlatformDraftCompilerServer;
use drafts::PlatformDraftCompilerService;
use media::MediaAssetProcessorService;
use tonic::transport::Server;
use tracing::info;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    init_tracing();

    let addr = service_addr()?;
    let media_service = MediaAssetProcessorServer::new(MediaAssetProcessorService::new()?);
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
