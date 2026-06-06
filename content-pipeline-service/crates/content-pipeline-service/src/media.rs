use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::Duration;

use content_pipeline_core::{DEFAULT_MAX_BYTES, MediaConstraints, MediaProcessor};
use content_pipeline_proto::mpp::contentpipeline::v1::{
    ProcessAssetRequest, ProcessAssetResponse, ProcessedAsset,
    media_asset_processor_server::MediaAssetProcessor, media_source, processed_asset,
};
use futures_util::StreamExt;
use reqwest::header::{CONTENT_TYPE, LOCATION};
use reqwest::redirect::Policy;
use reqwest::{Client, Response as HttpResponse, Url};
use tokio::time::timeout;
use tonic::{Request, Response, Status};

use crate::metrics::{ContentPipelineMetrics, MediaSourceKind};

const MEDIA_DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(20);
const MEDIA_REDIRECT_LIMIT: usize = 3;

#[derive(Debug, Clone)]
pub(crate) struct MediaAssetProcessorService {
    processor: MediaProcessor,
    metrics: ContentPipelineMetrics,
}

impl MediaAssetProcessorService {
    pub(crate) fn new(metrics: ContentPipelineMetrics) -> Result<Self, reqwest::Error> {
        Ok(Self {
            processor: MediaProcessor::new(),
            metrics,
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
                let (asset, warnings) = processed_asset_to_proto(processed);
                (
                    asset,
                    warnings,
                    MediaSourceKind::DataUrl,
                    Some(input_bytes),
                    Some(output_bytes),
                )
            }
            Some(media_source::Value::ObjectRef(object_ref)) => (
                ProcessedAsset {
                    content: Some(processed_asset::Content::ObjectRef(object_ref)),
                    mime_type: String::new(),
                    byte_size: 0,
                    width: 0,
                    height: 0,
                    sha256: String::new(),
                },
                vec!["object_ref passthrough was not validated".to_string()],
                MediaSourceKind::ObjectRef,
                None,
                None,
            ),
            Some(media_source::Value::Url(url)) => {
                let processed = self.process_url(&url, &constraints).await?;
                let input_bytes = processed.input_byte_size;
                let output_bytes = processed.byte_size;
                let (asset, warnings) = processed_asset_to_proto(processed);
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

fn processed_asset_to_proto(
    processed: content_pipeline_core::ProcessedAsset,
) -> (ProcessedAsset, Vec<String>) {
    let warnings = processed.warnings.clone();
    (
        ProcessedAsset {
            content: Some(processed_asset::Content::InlineBytes(processed.bytes)),
            mime_type: processed.mime_type,
            byte_size: processed.byte_size,
            width: processed.width,
            height: processed.height,
            sha256: processed.sha256,
        },
        warnings,
    )
}

fn media_http_client(resolved_host: Option<&ResolvedHost>) -> Result<Client, reqwest::Error> {
    let builder = Client::builder()
        .timeout(MEDIA_DOWNLOAD_TIMEOUT)
        .redirect(Policy::none());

    match resolved_host {
        Some(resolved_host) => builder
            .resolve_to_addrs(&resolved_host.host, &resolved_host.addrs)
            .build(),
        None => builder.build(),
    }
}

async fn fetch_media_url(mut url: Url) -> Result<HttpResponse, Status> {
    let mut redirects = 0;

    loop {
        let validated = validate_media_url(url).await?;
        let client = media_http_client(validated.resolved_host.as_ref())
            .map_err(|_| Status::internal("failed to build media HTTP client"))?;
        let response = client
            .get(validated.url)
            .send()
            .await
            .map_err(media_download_error_to_status)?;

        if !response.status().is_redirection() {
            return Ok(response);
        }

        if redirects >= MEDIA_REDIRECT_LIMIT {
            return Err(Status::invalid_argument("media redirect limit exceeded"));
        }
        redirects += 1;

        let base_url = response.url().clone();
        let location = response
            .headers()
            .get(LOCATION)
            .and_then(|value| value.to_str().ok())
            .ok_or_else(|| Status::invalid_argument("media redirect missing location"))?;
        url = base_url
            .join(location)
            .map_err(|_| Status::invalid_argument("invalid media redirect URL"))?;
    }
}

async fn read_limited_body(response: reqwest::Response, max_bytes: u64) -> Result<Vec<u8>, Status> {
    if let Some(content_length) = response.content_length()
        && content_length > max_bytes
    {
        return Err(Status::resource_exhausted(format!(
            "media exceeds max bytes: {content_length} > {max_bytes}"
        )));
    }

    let mut body = Vec::new();
    let mut stream = response.bytes_stream();
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(media_download_error_to_status)?;
        let next_len = body
            .len()
            .checked_add(chunk.len())
            .ok_or_else(|| Status::resource_exhausted("media body is too large"))?;
        let next_len = u64::try_from(next_len)
            .map_err(|_| Status::resource_exhausted("media body is too large"))?;
        if next_len > max_bytes {
            return Err(Status::resource_exhausted(format!(
                "media exceeds max bytes: {next_len} > {max_bytes}"
            )));
        }
        body.extend_from_slice(&chunk);
    }

    Ok(body)
}

fn response_content_type(response: &reqwest::Response) -> Option<String> {
    response
        .headers()
        .get(CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.split(';').next())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn media_download_error_to_status(err: reqwest::Error) -> Status {
    if err.is_timeout() {
        Status::deadline_exceeded("media download timed out")
    } else if err.is_redirect() {
        Status::invalid_argument("unsafe media redirect")
    } else {
        Status::unavailable("failed to download media")
    }
}

#[derive(Debug)]
struct ValidatedMediaUrl {
    url: Url,
    resolved_host: Option<ResolvedHost>,
}

#[derive(Debug)]
struct ResolvedHost {
    host: String,
    addrs: Vec<SocketAddr>,
}

async fn validate_media_url(mut url: Url) -> Result<ValidatedMediaUrl, Status> {
    if url.scheme() != "https" {
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    let host = normalized_media_host(&url)?;
    if is_blocked_hostname(&host) {
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    if let Ok(ip) = host.parse::<IpAddr>() {
        if is_public_ip(ip) {
            return Ok(ValidatedMediaUrl {
                url,
                resolved_host: None,
            });
        }
        return Err(Status::invalid_argument("unsafe media URL"));
    }

    url.set_host(Some(&host))
        .map_err(|_| Status::invalid_argument("invalid media URL"))?;
    let port = url
        .port_or_known_default()
        .ok_or_else(|| Status::invalid_argument("invalid media URL port"))?;
    let addrs = resolve_host_addrs(&host, port).await?;
    validate_resolved_addrs(&host, &addrs)?;

    Ok(ValidatedMediaUrl {
        url,
        resolved_host: Some(ResolvedHost { host, addrs }),
    })
}

fn normalized_media_host(url: &Url) -> Result<String, Status> {
    let Some(host) = url.host_str() else {
        return Err(Status::invalid_argument("invalid media URL host"));
    };

    let host = host.trim_end_matches('.');
    Ok(host
        .strip_prefix('[')
        .and_then(|value| value.strip_suffix(']'))
        .unwrap_or(host)
        .to_ascii_lowercase())
}

fn is_blocked_hostname(host: &str) -> bool {
    host == "localhost" || host.ends_with(".localhost")
}

async fn resolve_host_addrs(host: &str, port: u16) -> Result<Vec<SocketAddr>, Status> {
    let lookup = timeout(
        MEDIA_DOWNLOAD_TIMEOUT,
        tokio::net::lookup_host((host, port)),
    )
    .await
    .map_err(|_| Status::deadline_exceeded("media DNS lookup timed out"))?
    .map_err(|_| Status::unavailable("failed to resolve media host"))?;
    let addrs = lookup.collect::<Vec<_>>();
    if addrs.is_empty() {
        return Err(Status::unavailable("media host did not resolve"));
    }

    Ok(addrs)
}

fn validate_resolved_addrs(host: &str, addrs: &[SocketAddr]) -> Result<(), Status> {
    if addrs.iter().all(|addr| is_public_ip(addr.ip())) {
        return Ok(());
    }

    Err(Status::invalid_argument(format!(
        "media host {host} resolved to a private address"
    )))
}

fn is_public_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(ip) => is_public_ipv4(ip),
        IpAddr::V6(ip) => is_public_ipv6(ip),
    }
}

fn is_public_ipv4(ip: Ipv4Addr) -> bool {
    let [first, second, _, _] = ip.octets();

    !(first == 0
        || first == 10
        || first == 127
        || (first == 100 && (64..=127).contains(&second))
        || (first == 169 && second == 254)
        || (first == 172 && (16..=31).contains(&second))
        || (first == 192 && second == 168)
        || ip == Ipv4Addr::BROADCAST)
}

fn is_public_ipv6(ip: Ipv6Addr) -> bool {
    if let Some(ipv4) = ip.to_ipv4_mapped() {
        return is_public_ipv4(ipv4);
    }

    let first_segment = ip.segments()[0];
    let is_unique_local = (first_segment & 0xfe00) == 0xfc00;
    let is_link_local = (first_segment & 0xffc0) == 0xfe80;

    !(ip.is_loopback() || ip.is_unspecified() || is_unique_local || is_link_local)
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

    fn url(value: &str) -> Url {
        Url::parse(value).expect("test URL should parse")
    }

    #[test]
    fn accepts_public_resolved_addresses() {
        let addrs = vec!["93.184.216.34:443".parse().expect("address should parse")];

        validate_resolved_addrs("example.com", &addrs)
            .expect("public resolved address should be accepted");
    }

    #[test]
    fn rejects_private_resolved_addresses() {
        let addrs = vec![
            "93.184.216.34:443".parse().expect("address should parse"),
            "169.254.169.254:443".parse().expect("address should parse"),
        ];

        let err = validate_resolved_addrs("attacker.example", &addrs)
            .expect_err("private resolved address should be rejected");

        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_non_https_media_url() {
        let err = validate_media_url(url("http://example.com/image.png"))
            .await
            .expect_err("non-https URL should be rejected");

        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_localhost_media_url() {
        let err = validate_media_url(url("https://localhost/image.png"))
            .await
            .expect_err("localhost URL should be rejected");
        assert_eq!(err.code(), tonic::Code::InvalidArgument);

        let err = validate_media_url(url("https://assets.localhost/image.png"))
            .await
            .expect_err("localhost subdomain URL should be rejected");
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn rejects_private_ip_media_url() {
        for value in [
            "https://127.0.0.1/image.png",
            "https://10.0.0.1/image.png",
            "https://192.168.1.10/image.png",
            "https://[::1]/image.png",
            "https://[fd00::1]/image.png",
            "https://[::ffff:127.0.0.1]/image.png",
        ] {
            let err = validate_media_url(url(value))
                .await
                .expect_err("private IP URL should be rejected");
            assert_eq!(err.code(), tonic::Code::InvalidArgument);
        }
    }

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
}
