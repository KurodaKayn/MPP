use std::io::Cursor;

mod optimizer;
mod profiles;

use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use image::{ImageFormat, ImageReader};
use optimizer::{ProcessedImage, optimize_image_to_constraints};
use percent_encoding::percent_decode_str;
pub use profiles::{MediaProfile, supported_media_profiles};
use sha2::{Digest, Sha256};
use thiserror::Error;

pub const DEFAULT_MAX_BYTES: u64 = 10 * 1024 * 1024;
pub const WECHAT_MAX_BYTES: u64 = 2 * 1024 * 1024;
pub const X_MAX_BYTES: u64 = 5 * 1024 * 1024;
pub const MAX_DECODED_PIXELS: u64 = 40_000_000;

const SUPPORTED_IMAGE_MIME_TYPES: &[&str] = &[
    "image/png",
    "image/jpeg",
    "image/gif",
    "image/webp",
    "image/avif",
];
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessedAsset {
    pub input_byte_size: u64,
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
    compress_to_max_bytes: bool,
    output_mime_types: Vec<String>,
}

impl MediaConstraints {
    pub fn new(max_bytes: Option<u64>, preferred_mime_types: Vec<String>) -> Self {
        Self {
            max_bytes,
            preferred_mime_types: normalize_mime_types(preferred_mime_types),
            compress_to_max_bytes: false,
            output_mime_types: SUPPORTED_IMAGE_MIME_TYPES
                .iter()
                .map(|mime_type| (*mime_type).to_string())
                .collect(),
        }
    }

    pub fn for_platform(
        platform: &str,
        usage: &str,
        max_bytes: Option<u64>,
        preferred_mime_types: Vec<String>,
    ) -> Self {
        let profile = profiles::resolve_media_profile(platform, usage);
        let mut constraints =
            Self::new(max_bytes.or(Some(profile.max_bytes)), preferred_mime_types);
        constraints.compress_to_max_bytes =
            profile.compress_to_max_bytes && constraints.max_bytes.is_some();
        constraints.output_mime_types = normalize_mime_types(
            profile
                .output_mime_types
                .iter()
                .map(|mime_type| (*mime_type).to_string())
                .collect(),
        );
        constraints
    }

    fn allows_output_mime_type(&self, mime_type: &str) -> bool {
        let profile_allows = self
            .output_mime_types
            .iter()
            .any(|allowed| allowed == mime_type);
        let preference_allows = self.preferred_mime_types.is_empty()
            || self
                .preferred_mime_types
                .iter()
                .any(|preferred| preferred == mime_type);

        profile_allows && preference_allows
    }

    fn allowed_mime_types_for_error(&self) -> String {
        let allowed = self
            .output_mime_types
            .iter()
            .filter(|mime_type| {
                self.preferred_mime_types.is_empty()
                    || self
                        .preferred_mime_types
                        .iter()
                        .any(|preferred| preferred == *mime_type)
            })
            .cloned()
            .collect::<Vec<_>>();

        if allowed.is_empty() {
            self.output_mime_types.join(", ")
        } else {
            allowed.join(", ")
        }
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
    #[error("failed to encode processed image")]
    EncodeImage,
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

        let input_byte_size = bytes.len() as u64;
        if input_byte_size > DEFAULT_MAX_BYTES {
            return Err(MediaError::ResourceLimitExceeded {
                actual: input_byte_size,
                max: DEFAULT_MAX_BYTES,
            });
        }

        let format = image::guess_format(&bytes).map_err(|_| MediaError::UnsupportedFormat)?;
        let mime_type = mime_type_for_format(format).ok_or(MediaError::UnsupportedFormat)?;
        validate_supported_mime_type(mime_type)?;
        validate_preferred_mime_types(constraints)?;

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

        let mut processed = ProcessedImage {
            bytes,
            mime_type: mime_type.to_string(),
            width,
            height,
        };

        if let Some(max) = constraints.max_bytes
            && constraints.compress_to_max_bytes
            && (processed.byte_size() > max || !constraints.allows_output_mime_type(mime_type))
        {
            processed = optimize_image_to_constraints(&processed.bytes, format, constraints)?;
            if processed.mime_type == mime_type {
                if processed.width == width && processed.height == height {
                    warnings.push("image optimized to satisfy media constraints".to_string());
                } else {
                    warnings.push("image resized to satisfy media constraints".to_string());
                }
            } else {
                warnings.push(format!(
                    "image converted from {mime_type} to {} to satisfy media constraints",
                    processed.mime_type
                ));
            }
        }

        validate_mime_type(&processed.mime_type, constraints)?;

        let byte_size = processed.byte_size();
        if let Some(max) = constraints.max_bytes
            && byte_size > max
        {
            return Err(MediaError::ResourceLimitExceeded {
                actual: byte_size,
                max,
            });
        }

        let sha256 = sha256_hex(&processed.bytes);

        Ok(ProcessedAsset {
            input_byte_size,
            bytes: processed.bytes,
            mime_type: processed.mime_type,
            byte_size,
            width: processed.width,
            height: processed.height,
            sha256,
            warnings,
        })
    }
}

pub fn default_max_bytes(platform: &str, usage: &str) -> u64 {
    profiles::resolve_media_profile(platform, usage).max_bytes
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

fn validate_supported_mime_type(mime_type: &str) -> Result<(), MediaError> {
    if !SUPPORTED_IMAGE_MIME_TYPES.contains(&mime_type) {
        return Err(MediaError::UnsupportedMimeType {
            mime_type: mime_type.to_string(),
            allowed: SUPPORTED_IMAGE_MIME_TYPES.join(", "),
        });
    }

    Ok(())
}

fn validate_mime_type(mime_type: &str, constraints: &MediaConstraints) -> Result<(), MediaError> {
    validate_supported_mime_type(mime_type)?;

    if !constraints.allows_output_mime_type(mime_type) {
        return Err(MediaError::UnsupportedMimeType {
            mime_type: mime_type.to_string(),
            allowed: constraints.allowed_mime_types_for_error(),
        });
    }

    Ok(())
}

fn validate_preferred_mime_types(constraints: &MediaConstraints) -> Result<(), MediaError> {
    for mime_type in &constraints.preferred_mime_types {
        validate_supported_mime_type(mime_type)?;
    }

    if !constraints.preferred_mime_types.is_empty()
        && !constraints.preferred_mime_types.iter().any(|mime_type| {
            constraints
                .output_mime_types
                .iter()
                .any(|allowed| allowed == mime_type)
        })
    {
        return Err(MediaError::UnsupportedMimeType {
            mime_type: constraints.preferred_mime_types.join(", "),
            allowed: constraints.output_mime_types.join(", "),
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
        ImageFormat::Avif => Some("image/avif"),
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
mod tests;
