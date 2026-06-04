use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use thiserror::Error;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessedAsset {
    pub bytes: Vec<u8>,
    pub mime_type: String,
    pub byte_size: u64,
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum MediaError {
    #[error("media source is empty")]
    EmptySource,
    #[error("invalid data URL")]
    InvalidDataUrl,
    #[error("unsupported media source")]
    UnsupportedSource,
    #[error("media exceeds max bytes: {actual} > {max}")]
    ResourceLimitExceeded { actual: u64, max: u64 },
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
        max_bytes: Option<u64>,
    ) -> Result<ProcessedAsset, MediaError> {
        let data_url = data_url.trim();
        if data_url.is_empty() {
            return Err(MediaError::EmptySource);
        }

        let (metadata, payload) = data_url.split_once(',').ok_or(MediaError::InvalidDataUrl)?;
        if !metadata.starts_with("data:") {
            return Err(MediaError::InvalidDataUrl);
        }

        let mime_type = metadata
            .trim_start_matches("data:")
            .split(';')
            .next()
            .filter(|value| !value.is_empty())
            .unwrap_or("application/octet-stream")
            .to_string();

        let bytes = if metadata.contains(";base64") {
            STANDARD
                .decode(payload)
                .map_err(|_| MediaError::InvalidDataUrl)?
        } else {
            payload.as_bytes().to_vec()
        };

        let byte_size = bytes.len() as u64;
        if let Some(max) = max_bytes {
            if byte_size > max {
                return Err(MediaError::ResourceLimitExceeded {
                    actual: byte_size,
                    max,
                });
            }
        }

        Ok(ProcessedAsset {
            bytes,
            mime_type,
            byte_size,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decodes_base64_data_url() {
        let processor = MediaProcessor::new();

        let asset = processor
            .process_data_url("data:text/plain;base64,SGVsbG8=", Some(16))
            .expect("data url should decode");

        assert_eq!(asset.bytes, b"Hello");
        assert_eq!(asset.mime_type, "text/plain");
        assert_eq!(asset.byte_size, 5);
    }

    #[test]
    fn rejects_oversized_data_url() {
        let processor = MediaProcessor::new();

        let err = processor
            .process_data_url("data:text/plain;base64,SGVsbG8=", Some(4))
            .expect_err("asset should exceed limit");

        assert_eq!(err, MediaError::ResourceLimitExceeded { actual: 5, max: 4 });
    }
}
