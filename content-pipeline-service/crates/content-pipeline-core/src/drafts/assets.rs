use serde::Serialize;

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub(super) struct AdaptedAsset {
    #[serde(rename = "type")]
    asset_type: &'static str,
    source_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    alt: Option<String>,
}
