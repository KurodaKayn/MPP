use content_pipeline_core::{DraftCompiler, DraftTarget, SourceProject};

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

fn compile_for(
    platform: &str,
    profile: &str,
    title: &str,
    source_content: &str,
) -> serde_json::Value {
    let output = DraftCompiler::new()
        .compile(
            &SourceProject {
                id: "project-1".to_string(),
                title: title.to_string(),
                source_format: "html".to_string(),
                source_content: source_content.to_string(),
            },
            &DraftTarget {
                platform: platform.to_string(),
                profile: profile.to_string(),
                config_json: "{}".to_string(),
            },
        )
        .expect("draft should compile");

    serde_json::from_str(&output.adapted_content_json).expect("valid adapted content")
}
