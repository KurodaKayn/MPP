use content_pipeline_core::{
    DraftCompileError, DraftCompiler, DraftOutput, DraftTarget, SourceProject,
    supported_draft_profiles,
};
use serde_json::json;
use std::collections::BTreeSet;
use std::path::{Path, PathBuf};

#[test]
fn compiles_text_draft_with_title() {
    let output = compile_for("x", "x@v1", "Hello", "<h1>Hello</h1><p>World</p>");

    assert_eq!(output["format"], "text");
    assert!(
        output["text"]
            .as_str()
            .expect("text output")
            .contains("Hello")
    );
    assert!(
        output["text"]
            .as_str()
            .expect("text output")
            .contains("World")
    );
}

#[test]
fn keeps_long_urls_with_weighted_text_rules() {
    let url = format!("https://example.com/{}", "a".repeat(320));
    let output = compile_for("x", "x@v1", "", &format!("<p>{url}</p>"));
    let text = output["text"].as_str().expect("text output");

    assert!(text.contains(&url));
    assert!(!text.ends_with("..."));
}

#[test]
fn truncates_weighted_text_with_ellipsis() {
    let output = compile_for("x", "x@v1", "", &format!("<p>{}</p>", "中".repeat(200)));
    let text = output["text"].as_str().expect("text output");

    assert!(text.ends_with("..."));
}

#[test]
fn reports_truncation_warning_and_matching_summary() {
    let output = compile_output(
        "x",
        "x@v1",
        "",
        "html",
        &format!("<p>{}</p>", "中".repeat(200)),
    )
    .expect("x draft should compile");
    let adapted_content = decode_adapted_content(&output);

    assert_eq!(
        output.warnings,
        vec!["text truncated to satisfy x@v1 weighted length limit"]
    );
    assert_eq!(output.summary, adapted_content["summary"]);
}

#[test]
fn rejects_unsupported_source_format() {
    let err = compile_output("x", "x@v1", "Hello", "markdown", "# Hello")
        .expect_err("markdown input is not supported yet");

    assert!(matches!(
        err,
        DraftCompileError::UnsupportedSourceFormat(format) if format == "markdown"
    ));
}

#[test]
fn rejects_unsupported_platform() {
    let err = compile_output("mastodon", "mastodon@v1", "", "html", "<p>Hello</p>")
        .expect_err("unknown platform should not compile");

    assert!(matches!(
        err,
        DraftCompileError::UnsupportedPlatform(platform) if platform == "mastodon"
    ));
}

#[test]
fn rejects_unsupported_profile_version() {
    let err = compile_output("x", "x@v2", "", "html", "<p>Hello</p>")
        .expect_err("unsupported profile should not compile");

    assert!(matches!(
        err,
        DraftCompileError::UnsupportedProfile { platform, profile }
            if platform == "x" && profile == "x@v2"
    ));
}

#[test]
fn defaults_to_registered_profile_version() {
    let output = compile_output("wechat", "", "Hello", "html", "<p>Hello</p>")
        .expect("blank profile should use platform default");

    assert_eq!(output.profile, "wechat@v1");
}

#[test]
fn falls_back_to_title_for_text_draft_without_body_text() {
    let output = compile_for(
        "douyin",
        "douyin@v1",
        "Image draft",
        r#"<img src="https://example.com/a.png" alt="Image">"#,
    );

    assert_eq!(output["format"], "text");
    assert_eq!(output["text"], "Image draft");
}

#[test]
fn falls_back_to_source_for_text_draft_without_title_or_body_text() {
    let source = r#"<img src="https://example.com/a.png" alt="Image">"#;
    let output = compile_for("douyin", "douyin@v1", "", source);

    assert_eq!(output["format"], "text");
    assert_eq!(output["text"], source);
}

#[test]
fn compiles_html_to_markdown_draft() {
    let output = compile_for(
        "zhihu",
        "zhihu@v1",
        "Article",
        r#"
            <h2>Subtitle</h2>
            <p>This is a <strong>bold</strong> body.</p>
            <blockquote>Quote</blockquote>
            <ul><li>First point</li></ul>
            <p><img src="https://example.com/a.png" alt="Image"></p>
        "#,
    );

    let markdown = output["markdown"].as_str().expect("markdown output");
    assert_eq!(output["format"], "markdown");
    assert!(markdown.contains("## Subtitle"));
    assert!(markdown.contains("**bold**"));
    assert!(markdown.contains("> Quote"));
    assert!(markdown.contains("- First point"));
    assert!(markdown.contains("![Image](https://example.com/a.png)"));
}

