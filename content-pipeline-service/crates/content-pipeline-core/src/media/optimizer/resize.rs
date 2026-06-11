use image::DynamicImage;
use image::imageops::FilterType;

use super::{
    CandidateSearch, EncodeQualities, ProcessedImage, add_encoded_candidates, conversion, lossless,
    perceptual,
};
use crate::media::{MediaConstraints, MediaError};

const RESIZE_PERCENT_STEPS: &[u32] = &[
    90, 81, 73, 66, 59, 53, 48, 43, 39, 35, 31, 28, 25, 22, 20, 18, 16, 14, 12, 10, 8, 6, 5, 4, 3,
    2, 1,
];

pub(super) fn find_candidate(
    image: &DynamicImage,
    max_bytes: u64,
    constraints: &MediaConstraints,
    original_mime_type: &str,
    smallest_encoded_size: &mut u64,
) -> Result<Option<ProcessedImage>, MediaError> {
    for (next_width, next_height) in resize_dimensions(image.width(), image.height()) {
        let resized = image.resize_exact(next_width, next_height, FilterType::Lanczos3);
        let mut step_best = None;

        {
            let mut search = CandidateSearch::new(
                &mut step_best,
                max_bytes,
                constraints,
                original_mime_type,
                smallest_encoded_size,
            );

            if constraints.allows_output_mime_type("image/png") && image.has_alpha() {
                let encoded = lossless::encode_png(&resized)?;
                search.consider(
                    ProcessedImage {
                        bytes: encoded,
                        mime_type: "image/png".to_string(),
                        width: resized.width(),
                        height: resized.height(),
                    },
                    900,
                );
            }

            add_encoded_candidates(
                &mut search,
                &resized,
                EncodeQualities {
                    jpeg_max: perceptual::RESIZE_MAX_QUALITY,
                    jpeg_min: perceptual::RESIZE_MIN_QUALITY,
                    webp: conversion::WEBP_RESIZE_QUALITIES,
                    avif: conversion::AVIF_RESIZE_QUALITIES,
                },
            )?;
        }

        if let Some(candidate) = step_best {
            return Ok(Some(candidate.image));
        }
    }

    Ok(None)
}

fn scaled_dimension(value: u32, percent: u32) -> u32 {
    ((u64::from(value) * u64::from(percent)) / 100)
        .max(1)
        .min(u64::from(u32::MAX)) as u32
}

fn resize_dimensions(width: u32, height: u32) -> Vec<(u32, u32)> {
    let mut dimensions = Vec::new();
    let mut push_unique = |next_width, next_height| {
        if dimensions
            .last()
            .is_none_or(|previous| *previous != (next_width, next_height))
        {
            dimensions.push((next_width, next_height));
        }
    };

    for percent in RESIZE_PERCENT_STEPS {
        push_unique(
            scaled_dimension(width, *percent),
            scaled_dimension(height, *percent),
        );
    }

    let mut next_width = scaled_dimension(width, 1);
    let mut next_height = scaled_dimension(height, 1);
    while next_width > 1 || next_height > 1 {
        next_width = (next_width / 2).max(1);
        next_height = (next_height / 2).max(1);
        push_unique(next_width, next_height);
    }

    dimensions
}
