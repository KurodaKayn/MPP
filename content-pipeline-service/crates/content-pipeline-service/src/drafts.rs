use content_pipeline_core::{DraftCompiler, DraftTarget, SourceProject};
use content_pipeline_proto::mpp::contentpipeline::v1::platform_draft_compiler_server::PlatformDraftCompiler;
use content_pipeline_proto::mpp::contentpipeline::v1::{
    CompileDraftsRequest, CompileDraftsResponse, CompiledDraft,
};
use tonic::{Request, Response, Status};

use crate::metrics::{ContentPipelineMetrics, draft_error_class};

#[derive(Debug)]
pub(crate) struct PlatformDraftCompilerService {
    compiler: DraftCompiler,
    metrics: ContentPipelineMetrics,
}

impl PlatformDraftCompilerService {
    pub(crate) fn new(metrics: ContentPipelineMetrics) -> Self {
        Self {
            compiler: DraftCompiler::new(),
            metrics,
        }
    }
}

#[tonic::async_trait]
impl PlatformDraftCompiler for PlatformDraftCompilerService {
    async fn compile_drafts(
        &self,
        request: Request<CompileDraftsRequest>,
    ) -> Result<Response<CompileDraftsResponse>, Status> {
        let request_started_at = std::time::Instant::now();
        let request = request.into_inner();
        let project = request.project.ok_or_else(|| {
            self.metrics.record_compile_drafts_error(
                "unknown",
                "invalid_input",
                request_started_at.elapsed(),
            );
            Status::invalid_argument("source project is required")
        })?;
        let project = SourceProject {
            id: project.id,
            title: project.title,
            source_format: project.source_format,
            source_content: project.source_content,
        };

        let mut drafts = Vec::with_capacity(request.targets.len());
        for target in request.targets {
            let started_at = std::time::Instant::now();
            let target = DraftTarget {
                platform: target.platform,
                profile: target.profile,
                config_json: target.config_json,
            };
            let output = match self.compiler.compile(&project, &target) {
                Ok(output) => output,
                Err(err) => {
                    self.metrics.record_compile_drafts_error(
                        &target.platform,
                        draft_error_class(&err),
                        started_at.elapsed(),
                    );
                    return Err(draft_error_to_status(err));
                }
            };
            self.metrics.record_compile_drafts_success(
                &output.platform,
                &output.profile,
                output.warnings.len(),
                started_at.elapsed(),
            );
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

fn draft_error_to_status(err: content_pipeline_core::DraftCompileError) -> Status {
    match err {
        content_pipeline_core::DraftCompileError::EmptySource
        | content_pipeline_core::DraftCompileError::UnsupportedSourceFormat(_)
        | content_pipeline_core::DraftCompileError::UnsupportedPlatform(_)
        | content_pipeline_core::DraftCompileError::UnsupportedProfile { .. } => {
            Status::invalid_argument(err.to_string())
        }
        content_pipeline_core::DraftCompileError::Encode(_) => Status::internal(err.to_string()),
    }
}
