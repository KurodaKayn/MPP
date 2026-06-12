use serde::Serialize;
use serde_json::Value;

use super::assets::AdaptedAsset;
use super::{DraftCompileError, DraftFormat, DraftProfile};

#[derive(Serialize)]
pub(super) struct AdaptedContent<'a> {
    pub(super) schema_version: u32,
    pub(super) format: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(super) html: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(super) markdown: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(super) text: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(super) summary: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(super) assets: Option<&'a [AdaptedAsset]>,
}

pub(super) fn encode_validated(
    profile: &DraftProfile,
    value: AdaptedContent<'_>,
) -> Result<String, DraftCompileError> {
    let encoded = serde_json::to_string(&value)?;
    validate_adapted_content_json(profile, &encoded)?;
    Ok(encoded)
}

fn validate_adapted_content_json(
    profile: &DraftProfile,
    encoded: &str,
) -> Result<(), DraftCompileError> {
    let value: Value = serde_json::from_str(encoded)?;
    let Some(object) = value.as_object() else {
        return Err(schema_validation_error(
            profile,
            "adapted content must be an object",
        ));
    };

    if object
        .get("schema_version")
        .and_then(Value::as_u64)
        .is_none_or(|version| version != u64::from(profile.schema_version))
    {
        return Err(schema_validation_error(
            profile,
            "schema_version must match the draft profile",
        ));
    }

    if object.get("format").and_then(Value::as_str) != Some(profile.format.as_str()) {
        return Err(schema_validation_error(
            profile,
            "format must match the draft profile",
        ));
    }

    let required_field = match profile.format {
        DraftFormat::Html => "html",
        DraftFormat::Markdown => "markdown",
        DraftFormat::Text => "text",
    };
    if object
        .get(required_field)
        .and_then(Value::as_str)
        .is_none_or(|value| value.trim().is_empty())
    {
        return Err(schema_validation_error(
            profile,
            format!("{required_field} content is required"),
        ));
    }

    validate_optional_string(profile, object.get("summary"), "summary")?;
    validate_assets(profile, object.get("assets"))?;

    Ok(())
}

fn validate_optional_string(
    profile: &DraftProfile,
    value: Option<&Value>,
    field: &'static str,
) -> Result<(), DraftCompileError> {
    if let Some(value) = value
        && !value.is_string()
    {
        return Err(schema_validation_error(
            profile,
            format!("{field} must be a string"),
        ));
    }

    Ok(())
}

fn validate_assets(profile: &DraftProfile, value: Option<&Value>) -> Result<(), DraftCompileError> {
    let Some(value) = value else {
        return Ok(());
    };
    let Some(assets) = value.as_array() else {
        return Err(schema_validation_error(profile, "assets must be an array"));
    };

    for asset in assets {
        let Some(asset) = asset.as_object() else {
            return Err(schema_validation_error(profile, "asset must be an object"));
        };
        if asset.get("type").and_then(Value::as_str) != Some("image") {
            return Err(schema_validation_error(profile, "asset type must be image"));
        }
        if asset
            .get("source_url")
            .and_then(Value::as_str)
            .is_none_or(|value| value.trim().is_empty())
        {
            return Err(schema_validation_error(
                profile,
                "asset source_url is required",
            ));
        }
        validate_optional_string(profile, asset.get("alt"), "asset alt")?;
    }

    Ok(())
}

fn schema_validation_error(profile: &DraftProfile, reason: impl Into<String>) -> DraftCompileError {
    DraftCompileError::SchemaValidation {
        platform: profile.platform.to_string(),
        profile: profile.profile.to_string(),
        reason: reason.into(),
    }
}

#[cfg(test)]
mod tests {
    use super::super::profiles;
    use super::*;

    #[test]
    fn validates_schema_matching_profile() {
        validate_adapted_content_json(
            profile("wechat"),
            r#"{"schema_version":1,"format":"html","html":"<p>Hello</p>","assets":[{"type":"image","source_url":"mpp://media/11111111-1111-1111-1111-111111111111","alt":"Hero"}]}"#,
        )
        .expect("valid adapted content should pass schema validation");
    }

    #[test]
    fn rejects_schema_format_mismatch() {
        let err = validate_adapted_content_json(
            profile("zhihu"),
            r#"{"schema_version":1,"format":"html","html":"<p>Hello</p>"}"#,
        )
        .expect_err("format mismatch should fail");

        assert_schema_validation_reason(err, "format must match the draft profile");
    }

    #[test]
    fn rejects_schema_missing_primary_content() {
        let err = validate_adapted_content_json(
            profile("x"),
            r#"{"schema_version":1,"format":"text","summary":"Only summary"}"#,
        )
        .expect_err("missing text should fail");

        assert_schema_validation_reason(err, "text content is required");
    }

    #[test]
    fn rejects_schema_invalid_asset() {
        let err = validate_adapted_content_json(
            profile("douyin"),
            r#"{"schema_version":1,"format":"text","text":"Hello","assets":[{"type":"image","source_url":""}]}"#,
        )
        .expect_err("empty source_url should fail");

        assert_schema_validation_reason(err, "asset source_url is required");
    }

    fn profile(platform: &str) -> &'static DraftProfile {
        profiles::resolve_draft_profile(platform, &format!("{platform}@v1"))
            .expect("test profile should exist")
    }

    fn assert_schema_validation_reason(err: DraftCompileError, expected: &str) {
        match err {
            DraftCompileError::SchemaValidation { reason, .. } => assert_eq!(reason, expected),
            other => panic!("expected schema validation error, got {other:?}"),
        }
    }
}
