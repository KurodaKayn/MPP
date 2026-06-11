use image::codecs::avif::AvifEncoder;
use image::{DynamicImage, ExtendedColorType, ImageEncoder};

use super::{CandidateSearch, ProcessedImage};
use crate::media::MediaError;

pub(super) const WEBP_VISUAL_QUALITIES: &[f32] = &[92.0, 90.0, 88.0, 85.0, 82.0, 80.0];
pub(super) const WEBP_RESIZE_QUALITIES: &[f32] = &[90.0, 88.0, 85.0, 82.0, 80.0, 76.0, 72.0];
pub(super) const AVIF_VISUAL_QUALITIES: &[u8] = &[88, 85, 82, 80, 76, 72];
pub(super) const AVIF_RESIZE_QUALITIES: &[u8] = &[85, 82, 80, 76, 72, 68, 64];

pub(super) fn add_candidates(
    search: &mut CandidateSearch<'_>,
    image: &DynamicImage,
    webp_qualities: &[f32],
    avif_qualities: &[u8],
) -> Result<(), MediaError> {
    if search.constraints.allows_output_mime_type("image/webp") {
        for quality in webp_qualities {
            let encoded = encode_webp(image, *quality)?;
            search.consider(
                ProcessedImage {
                    bytes: encoded,
                    mime_type: "image/webp".to_string(),
                    width: image.width(),
                    height: image.height(),
                },
                (*quality * 10.0).round() as u16,
            );
        }
    }

    if search.constraints.allows_output_mime_type("image/avif") {
        for quality in avif_qualities {
            let encoded = encode_avif(image, *quality)?;
            search.consider(
                ProcessedImage {
                    bytes: encoded,
                    mime_type: "image/avif".to_string(),
                    width: image.width(),
                    height: image.height(),
                },
                u16::from(*quality) * 10,
            );
        }
    }

    Ok(())
}

fn encode_webp(image: &DynamicImage, quality: f32) -> Result<Vec<u8>, MediaError> {
    let rgba = image.to_rgba8();
    webp::Encoder::from_rgba(rgba.as_raw(), rgba.width(), rgba.height())
        .encode_simple(false, quality)
        .map(|encoded| encoded.to_vec())
        .map_err(|_| MediaError::EncodeImage)
}

fn encode_avif(image: &DynamicImage, quality: u8) -> Result<Vec<u8>, MediaError> {
    let rgba = image.to_rgba8();
    let mut output = Vec::new();
    AvifEncoder::new_with_speed_quality(&mut output, 6, quality)
        .write_image(
            rgba.as_raw(),
            rgba.width(),
            rgba.height(),
            ExtendedColorType::Rgba8,
        )
        .map_err(|_| MediaError::EncodeImage)?;
    Ok(output)
}
