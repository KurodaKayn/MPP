use std::io;
use std::panic::{AssertUnwindSafe, catch_unwind};

use image::codecs::avif::AvifEncoder;
use image::codecs::png::PngEncoder;
use image::imageops::FilterType;
use image::{DynamicImage, ExtendedColorType, ImageEncoder, ImageFormat, Rgb};
use mozjpeg::{ColorSpace, Compress};
use oxipng::{Options as OxipngOptions, StripChunks};

use super::{MediaConstraints, MediaError, mime_type_for_format};

const JPEG_VISUAL_MAX_QUALITY: u8 = 95;
const JPEG_VISUAL_MIN_QUALITY: u8 = 80;
const JPEG_RESIZE_MAX_QUALITY: u8 = 90;
const JPEG_RESIZE_MIN_QUALITY: u8 = 80;
const WEBP_VISUAL_QUALITIES: &[f32] = &[92.0, 90.0, 88.0, 85.0, 82.0, 80.0];
const WEBP_RESIZE_QUALITIES: &[f32] = &[90.0, 88.0, 85.0, 82.0, 80.0, 76.0, 72.0];
const AVIF_VISUAL_QUALITIES: &[u8] = &[88, 85, 82, 80, 76, 72];
const AVIF_RESIZE_QUALITIES: &[u8] = &[85, 82, 80, 76, 72, 68, 64];
const RESIZE_PERCENT_STEPS: &[u32] = &[90, 81, 73, 66, 59, 53, 48, 43, 39, 35, 31, 28];

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
            && let Some(optimized) = optimize_png_lossless(bytes)
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
            && let Some(stripped) = strip_jpeg_metadata_lossless(bytes)
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
                jpeg_max: JPEG_VISUAL_MAX_QUALITY,
                jpeg_min: JPEG_VISUAL_MIN_QUALITY,
                webp: WEBP_VISUAL_QUALITIES,
                avif: AVIF_VISUAL_QUALITIES,
            },
        )?;
    }

    if let Some(candidate) = best {
        return Ok(candidate.image);
    }

    for percent in RESIZE_PERCENT_STEPS {
        let next_width = scaled_dimension(image.width(), *percent);
        let next_height = scaled_dimension(image.height(), *percent);
        let resized = image.resize_exact(next_width, next_height, FilterType::Lanczos3);
        let mut step_best = None;

        {
            let mut search = CandidateSearch::new(
                &mut step_best,
                max_bytes,
                constraints,
                original_mime_type,
                &mut smallest_encoded_size,
            );

            if constraints.allows_output_mime_type("image/png") && image.has_alpha() {
                let encoded = encode_png(&resized)?;
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
                    jpeg_max: JPEG_RESIZE_MAX_QUALITY,
                    jpeg_min: JPEG_RESIZE_MIN_QUALITY,
                    webp: WEBP_RESIZE_QUALITIES,
                    avif: AVIF_RESIZE_QUALITIES,
                },
            )?;
        }

        if let Some(candidate) = step_best {
            return Ok(candidate.image);
        }
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
        let encoded = encode_png(image)?;
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
        add_jpeg_candidate(search, image, qualities.jpeg_max, qualities.jpeg_min)?;
    }

    if search.constraints.allows_output_mime_type("image/webp") {
        for quality in qualities.webp {
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
        for quality in qualities.avif {
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

fn add_jpeg_candidate(
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

fn optimize_png_lossless(bytes: &[u8]) -> Option<Vec<u8>> {
    let mut options = OxipngOptions::from_preset(2);
    options.strip = StripChunks::Safe;
    oxipng::optimize_from_memory(bytes, &options)
        .ok()
        .filter(|optimized| optimized.len() < bytes.len())
}

pub(super) fn strip_jpeg_metadata_lossless(bytes: &[u8]) -> Option<Vec<u8>> {
    if bytes.len() < 4 || bytes[0..2] != [0xff, 0xd8] {
        return None;
    }

    let mut output = Vec::with_capacity(bytes.len());
    output.extend_from_slice(&bytes[0..2]);
    let mut offset = 2;
    let mut stripped = false;

    while offset < bytes.len() {
        let marker_start = offset;
        if bytes[offset] != 0xff {
            return None;
        }

        while offset < bytes.len() && bytes[offset] == 0xff {
            offset += 1;
        }
        if offset >= bytes.len() {
            return None;
        }

        let marker = bytes[offset];
        offset += 1;

        if marker == 0xd9 {
            output.extend_from_slice(&[0xff, marker]);
            return (stripped && output.len() < bytes.len()).then_some(output);
        }

        if marker == 0xda {
            output.extend_from_slice(&bytes[marker_start..]);
            return (stripped && output.len() < bytes.len()).then_some(output);
        }

        if marker == 0x01 || (0xd0..=0xd7).contains(&marker) {
            output.extend_from_slice(&[0xff, marker]);
            continue;
        }

        if offset + 2 > bytes.len() {
            return None;
        }
        let segment_len = u16::from_be_bytes([bytes[offset], bytes[offset + 1]]) as usize;
        if segment_len < 2 {
            return None;
        }
        let segment_end = offset + segment_len;
        if segment_end > bytes.len() {
            return None;
        }

        if should_strip_jpeg_marker(marker) {
            stripped = true;
        } else {
            output.extend_from_slice(&[0xff, marker]);
            output.extend_from_slice(&bytes[offset..segment_end]);
        }

        offset = segment_end;
    }

    (stripped && output.len() < bytes.len()).then_some(output)
}

fn should_strip_jpeg_marker(marker: u8) -> bool {
    matches!(marker, 0xe0 | 0xe1 | 0xed | 0xfe)
}

fn scaled_dimension(value: u32, percent: u32) -> u32 {
    ((u64::from(value) * u64::from(percent)) / 100)
        .max(1)
        .min(u64::from(u32::MAX)) as u32
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

fn encode_png(image: &DynamicImage) -> Result<Vec<u8>, MediaError> {
    let mut output = Vec::new();
    if image.has_alpha() {
        let rgba = image.to_rgba8();
        PngEncoder::new(&mut output)
            .write_image(
                rgba.as_raw(),
                rgba.width(),
                rgba.height(),
                ExtendedColorType::Rgba8,
            )
            .map_err(|_| MediaError::EncodeImage)?;
    } else {
        let rgb = image.to_rgb8();
        PngEncoder::new(&mut output)
            .write_image(
                rgb.as_raw(),
                rgb.width(),
                rgb.height(),
                ExtendedColorType::Rgb8,
            )
            .map_err(|_| MediaError::EncodeImage)?;
    }
    Ok(optimize_png_lossless(&output).unwrap_or(output))
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
