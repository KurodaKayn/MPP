use image::codecs::png::PngEncoder;
use image::{DynamicImage, ExtendedColorType, ImageEncoder};
use oxipng::{Options as OxipngOptions, StripChunks};

use crate::media::MediaError;

pub(super) fn optimize_png(bytes: &[u8]) -> Option<Vec<u8>> {
    let mut options = OxipngOptions::from_preset(2);
    options.strip = StripChunks::Safe;
    oxipng::optimize_from_memory(bytes, &options)
        .ok()
        .filter(|optimized| optimized.len() < bytes.len())
}

pub(in crate::media) fn strip_jpeg_metadata(bytes: &[u8]) -> Option<Vec<u8>> {
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

        if should_strip_jpeg_segment(marker, &bytes[offset..segment_end]) {
            stripped = true;
        } else {
            output.extend_from_slice(&[0xff, marker]);
            output.extend_from_slice(&bytes[offset..segment_end]);
        }

        offset = segment_end;
    }

    (stripped && output.len() < bytes.len()).then_some(output)
}

fn should_strip_jpeg_segment(marker: u8, segment: &[u8]) -> bool {
    match marker {
        0xe0 | 0xed | 0xfe => true,
        0xe1 => !is_exif_segment(segment),
        _ => false,
    }
}

fn is_exif_segment(segment: &[u8]) -> bool {
    segment
        .get(2..)
        .is_some_and(|payload| payload.starts_with(b"Exif\0\0"))
}

pub(super) fn encode_png(image: &DynamicImage) -> Result<Vec<u8>, MediaError> {
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
    Ok(optimize_png(&output).unwrap_or(output))
}
