use std::io::Cursor;

use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use image::{ImageFormat, ImageReader};
use percent_encoding::percent_decode_str;
use sha2::{Digest, Sha256};
use thiserror::Error;

pub const DEFAULT_MAX_BYTES: u64 = 10 * 1024 * 1024;
pub const WECHAT_MAX_BYTES: u64 = 2 * 1024 * 1024;
pub const MAX_DECODED_PIXELS: u64 = 40_000_000;

const SUPPORTED_IMAGE_MIME_TYPES: &[&str] = &["image/png", "image/jpeg", "image/gif", "image/webp"];

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessedAsset {
    pub bytes: Vec<u8>,
    pub mime_type: String,
    pub byte_size: u64,
    pub width: u32,
    pub height: u32,
    pub sha256: String,
    pub warnings: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MediaConstraints {
    pub max_bytes: Option<u64>,
    pub preferred_mime_types: Vec<String>,
}

impl MediaConstraints {
    pub fn new(max_bytes: Option<u64>, preferred_mime_types: Vec<String>) -> Self {
        Self {
            max_bytes,
            preferred_mime_types: normalize_mime_types(preferred_mime_types),
        }
    }

    pub fn for_platform(
        platform: &str,
        usage: &str,
        max_bytes: Option<u64>,
        preferred_mime_types: Vec<String>,
    ) -> Self {
        Self::new(
            max_bytes.or_else(|| Some(default_max_bytes(platform, usage))),
            preferred_mime_types,
        )
    }
}

impl Default for MediaConstraints {
    fn default() -> Self {
        Self::new(Some(DEFAULT_MAX_BYTES), Vec::new())
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum MediaError {
    #[error("media source is empty")]
    EmptySource,
    #[error("invalid data URL")]
    InvalidDataUrl,
    #[error("unsupported media source")]
    UnsupportedSource,
    #[error("unsupported image format")]
    UnsupportedFormat,
    #[error("unsupported MIME type: {mime_type}; allowed: {allowed}")]
    UnsupportedMimeType { mime_type: String, allowed: String },
    #[error("failed to decode image metadata")]
    DecodeImage,
    #[error("media exceeds max bytes: {actual} > {max}")]
    ResourceLimitExceeded { actual: u64, max: u64 },
    #[error("image exceeds max decoded pixels: {actual} > {max}")]
    ImageDimensionsExceeded { actual: u64, max: u64 },
}

#[derive(Debug, Default, Clone)]
pub struct MediaProcessor;

impl MediaProcessor {
    pub fn new() -> Self {
        Self
    }

    pub fn process_data_url(
        &self,
        data_url: &str,
        constraints: &MediaConstraints,
    ) -> Result<ProcessedAsset, MediaError> {
        let data_url = data_url.trim();
        if data_url.is_empty() {
            return Err(MediaError::EmptySource);
        }

        let (metadata, payload) = data_url.split_once(',').ok_or(MediaError::InvalidDataUrl)?;
        if !metadata.to_ascii_lowercase().starts_with("data:") {
            return Err(MediaError::InvalidDataUrl);
        }

        let declared_mime_type = metadata
            .get(5..)
            .ok_or(MediaError::InvalidDataUrl)?
            .split(';')
            .next()
            .filter(|value| !value.is_empty())
            .map(normalize_mime_type);

        let bytes = if metadata.to_ascii_lowercase().contains(";base64") {
            STANDARD
                .decode(payload)
                .map_err(|_| MediaError::InvalidDataUrl)?
        } else {
            percent_decode_str(payload).collect::<Vec<u8>>()
        };

        self.process_bytes(bytes, declared_mime_type.as_deref(), constraints)
    }

    pub fn process_bytes(
        &self,
        bytes: Vec<u8>,
        declared_mime_type: Option<&str>,
        constraints: &MediaConstraints,
    ) -> Result<ProcessedAsset, MediaError> {
        if bytes.is_empty() {
            return Err(MediaError::EmptySource);
        }

        let byte_size = bytes.len() as u64;
        if let Some(max) = constraints.max_bytes {
            if byte_size > max {
                return Err(MediaError::ResourceLimitExceeded {
                    actual: byte_size,
                    max,
                });
            }
        }

        let format = image::guess_format(&bytes).map_err(|_| MediaError::UnsupportedFormat)?;
        let mime_type = mime_type_for_format(format).ok_or(MediaError::UnsupportedFormat)?;
        validate_mime_type(mime_type, constraints)?;

        let (width, height) = image_dimensions(&bytes, format)?;
        let pixels = u64::from(width) * u64::from(height);
        if pixels > MAX_DECODED_PIXELS {
            return Err(MediaError::ImageDimensionsExceeded {
                actual: pixels,
                max: MAX_DECODED_PIXELS,
            });
        }

        let mut warnings = Vec::new();
        if let Some(declared_mime_type) = declared_mime_type {
            let declared_mime_type = normalize_mime_type(declared_mime_type);
            if declared_mime_type != mime_type {
                warnings.push(format!(
                    "declared MIME type {declared_mime_type} did not match detected {mime_type}"
                ));
            }
        }

        let sha256 = sha256_hex(&bytes);

        Ok(ProcessedAsset {
            bytes,
            mime_type: mime_type.to_string(),
            byte_size,
            width,
            height,
            sha256,
            warnings,
        })
    }
}

pub fn default_max_bytes(platform: &str, _usage: &str) -> u64 {
    match platform.trim().to_ascii_lowercase().as_str() {
        "wechat" => WECHAT_MAX_BYTES,
        _ => DEFAULT_MAX_BYTES,
    }
}

fn normalize_mime_types(values: Vec<String>) -> Vec<String> {
    values
        .into_iter()
        .map(|value| normalize_mime_type(&value))
        .filter(|value| !value.is_empty())
        .collect()
}

fn normalize_mime_type(value: &str) -> String {
    match value.trim().to_ascii_lowercase().as_str() {
        "image/jpg" => "image/jpeg".to_string(),
        normalized => normalized.to_string(),
    }
}

fn validate_mime_type(mime_type: &str, constraints: &MediaConstraints) -> Result<(), MediaError> {
    if !SUPPORTED_IMAGE_MIME_TYPES.contains(&mime_type) {
        return Err(MediaError::UnsupportedMimeType {
            mime_type: mime_type.to_string(),
            allowed: SUPPORTED_IMAGE_MIME_TYPES.join(", "),
        });
    }

    if !constraints.preferred_mime_types.is_empty()
        && !constraints
            .preferred_mime_types
            .iter()
            .any(|allowed| allowed == mime_type)
    {
        return Err(MediaError::UnsupportedMimeType {
            mime_type: mime_type.to_string(),
            allowed: constraints.preferred_mime_types.join(", "),
        });
    }

    Ok(())
}

fn mime_type_for_format(format: ImageFormat) -> Option<&'static str> {
    match format {
        ImageFormat::Png => Some("image/png"),
        ImageFormat::Jpeg => Some("image/jpeg"),
        ImageFormat::Gif => Some("image/gif"),
        ImageFormat::WebP => Some("image/webp"),
        _ => None,
    }
}

fn image_dimensions(bytes: &[u8], format: ImageFormat) -> Result<(u32, u32), MediaError> {
    ImageReader::with_format(Cursor::new(bytes), format)
        .into_dimensions()
        .map_err(|_| MediaError::DecodeImage)
}

fn sha256_hex(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    hex::encode(hasher.finalize())
}

#[cfg(test)]
mod tests {
    use super::*;

    const ONE_BY_ONE_GIF_DATA_URL: &str =
        "data:image/gif;base64,R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==";

    #[test]
    fn decodes_base64_image_data_url() {
        let processor = MediaProcessor::new();

        let asset = processor
            .process_data_url(
                ONE_BY_ONE_GIF_DATA_URL,
                &MediaConstraints::new(Some(128), Vec::new()),
            )
            .expect("data url should decode");

        assert_eq!(asset.mime_type, "image/gif");
        assert_eq!(asset.byte_size, asset.bytes.len() as u64);
        assert_eq!(asset.width, 1);
        assert_eq!(asset.height, 1);
        assert_eq!(asset.sha256, sha256_hex(&asset.bytes));
        assert!(asset.warnings.is_empty());
    }

    #[test]
    fn rejects_oversized_data_url() {
        let processor = MediaProcessor::new();

        let err = processor
            .process_data_url(
                ONE_BY_ONE_GIF_DATA_URL,
                &MediaConstraints::new(Some(4), Vec::new()),
            )
            .expect_err("asset should exceed limit");

        assert_eq!(
            err,
            MediaError::ResourceLimitExceeded { actual: 43, max: 4 }
        );
    }

    #[test]
    fn rejects_unsupported_image_payload() {
        let processor = MediaProcessor::new();

        let err = processor
            .process_data_url(
                "data:text/plain;base64,SGVsbG8=",
                &MediaConstraints::new(Some(128), Vec::new()),
            )
            .expect_err("plain text is not a processable image");

        assert_eq!(err, MediaError::UnsupportedFormat);
    }

    #[test]
    fn rejects_mime_type_outside_requested_preferences() {
        let processor = MediaProcessor::new();

        let err = processor
            .process_data_url(
                ONE_BY_ONE_GIF_DATA_URL,
                &MediaConstraints::new(Some(128), vec!["image/png".to_string()]),
            )
            .expect_err("gif should be outside preferred png constraint");

        assert_eq!(
            err,
            MediaError::UnsupportedMimeType {
                mime_type: "image/gif".to_string(),
                allowed: "image/png".to_string()
            }
        );
    }

    #[test]
    fn reports_declared_mime_mismatch_as_warning() {
        let processor = MediaProcessor::new();
        let mismatched_data_url = ONE_BY_ONE_GIF_DATA_URL.replacen("image/gif", "image/png", 1);

        let asset = processor
            .process_data_url(
                &mismatched_data_url,
                &MediaConstraints::new(Some(128), Vec::new()),
            )
            .expect("detected image content should still be processed");

        assert_eq!(asset.mime_type, "image/gif");
        assert_eq!(
            asset.warnings,
            vec!["declared MIME type image/png did not match detected image/gif"]
        );
    }
}
