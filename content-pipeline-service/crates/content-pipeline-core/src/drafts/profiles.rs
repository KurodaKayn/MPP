#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DraftFormat {
    Html,
    Markdown,
    Text,
}

impl DraftFormat {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Html => "html",
            Self::Markdown => "markdown",
            Self::Text => "text",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct DraftProfile {
    pub platform: &'static str,
    pub profile: &'static str,
    pub schema_version: u32,
    pub format: DraftFormat,
}

const SUPPORTED_DRAFT_PROFILES: &[DraftProfile] = &[
    DraftProfile {
        platform: "wechat",
        profile: "wechat@v1",
        schema_version: 1,
        format: DraftFormat::Html,
    },
    DraftProfile {
        platform: "zhihu",
        profile: "zhihu@v1",
        schema_version: 1,
        format: DraftFormat::Markdown,
    },
    DraftProfile {
        platform: "x",
        profile: "x@v1",
        schema_version: 1,
        format: DraftFormat::Text,
    },
    DraftProfile {
        platform: "douyin",
        profile: "douyin@v1",
        schema_version: 1,
        format: DraftFormat::Text,
    },
];

pub fn supported_draft_profiles() -> &'static [DraftProfile] {
    SUPPORTED_DRAFT_PROFILES
}

pub(super) fn resolve_draft_profile(
    platform: &str,
    requested_profile: &str,
) -> Option<&'static DraftProfile> {
    let requested_profile = requested_profile.trim();
    if requested_profile.is_empty() {
        return default_profile(platform);
    }

    SUPPORTED_DRAFT_PROFILES
        .iter()
        .find(|profile| profile.platform == platform && profile.profile == requested_profile)
}

pub(super) fn supports_platform(platform: &str) -> bool {
    SUPPORTED_DRAFT_PROFILES
        .iter()
        .any(|profile| profile.platform == platform)
}

pub(super) fn default_profile_name(platform: &str) -> Option<String> {
    default_profile(platform).map(|profile| profile.profile.to_string())
}

fn default_profile(platform: &str) -> Option<&'static DraftProfile> {
    SUPPORTED_DRAFT_PROFILES
        .iter()
        .find(|profile| profile.platform == platform)
}
