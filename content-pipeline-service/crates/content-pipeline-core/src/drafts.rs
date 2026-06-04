mod html;
mod text;

use html::{html_to_markdown, html_to_text};
use serde::Serialize;
use text::{
    SHORT_TEXT_MAX_WEIGHT, SHORT_TEXT_WEIGHT_RULES, join_title_and_body_text,
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

        let profile = if target.profile.trim().is_empty() {
            format!("{}@v1", target.platform)
        } else {
            target.profile.trim().to_string()
        };
        let text = html_to_text(&project.source_content);
        let summary = summarize(&text);
        let adapted_content_json = match target.platform.as_str() {
            "wechat" => encode(AdaptedContent {
                schema_version: 1,
                format: "html",
                html: Some(project.source_content.as_str()),
                markdown: None,
                text: None,
                summary: Some(summary.as_str()),
            })?,
            "zhihu" => {
                let markdown = html_to_markdown(&project.source_content);
                encode(AdaptedContent {
                    schema_version: 1,
                    format: "markdown",
                    html: None,
                    markdown: Some(markdown.as_str()),
                    text: None,
                    summary: Some(summary.as_str()),
                })?
            }
            "x" => {
                let text = join_title_and_body_text(&project.title, &text);
                let text = truncate_weighted_text_with_ellipsis(
                    &text,
                    SHORT_TEXT_MAX_WEIGHT,
                    SHORT_TEXT_WEIGHT_RULES,
                );
                let summary = summarize(&text);
                encode(AdaptedContent {
                    schema_version: 1,
                    format: "text",
                    html: None,
                    markdown: None,
                    text: Some(text.as_str()),
                    summary: Some(summary.as_str()),
                })?
            }
            "douyin" => encode(AdaptedContent {
                schema_version: 1,
                format: "text",
                html: None,
                markdown: None,
                text: Some(text.as_str()),
                summary: Some(summary.as_str()),
            })?,
            _ => encode(AdaptedContent {
                schema_version: 1,
                format: "text",
                html: None,
                markdown: None,
                text: Some(text.as_str()),
                summary: Some(summary.as_str()),
            })?,
        };

        Ok(DraftOutput {
            platform: target.platform.clone(),
            profile,
            status: "compiled".to_string(),
            adapted_content_json,
            summary: summarize(&text),
            warnings: Vec::new(),
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
