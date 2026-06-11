use super::{DEFAULT_MAX_BYTES, WECHAT_MAX_BYTES, X_MAX_BYTES};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MediaProfile {
    pub platform: &'static str,
    pub profile: &'static str,
    pub max_bytes: u64,
    pub compress_to_max_bytes: bool,
    pub output_mime_types: &'static [&'static str],
}

const GENERIC_MEDIA_PROFILE_V1: MediaProfile = MediaProfile {
    platform: "generic",
    profile: "generic@v1",
    max_bytes: DEFAULT_MAX_BYTES,
    compress_to_max_bytes: true,
    output_mime_types: &["image/jpeg", "image/png", "image/gif", "image/webp"],
};

const SUPPORTED_MEDIA_PROFILES: &[MediaProfile] = &[
    MediaProfile {
        platform: "wechat",
        profile: "wechat@v1",
        max_bytes: WECHAT_MAX_BYTES,
        compress_to_max_bytes: true,
        output_mime_types: &["image/jpeg", "image/png", "image/gif"],
    },
    MediaProfile {
        platform: "douyin",
        profile: "douyin@v1",
        max_bytes: DEFAULT_MAX_BYTES,
        compress_to_max_bytes: true,
        output_mime_types: &["image/jpeg", "image/png", "image/gif"],
    },
    MediaProfile {
        platform: "x",
        profile: "x@v1",
        max_bytes: X_MAX_BYTES,
        compress_to_max_bytes: true,
        output_mime_types: &["image/jpeg", "image/png", "image/gif", "image/webp"],
    },
    MediaProfile {
        platform: "zhihu",
        profile: "zhihu@v1",
        max_bytes: DEFAULT_MAX_BYTES,
        compress_to_max_bytes: true,
        output_mime_types: &["image/jpeg", "image/png", "image/gif"],
    },
    GENERIC_MEDIA_PROFILE_V1,
];

pub fn supported_media_profiles() -> &'static [MediaProfile] {
    SUPPORTED_MEDIA_PROFILES
}

pub(super) fn resolve_media_profile(platform: &str, _usage: &str) -> &'static MediaProfile {
    let platform = normalize_token(platform);
    if platform.is_empty() {
        return &GENERIC_MEDIA_PROFILE_V1;
    }

    if let Some(profile) = SUPPORTED_MEDIA_PROFILES
        .iter()
        .find(|profile| profile.profile == platform)
    {
        return profile;
    }

    SUPPORTED_MEDIA_PROFILES
        .iter()
        .rev()
        .find(|profile| profile.platform == platform)
        .unwrap_or(&GENERIC_MEDIA_PROFILE_V1)
}

fn normalize_token(value: &str) -> String {
    value.trim().to_ascii_lowercase()
}
