use super::{DEFAULT_MAX_BYTES, WECHAT_MAX_BYTES};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MediaProfile {
    pub platform: &'static str,
    pub profile: &'static str,
    pub max_bytes: u64,
    pub compress_to_max_bytes: bool,
}

const GENERIC_MEDIA_PROFILE: MediaProfile = MediaProfile {
    platform: "generic",
    profile: "generic@v1",
    max_bytes: DEFAULT_MAX_BYTES,
    compress_to_max_bytes: false,
};

const SUPPORTED_MEDIA_PROFILES: &[MediaProfile] = &[
    MediaProfile {
        platform: "wechat",
        profile: "wechat@v1",
        max_bytes: WECHAT_MAX_BYTES,
        compress_to_max_bytes: true,
    },
    MediaProfile {
        platform: "douyin",
        profile: "douyin@v1",
        max_bytes: DEFAULT_MAX_BYTES,
        compress_to_max_bytes: false,
    },
    GENERIC_MEDIA_PROFILE,
];

pub fn supported_media_profiles() -> &'static [MediaProfile] {
    SUPPORTED_MEDIA_PROFILES
}

pub(super) fn resolve_media_profile(platform: &str, _usage: &str) -> &'static MediaProfile {
    let platform = normalize_token(platform);
    if platform.is_empty() {
        return &GENERIC_MEDIA_PROFILE;
    }

    SUPPORTED_MEDIA_PROFILES
        .iter()
        .find(|profile| profile.platform == platform)
        .unwrap_or(&GENERIC_MEDIA_PROFILE)
}

fn normalize_token(value: &str) -> String {
    value.trim().to_ascii_lowercase()
}
