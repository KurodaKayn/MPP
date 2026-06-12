use serde::Serialize;

use super::html::{HtmlImageAsset, html_image_assets};

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub(super) struct AdaptedAsset {
    #[serde(rename = "type")]
    asset_type: &'static str,
    source_url: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    alt: Option<String>,
}

pub(super) fn adapted_image_assets(source_content: &str) -> Vec<AdaptedAsset> {
    html_image_assets(source_content)
        .into_iter()
        .map(adapted_image_asset)
        .collect()
}

pub(super) fn optional_assets(assets: &[AdaptedAsset]) -> Option<&[AdaptedAsset]> {
    (!assets.is_empty()).then_some(assets)
}

fn adapted_image_asset(asset: HtmlImageAsset) -> AdaptedAsset {
    AdaptedAsset {
        asset_type: "image",
        source_url: asset.source_url,
        alt: asset.alt,
    }
}
