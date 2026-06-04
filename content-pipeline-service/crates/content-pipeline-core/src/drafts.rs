use serde::Serialize;
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
        let text = plain_text(&project.source_content);
        let adapted_content_json = match target.platform.as_str() {
            "wechat" => encode(AdaptedContent {
                schema_version: 1,
                format: "html",
                html: Some(project.source_content.as_str()),
                text: None,
            })?,
            "x" | "douyin" => encode(AdaptedContent {
                schema_version: 1,
                format: "text",
                html: None,
                text: Some(text.as_str()),
            })?,
            _ => encode(AdaptedContent {
                schema_version: 1,
                format: "text",
                html: None,
                text: Some(text.as_str()),
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
    text: Option<&'a str>,
}

fn encode(value: AdaptedContent<'_>) -> Result<String, serde_json::Error> {
    serde_json::to_string(&value)
}

fn plain_text(value: &str) -> String {
    let mut output = String::with_capacity(value.len());
    let mut in_tag = false;

    for ch in value.chars() {
        match ch {
            '<' => in_tag = true,
            '>' => in_tag = false,
            _ if !in_tag => output.push(ch),
            _ => {}
        }
    }

    output.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn summarize(value: &str) -> String {
    const MAX_SUMMARY_CHARS: usize = 80;
    value.chars().take(MAX_SUMMARY_CHARS).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn compiles_x_text_draft() {
        let compiler = DraftCompiler::new();
        let output = compiler
            .compile(
                &SourceProject {
                    id: "project-1".to_string(),
                    title: "Hello".to_string(),
                    source_format: "html".to_string(),
                    source_content: "<h1>Hello</h1><p>World</p>".to_string(),
                },
                &DraftTarget {
                    platform: "x".to_string(),
                    profile: "x@v1".to_string(),
                    config_json: "{}".to_string(),
                },
            )
            .expect("draft should compile");

        assert_eq!(output.platform, "x");
        assert_eq!(output.status, "compiled");
        assert!(output.adapted_content_json.contains("\"format\":\"text\""));
        assert!(output.adapted_content_json.contains("HelloWorld"));
    }
}