#[test]
fn matches_wechat_representative_snapshot() {
    assert_representative_snapshot("wechat");
}

#[test]
fn matches_zhihu_representative_snapshot() {
    assert_representative_snapshot("zhihu");
}

#[test]
fn matches_x_representative_snapshot() {
    assert_representative_snapshot("x");
}

#[test]
fn matches_douyin_representative_snapshot() {
    assert_representative_snapshot("douyin");
}

#[test]
fn supported_profiles_match_representative_snapshots() {
    let snapshot_platforms = snapshot_platforms();
    let supported_platforms = supported_draft_profiles()
        .iter()
        .map(|profile| profile.platform.to_string())
        .collect::<BTreeSet<_>>();
    assert_eq!(supported_platforms, snapshot_platforms);

    for profile in supported_draft_profiles() {
        let snapshot = fixture_json(&format!("draft_snapshots/{}.json", profile.platform));
        assert_eq!(snapshot["platform"], profile.platform);
        assert_eq!(snapshot["profile"], profile.profile);
        assert_eq!(
            snapshot["adapted_content"]["schema_version"],
            profile.schema_version
        );
        assert_eq!(
            snapshot["adapted_content"]["format"],
            profile.format.as_str()
        );
    }
}

fn compile_for(
    platform: &str,
    profile: &str,
    title: &str,
    source_content: &str,
) -> serde_json::Value {
    let output = compile_output(platform, profile, title, "html", source_content)
        .expect("draft should compile");

    decode_adapted_content(&output)
}

fn compile_output(
    platform: &str,
    profile: &str,
    title: &str,
    source_format: &str,
    source_content: &str,
) -> Result<DraftOutput, DraftCompileError> {
    DraftCompiler::new().compile(
        &SourceProject {
            id: "project-1".to_string(),
            title: title.to_string(),
            source_format: source_format.to_string(),
            source_content: source_content.to_string(),
        },
        &DraftTarget {
            platform: platform.to_string(),
            profile: profile.to_string(),
            config_json: "{}".to_string(),
        },
    )
}

fn decode_adapted_content(output: &DraftOutput) -> serde_json::Value {
    serde_json::from_str(&output.adapted_content_json).expect("valid adapted content")
}

fn assert_representative_snapshot(platform: &str) {
    let source_content = fixture_text("representative_article.html");
    let output = compile_output(
        platform,
        &format!("{platform}@v1"),
        "Launch Notes",
        "html",
        &source_content,
    )
    .expect("representative draft should compile");

    let expected = fixture_json(&format!("draft_snapshots/{platform}.json"));
    assert_eq!(draft_snapshot(&output), expected);
}

fn draft_snapshot(output: &DraftOutput) -> serde_json::Value {
    json!({
        "platform": output.platform,
        "profile": output.profile,
        "status": output.status,
        "adapted_content": decode_adapted_content(output),
        "summary": output.summary,
        "warnings": output.warnings,
    })
}

fn fixture_text(name: &str) -> String {
    std::fs::read_to_string(fixture_path(name))
        .expect("fixture should be readable")
        .trim_end()
        .to_string()
}

fn fixture_json(name: &str) -> serde_json::Value {
    serde_json::from_str(&fixture_text(name)).expect("fixture should contain valid JSON")
}

fn fixture_path(name: &str) -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("tests")
        .join("fixtures")
        .join(name)
}

fn snapshot_platforms() -> BTreeSet<String> {
    std::fs::read_dir(fixture_path("draft_snapshots"))
        .expect("snapshot fixture dir should be readable")
        .map(|entry| {
            entry
                .expect("snapshot fixture entry should be readable")
                .path()
                .file_stem()
                .expect("snapshot fixture should have a file stem")
                .to_string_lossy()
                .to_string()
        })
        .collect()
}
