use std::io;
use std::panic::{AssertUnwindSafe, catch_unwind};

use image::{DynamicImage, Rgb};
use mozjpeg::{ColorSpace, Compress};

use super::{CandidateSearch, ProcessedImage};
use crate::media::MediaError;

pub(super) const VISUAL_MAX_QUALITY: u8 = 95;
pub(super) const VISUAL_MIN_QUALITY: u8 = 80;
pub(super) const RESIZE_MAX_QUALITY: u8 = 90;
pub(super) const RESIZE_MIN_QUALITY: u8 = 80;

pub(super) fn add_jpeg_candidate(
    search: &mut CandidateSearch<'_>,
    image: &DynamicImage,
    max_quality: u8,
    min_quality: u8,
) -> Result<(), MediaError> {
    let mut low = min_quality;
    let mut high = max_quality;

    while low <= high {
        let quality = low + (high - low) / 2;
        let encoded = encode_jpeg(image, quality)?;
        let encoded_size = encoded.len() as u64;
        search.consider(
            ProcessedImage {
                bytes: encoded,
                mime_type: "image/jpeg".to_string(),
                width: image.width(),
                height: image.height(),
            },
            jpeg_quality_score(image, quality),
        );

        if encoded_size <= search.max_bytes {
            low = quality.saturating_add(1);
        } else if quality == 0 {
            break;
        } else {
            high = quality - 1;
        }
    }

    Ok(())
}

fn jpeg_quality_score(image: &DynamicImage, quality: u8) -> u16 {
    let score = u16::from(quality) * 10;
    if image.has_alpha() { score / 2 } else { score }
}

fn encode_jpeg(image: &DynamicImage, quality: u8) -> Result<Vec<u8>, MediaError> {
    let rgb = flatten_for_jpeg(image);
    catch_unwind(AssertUnwindSafe(|| -> io::Result<Vec<u8>> {
        let mut compressor = Compress::new(ColorSpace::JCS_RGB);
        compressor.set_size(rgb.width() as usize, rgb.height() as usize);
        compressor.set_quality(f32::from(quality));
        compressor.set_optimize_coding(true);
        if quality >= 90 {
            compressor.set_chroma_sampling_pixel_sizes((1, 1), (1, 1));
        } else {
            compressor.set_chroma_sampling_pixel_sizes((2, 2), (2, 2));
        }

        let mut compressor = compressor.start_compress(Vec::new())?;
        compressor.write_scanlines(rgb.as_raw())?;
        compressor.finish()
    }))
    .map_err(|_| MediaError::EncodeImage)?
    .map_err(|_| MediaError::EncodeImage)
}

fn flatten_for_jpeg(image: &DynamicImage) -> image::RgbImage {
    if !image.has_alpha() {
        return image.to_rgb8();
    }

    let rgba = image.to_rgba8();
    image::RgbImage::from_fn(rgba.width(), rgba.height(), |x, y| {
        let [red, green, blue, alpha] = rgba.get_pixel(x, y).0;
        let alpha = u16::from(alpha);
        let inverse = 255 - alpha;
        Rgb([
            composite_on_white(red, alpha, inverse),
            composite_on_white(green, alpha, inverse),
            composite_on_white(blue, alpha, inverse),
        ])
    })
}

fn composite_on_white(channel: u8, alpha: u16, inverse_alpha: u16) -> u8 {
    ((u16::from(channel) * alpha + 255 * inverse_alpha) / 255) as u8
}
