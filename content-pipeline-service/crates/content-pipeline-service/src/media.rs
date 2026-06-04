use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use std::time::Duration;

use content_pipeline_core::{DEFAULT_MAX_BYTES, MediaConstraints, MediaProcessor};
use content_pipeline_proto::mpp::contentpipeline::v1::{
    ProcessAssetRequest, ProcessAssetResponse, ProcessedAsset,
    media_asset_processor_server::MediaAssetProcessor, media_source, processed_asset,
};
use futures_util::StreamExt;
use reqwest::header::CONTENT_TYPE;
use reqwest::redirect::Policy;
use reqwest::{Client, Url};
use tonic::{Request, Response, Status};

const MEDIA_DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(20);
const MEDIA_REDIRECT_LIMIT: usize = 3;

#[derive(Debug, Clone)]
pub(crate) struct MediaAssetProcessorService {
    processor: MediaProcessor,
    http_client: Client,
}

impl MediaAssetProcessorService {
    pub(crate) fn new() -> Result<Self, reqwest::Error> {
        Ok(Self {
            processor: MediaProcessor::new(),
            http_client: media_http_client()?,
        })
    }

    async fn process_url(
        &self,
        source_url: &str,
        constraints: &MediaConstraints,
    ) -> Result<content_pipeline_core::ProcessedAsset, Status> {
        let url =
            Url::parse(source_url).map_err(|_| Status::invalid_argument("invalid media URL"))?;
        if !is_safe_media_url(&url) {
            return Err(Status::invalid_argument("unsafe media URL"));
        }

        let response = self
            .http_client
            .get(url)
            .send()
            .await
            .map_err(media_download_error_to_status)?;

        if !response.status().is_success() {
            return Err(Status::unavailable(format!(
                "media download returned HTTP {}",
                response.status().as_u16()
            )));
        }

        let declared_mime_type = response_content_type(&response);
        let bytes =
            read_limited_body(response, constraints.max_bytes.unwrap_or(DEFAULT_MAX_BYTES)).await?;

        self.processor
            .process_bytes(bytes, declared_mime_type.as_deref(), constraints)
            .map_err(media_error_to_status)
    }
}

#[tonic::async_trait]
impl MediaAssetProcessor for MediaAssetProcessorService {
    async fn process_asset(
        &self,
        request: Request<ProcessAssetRequest>,
    ) -> Result<Response<ProcessAssetResponse>, Status> {
        let request = request.into_inner();
        let constraints = media_constraints_from_request(&request);
        let source = request
            .source
            .ok_or_else(|| Status::invalid_argument("media source is required"))?;

        let (asset, warnings) = match source.value {
            Some(media_source::Value::DataUrl(data_url)) => {
                let processed = self
                    .processor
                    .process_data_url(&data_url, &constraints)
                    .map_err(media_error_to_status)?;
                processed_asset_to_proto(processed)
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
            ),
            Some(media_source::Value::Url(url)) => {
                let processed = self.process_url(&url, &constraints).await?;
                processed_asset_to_proto(processed)
            }
            None => return Err(Status::invalid_argument("media source value is required")),
        };

        Ok(Response::new(ProcessAssetResponse {
            asset: Some(asset),
            status: "processed".to_string(),
            warnings,
        }))
    }
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

fn media_http_client() -> Result<Client, reqwest::Error> {
    Client::builder()
        .timeout(MEDIA_DOWNLOAD_TIMEOUT)
        .redirect(Policy::custom(|attempt| {
            if attempt.previous().len() >= MEDIA_REDIRECT_LIMIT {
                attempt.stop()
            } else if is_safe_media_url(attempt.url()) {
                attempt.follow()
            } else {
                attempt.error("unsafe media redirect")
            }
        }))
        .build()
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

fn is_safe_media_url(url: &Url) -> bool {
    if url.scheme() != "https" {
        return false;
    }

    let Some(host) = url.host_str() else {
        return false;
    };

    let host = host.trim_end_matches('.');
    let host = host
        .strip_prefix('[')
        .and_then(|value| value.strip_suffix(']'))
        .unwrap_or(host)
        .to_ascii_lowercase();
    if host == "localhost" || host.ends_with(".localhost") {
        return false;
    }

    match host.parse::<IpAddr>() {
        Ok(ip) => is_public_ip(ip),
        Err(_) => true,
    }
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
        | content_pipeline_core::MediaError::DecodeImage => {
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
    fn allows_public_https_media_url() {
        assert!(is_safe_media_url(&url("https://example.com/image.png")));
    }

    #[test]
    fn rejects_non_https_media_url() {
        assert!(!is_safe_media_url(&url("http://example.com/image.png")));
    }

    #[test]
    fn rejects_localhost_media_url() {
        assert!(!is_safe_media_url(&url("https://localhost/image.png")));
        assert!(!is_safe_media_url(&url(
            "https://assets.localhost/image.png"
        )));
    }

    #[test]
    fn rejects_private_ip_media_url() {
        assert!(!is_safe_media_url(&url("https://127.0.0.1/image.png")));
        assert!(!is_safe_media_url(&url("https://10.0.0.1/image.png")));
        assert!(!is_safe_media_url(&url("https://192.168.1.10/image.png")));
        assert!(!is_safe_media_url(&url("https://[::1]/image.png")));
        assert!(!is_safe_media_url(&url("https://[fd00::1]/image.png")));
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
