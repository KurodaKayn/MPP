mod html;
mod text;

use html::{html_to_markdown, html_to_text};
use serde::Serialize;
use text::{
    SHORT_TEXT_MAX_WEIGHT, SHORT_TEXT_WEIGHT_RULES, join_title_and_body_text, text_with_fallback,
    truncate_weighted_text_with_ellipsis,
};
use thiserror::Error;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SourceProject {
    pub id: String,
    pub title: String,
    pub source_format: String,
    pub source_content: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraftTarget {
    pub platform: String,
    pub profile: String,
    pub config_json: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraftOutput {
    pub platform: String,
    pub profile: String,
    pub status: String,
    pub adapted_content_json: String,
    pub summary: String,
    pub warnings: Vec<String>,
}

#[derive(Debug, Error)]
pub enum DraftCompileError {
    #[error("source project is missing content")]
    EmptySource,
    #[error("unsupported source format: {0}")]
    UnsupportedSourceFormat(String),
    #[error("unsupported draft platform: {0}")]
    UnsupportedPlatform(String),
    #[error("unsupported draft profile {profile} for platform {platform}")]
    UnsupportedProfile { platform: String, profile: String },
    #[error("failed to encode adapted content: {0}")]
    Encode(#[from] serde_json::Error),
}

#[derive(Debug, Default, Clone)]
pub struct DraftCompiler;

impl DraftCompiler {
    pub fn new() -> Self {
        Self
    }

    pub fn compile(
        &self,
        project: &SourceProject,
        target: &DraftTarget,
    ) -> Result<DraftOutput, DraftCompileError> {
        if project.source_content.trim().is_empty() {
            return Err(DraftCompileError::EmptySource);
        }

        let source_format = normalize_token(&project.source_format);
        if source_format != "html" {
            return Err(DraftCompileError::UnsupportedSourceFormat(source_format));
        }

        let platform = normalize_token(&target.platform);
        validate_platform(&platform)?;
        let profile = resolve_profile(&platform, &target.profile)?;

        let text = html_to_text(&project.source_content);
        let source_summary = summarize(&text);
        let (adapted_content_json, summary, warnings) = match platform.as_str() {
            "wechat" => encode(AdaptedContent {
                schema_version: 1,
                format: "html",
                html: Some(project.source_content.as_str()),
                markdown: None,
                text: None,
                summary: Some(source_summary.as_str()),
            })
            .map(|value| (value, source_summary.clone(), Vec::new()))?,
            "zhihu" => {
                let markdown = html_to_markdown(&project.source_content);
                encode(AdaptedContent {
                    schema_version: 1,
                    format: "markdown",
                    html: None,
                    markdown: Some(markdown.as_str()),
                    text: None,
                    summary: Some(source_summary.as_str()),
                })
                .map(|value| (value, source_summary.clone(), Vec::new()))?
            }
            "x" => {
                let text = join_title_and_body_text(&project.title, &text);
                let truncated_text = truncate_weighted_text_with_ellipsis(
                    &text,
                    SHORT_TEXT_MAX_WEIGHT,
                    SHORT_TEXT_WEIGHT_RULES,
                );
                let summary = summarize(&truncated_text);
                let mut warnings = Vec::new();
                if truncated_text != text {
                    warnings.push(format!(
                        "text truncated to satisfy {profile} weighted length limit"
                    ));
                }
                let adapted_content_json = encode(AdaptedContent {
                    schema_version: 1,
                    format: "text",
                    html: None,
                    markdown: None,
                    text: Some(truncated_text.as_str()),
                    summary: Some(summary.as_str()),
                })?;
                (adapted_content_json, summary, warnings)
            }
            "douyin" => {
                let text = text_with_fallback(&text, &project.title, &project.source_content);
                let summary = summarize(text);
                let adapted_content_json = encode(AdaptedContent {
                    schema_version: 1,
                    format: "text",
                    html: None,
                    markdown: None,
                    text: Some(text),
                    summary: Some(summary.as_str()),
                })?;
                (adapted_content_json, summary, Vec::new())
            }
            _ => unreachable!("draft platform was validated before compilation"),
        };

        Ok(DraftOutput {
            platform,
            profile,
            status: "compiled".to_string(),
            adapted_content_json,
            summary,
            warnings,
        })
    }
}

#[derive(Serialize)]
struct AdaptedContent<'a> {
    schema_version: u32,
    format: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    html: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    markdown: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    text: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    summary: Option<&'a str>,
}

fn encode(value: AdaptedContent<'_>) -> Result<String, serde_json::Error> {
    serde_json::to_string(&value)
}

fn summarize(value: &str) -> String {
    const MAX_SUMMARY_CHARS: usize = 80;
    value.chars().take(MAX_SUMMARY_CHARS).collect()
}

fn normalize_token(value: &str) -> String {
    value.trim().to_ascii_lowercase()
}

fn validate_platform(platform: &str) -> Result<(), DraftCompileError> {
    match platform {
        "wechat" | "zhihu" | "x" | "douyin" => Ok(()),
        _ => Err(DraftCompileError::UnsupportedPlatform(platform.to_string())),
    }
}

fn resolve_profile(platform: &str, requested: &str) -> Result<String, DraftCompileError> {
    let default_profile = format!("{platform}@v1");
    let profile = normalize_token(requested);
    if profile.is_empty() || profile == default_profile {
        return Ok(default_profile);
    }

    Err(DraftCompileError::UnsupportedProfile {
        platform: platform.to_string(),
        profile,
    })
}
