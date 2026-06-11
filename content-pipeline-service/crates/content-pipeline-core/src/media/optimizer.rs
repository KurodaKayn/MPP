mod conversion;
mod lossless;
mod perceptual;
mod resize;

use image::{DynamicImage, ImageFormat};

use super::{MediaConstraints, MediaError, mime_type_for_format};

pub(super) struct ProcessedImage {
    pub(super) bytes: Vec<u8>,
    pub(super) mime_type: String,
    pub(super) width: u32,
    pub(super) height: u32,
}

impl ProcessedImage {
    pub(super) fn byte_size(&self) -> u64 {
        self.bytes.len() as u64
    }
}

struct CompressionCandidate {
    image: ProcessedImage,
    pixel_count: u64,
    quality_score: u16,
    conversion_penalty: u8,
}

impl CompressionCandidate {
    fn new(image: ProcessedImage, original_mime_type: &str, quality_score: u16) -> Self {
        let conversion_penalty = if image.mime_type == original_mime_type {
            0
        } else {
            1
        };
        Self {
            pixel_count: u64::from(image.width) * u64::from(image.height),
            image,
            quality_score,
            conversion_penalty,
        }
    }

    fn better_than(&self, other: &Self) -> bool {
        self.pixel_count > other.pixel_count
            || (self.pixel_count == other.pixel_count
                && (self.quality_score > other.quality_score
                    || (self.quality_score == other.quality_score
                        && (self.conversion_penalty < other.conversion_penalty
                            || (self.conversion_penalty == other.conversion_penalty
                                && self.image.byte_size() < other.image.byte_size())))))
    }
}

pub(super) fn optimize_image_to_constraints(
    bytes: &[u8],
    format: ImageFormat,
    constraints: &MediaConstraints,
) -> Result<ProcessedImage, MediaError> {
    let max_bytes = constraints
        .max_bytes
        .expect("compression is only called with a byte limit");
    let original_mime_type = mime_type_for_format(format).ok_or(MediaError::UnsupportedFormat)?;
    let mut smallest_encoded_size = bytes.len() as u64;
    let mut best = None;

    let image =
        image::load_from_memory_with_format(bytes, format).map_err(|_| MediaError::DecodeImage)?;

    {
        let mut search = CandidateSearch::new(
            &mut best,
            max_bytes,
            constraints,
            original_mime_type,
            &mut smallest_encoded_size,
        );

        if constraints.allows_output_mime_type(original_mime_type) {
            search.consider(
                ProcessedImage {
                    bytes: bytes.to_vec(),
                    mime_type: original_mime_type.to_string(),
                    width: image.width(),
                    height: image.height(),
                },
                1000,
            );
        }

        if format == ImageFormat::Png
            && constraints.allows_output_mime_type("image/png")
            && let Some(optimized) = lossless::optimize_png(bytes)
        {
            search.consider(
                ProcessedImage {
                    bytes: optimized,
                    mime_type: "image/png".to_string(),
                    width: image.width(),
                    height: image.height(),
                },
                1000,
            );
        }

        if format == ImageFormat::Jpeg
            && constraints.allows_output_mime_type("image/jpeg")
            && let Some(stripped) = lossless::strip_jpeg_metadata(bytes)
        {
            search.consider(
                ProcessedImage {
                    bytes: stripped,
                    mime_type: "image/jpeg".to_string(),
                    width: image.width(),
                    height: image.height(),
                },
                1000,
            );
        }

        add_encoded_candidates(
            &mut search,
            &image,
            EncodeQualities {
                jpeg_max: perceptual::VISUAL_MAX_QUALITY,
                jpeg_min: perceptual::VISUAL_MIN_QUALITY,
                webp: conversion::WEBP_VISUAL_QUALITIES,
                avif: conversion::AVIF_VISUAL_QUALITIES,
            },
        )?;
    }

    if let Some(candidate) = best {
        return Ok(candidate.image);
    }

    if let Some(resized) = resize::find_candidate(
        &image,
        max_bytes,
        constraints,
        original_mime_type,
        &mut smallest_encoded_size,
    )? {
        return Ok(resized);
    }

    Err(MediaError::ResourceLimitExceeded {
        actual: smallest_encoded_size,
        max: max_bytes,
    })
}

struct CandidateSearch<'a> {
    best: &'a mut Option<CompressionCandidate>,
    max_bytes: u64,
    constraints: &'a MediaConstraints,
    original_mime_type: &'a str,
    smallest_encoded_size: &'a mut u64,
}

impl<'a> CandidateSearch<'a> {
    fn new(
        best: &'a mut Option<CompressionCandidate>,
        max_bytes: u64,
        constraints: &'a MediaConstraints,
        original_mime_type: &'a str,
        smallest_encoded_size: &'a mut u64,
    ) -> Self {
        Self {
            best,
            max_bytes,
            constraints,
            original_mime_type,
            smallest_encoded_size,
        }
    }

    fn consider(&mut self, image: ProcessedImage, quality_score: u16) {
        *self.smallest_encoded_size = (*self.smallest_encoded_size).min(image.byte_size());
        if image.byte_size() > self.max_bytes
            || !self.constraints.allows_output_mime_type(&image.mime_type)
        {
            return;
        }

        let candidate = CompressionCandidate::new(image, self.original_mime_type, quality_score);
        if self
            .best
            .as_ref()
            .is_none_or(|current| candidate.better_than(current))
        {
            *self.best = Some(candidate);
        }
    }
}

struct EncodeQualities<'a> {
    jpeg_max: u8,
    jpeg_min: u8,
    webp: &'a [f32],
    avif: &'a [u8],
}

fn add_encoded_candidates(
    search: &mut CandidateSearch<'_>,
    image: &DynamicImage,
    qualities: EncodeQualities<'_>,
) -> Result<(), MediaError> {
    if search.constraints.allows_output_mime_type("image/png") && image.has_alpha() {
        let encoded = lossless::encode_png(image)?;
        search.consider(
            ProcessedImage {
                bytes: encoded,
                mime_type: "image/png".to_string(),
                width: image.width(),
                height: image.height(),
            },
            1000,
        );
    }

    if search.constraints.allows_output_mime_type("image/jpeg") {
        perceptual::add_jpeg_candidate(search, image, qualities.jpeg_max, qualities.jpeg_min)?;
    }

    conversion::add_candidates(search, image, qualities.webp, qualities.avif)
}

#[cfg(test)]
pub(super) use lossless::strip_jpeg_metadata as strip_jpeg_metadata_lossless;
