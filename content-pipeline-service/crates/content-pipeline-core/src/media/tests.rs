use super::*;
use image::codecs::jpeg::JpegEncoder;

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
fn rejects_platform_incompatible_requested_preferences() {
    let processor = MediaProcessor::new();

    let err = processor
        .process_data_url(
            ONE_BY_ONE_GIF_DATA_URL,
            &MediaConstraints::for_platform(
                "wechat",
                "inline_image",
                Some(128),
                vec!["image/webp".to_string()],
            ),
        )
        .expect_err("WeChat should not accept WebP just because it was requested");

    assert_eq!(
        err,
        MediaError::UnsupportedMimeType {
            mime_type: "image/webp".to_string(),
            allowed: "image/jpeg, image/png, image/gif".to_string()
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

#[test]
fn exposes_versioned_media_profiles() {
    let profiles = supported_media_profiles()
        .iter()
        .map(|profile| {
            (
                profile.platform,
                profile.profile,
                profile.max_bytes,
                profile.compress_to_max_bytes,
                profile.output_mime_types,
            )
        })
        .collect::<Vec<_>>();

    assert_eq!(
        profiles,
        vec![
            (
                "wechat",
                "wechat@v1",
                WECHAT_MAX_BYTES,
                true,
                &["image/jpeg", "image/png", "image/gif"][..],
            ),
            (
                "douyin",
                "douyin@v1",
                DEFAULT_MAX_BYTES,
                true,
                &["image/jpeg", "image/png", "image/gif"][..],
            ),
            (
                "x",
                "x@v1",
                X_MAX_BYTES,
                true,
                &["image/jpeg", "image/png", "image/gif", "image/webp"][..],
            ),
            (
                "zhihu",
                "zhihu@v1",
                DEFAULT_MAX_BYTES,
                true,
                &["image/jpeg", "image/png", "image/gif"][..],
            ),
            (
                "generic",
                "generic@v1",
                DEFAULT_MAX_BYTES,
                true,
                &["image/jpeg", "image/png", "image/gif", "image/webp"][..],
            ),
        ]
    );
}

#[test]
fn defaults_unknown_media_platform_to_generic_profile() {
    assert_eq!(
        default_max_bytes("mastodon", "inline_image"),
        DEFAULT_MAX_BYTES
    );

    let constraints = MediaConstraints::for_platform("mastodon", "inline_image", None, vec![]);

    assert_eq!(constraints.max_bytes, Some(DEFAULT_MAX_BYTES));
}

#[test]
fn resizes_wechat_images_only_after_quality_floor_cannot_fit() {
    let processor = MediaProcessor::new();
    let max_bytes = 50 * 1024;
    let source = noisy_jpeg(768, 768, 100);
    assert!(
        source.len() as u64 > max_bytes,
        "fixture must start over the output limit"
    );

    let asset = processor
        .process_bytes(
            source,
            Some("image/jpeg"),
            &MediaConstraints::for_platform("wechat", "inline_image", Some(max_bytes), Vec::new()),
        )
        .expect("compressible WeChat image should process");

    assert_eq!(asset.mime_type, "image/jpeg");
    assert!(asset.byte_size <= max_bytes);
    assert!(asset.width < 768);
    assert!(asset.height < 768);
    assert_eq!(
        asset.warnings,
        vec!["image resized to satisfy media constraints"]
    );
}

#[test]
fn compresses_wechat_images_with_profile_default_limit() {
    let processor = MediaProcessor::new();
    let source = noisy_jpeg(1536, 1536, 100);
    assert!(
        source.len() as u64 > WECHAT_MAX_BYTES,
        "fixture must start over the WeChat profile limit"
    );

    let asset = processor
        .process_bytes(
            source,
            Some("image/jpeg"),
            &MediaConstraints::for_platform("wechat", "inline_image", None, Vec::new()),
        )
        .expect("compressible WeChat image should use profile default limit");

    assert_eq!(asset.mime_type, "image/jpeg");
    assert!(asset.byte_size <= WECHAT_MAX_BYTES);
    assert_eq!(asset.width, 1536);
    assert_eq!(asset.height, 1536);
    assert_eq!(
        asset.warnings,
        vec!["image optimized to satisfy media constraints"]
    );
}

#[test]
fn strips_jpeg_metadata_before_lossy_reencoding() {
    let processor = MediaProcessor::new();
    let source_pixels = noisy_jpeg(128, 128, 95);
    let source = jpeg_with_app1_metadata(&source_pixels, 4096);
    let stripped = super::optimizer::strip_jpeg_metadata_lossless(&source)
        .expect("fixture should contain removable metadata");
    assert!(
        source.len() > stripped.len(),
        "fixture should start larger than stripped JPEG"
    );

    let asset = processor
        .process_bytes(
            source,
            Some("image/jpeg"),
            &MediaConstraints::for_platform(
                "wechat",
                "inline_image",
                Some(stripped.len() as u64),
                Vec::new(),
            ),
        )
        .expect("metadata-only overage should be fixed without reencoding");

    assert_eq!(asset.mime_type, "image/jpeg");
    assert_eq!(asset.bytes, stripped);
    assert_eq!(asset.width, 128);
    assert_eq!(asset.height, 128);
    assert_eq!(
        asset.warnings,
        vec!["image optimized to satisfy media constraints"]
    );
}

fn noisy_jpeg(width: u32, height: u32, quality: u8) -> Vec<u8> {
    let mut image = image::RgbImage::new(width, height);
    for y in 0..height {
        for x in 0..width {
            image.put_pixel(
                x,
                y,
                image::Rgb([
                    ((x * 31 + y * 17) % 256) as u8,
                    ((x * 13 + y * 29) % 256) as u8,
                    ((x * 7 + y * 11) % 256) as u8,
                ]),
            );
        }
    }

    let mut bytes = Vec::new();
    JpegEncoder::new_with_quality(&mut bytes, quality)
        .encode_image(&image)
        .expect("test image should encode");
    bytes
}

fn jpeg_with_app1_metadata(jpeg: &[u8], payload_size: usize) -> Vec<u8> {
    assert!(jpeg.len() > 2);
    assert_eq!(&jpeg[0..2], &[0xff, 0xd8]);
    let mut payload = b"Exif\0\0".to_vec();
    payload.resize(payload_size, b'x');
    let segment_len = u16::try_from(payload.len() + 2).expect("segment should fit in JPEG");

    let mut bytes = Vec::with_capacity(jpeg.len() + payload.len() + 4);
    bytes.extend_from_slice(&jpeg[0..2]);
    bytes.extend_from_slice(&[0xff, 0xe1]);
    bytes.extend_from_slice(&segment_len.to_be_bytes());
    bytes.extend_from_slice(&payload);
    bytes.extend_from_slice(&jpeg[2..]);
    bytes
}
