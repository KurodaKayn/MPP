use std::collections::BTreeSet;
use std::fmt::Write;
use std::ops::Deref;

use ego_tree::NodeRef;
use scraper::{Html, Node};

pub(super) fn html_to_text(value: &str) -> String {
    let fragment = Html::parse_fragment(value);
    let mut renderer = TextRenderer::default();
    for child in fragment.tree.root().children() {
        renderer.render(child);
    }
    renderer.finish()
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) struct HtmlImageAsset {
    pub source_url: String,
    pub alt: Option<String>,
}

pub(super) fn html_image_assets(value: &str) -> Vec<HtmlImageAsset> {
    let fragment = Html::parse_fragment(value);
    let mut assets = Vec::new();
    for child in fragment.tree.root().children() {
        collect_image_assets(child, &mut assets);
    }
    assets
}

pub(super) fn html_lossy_warnings(value: &str) -> Vec<String> {
    let fragment = Html::parse_fragment(value);
    let mut warnings = BTreeSet::new();
    for child in fragment.tree.root().children() {
        collect_lossy_warnings(child, &mut warnings);
    }
    warnings.into_iter().collect()
}

#[derive(Default)]
struct TextRenderer {
    output: String,
}

impl TextRenderer {
    fn render(&mut self, node: NodeRef<'_, Node>) {
        match node.value() {
            Node::Text(text) => self.write_text(text.deref()),
            Node::Element(element) => {
                if is_non_content_element(element.name()) {
                    return;
                }
                if element.name() == "br" {
                    self.output.push('\n');
                }
                for child in node.children() {
                    self.render(child);
                }
                if is_block_element(element.name()) && !self.output.is_empty() {
                    self.output.push('\n');
                }
            }
            _ => {
                for child in node.children() {
                    self.render(child);
                }
            }
        }
    }

    fn write_text(&mut self, value: &str) {
        let collapsed = collapse_inline_whitespace(value);
        if collapsed.is_empty() {
            return;
        }
        if value.starts_with(char::is_whitespace)
            && !self.output.is_empty()
            && !self.output.ends_with([' ', '\n'])
        {
            self.output.push(' ');
        }
        self.output.push_str(&collapsed);
        if value.ends_with(char::is_whitespace) {
            self.output.push(' ');
        }
    }

    fn finish(self) -> String {
        self.output
            .lines()
            .filter_map(|line| {
                let line = collapse_inline_whitespace(line);
                if line.is_empty() { None } else { Some(line) }
            })
            .collect::<Vec<_>>()
            .join("\n")
    }
}

pub(super) fn html_to_markdown(value: &str) -> String {
    let fragment = Html::parse_fragment(value);
    let mut renderer = MarkdownRenderer::default();
    for child in fragment.tree.root().children() {
        renderer.render(child);
    }
    renderer.finish()
}

#[derive(Default)]
struct MarkdownRenderer {
    output: String,
}

impl MarkdownRenderer {
    fn render(&mut self, node: NodeRef<'_, Node>) {
        match node.value() {
            Node::Text(text) => self.write_text(text.deref()),
            Node::Element(element) => self.render_element(node, element.name()),
            _ => self.render_children(node),
        }
    }

    fn write_text(&mut self, value: &str) {
        let collapsed = collapse_inline_whitespace(value);
        if collapsed.is_empty() {
            return;
        }
        if value.starts_with(char::is_whitespace)
            && !self.output.is_empty()
            && !self.output.ends_with([' ', '\n'])
        {
            self.output.push(' ');
        }
        self.output.push_str(&collapsed);
        if value.ends_with(char::is_whitespace) {
            self.output.push(' ');
        }
    }

    fn render_element(&mut self, node: NodeRef<'_, Node>, name: &str) {
        if is_non_content_element(name) {
            return;
        }

        match name {
            "h1" | "h2" | "h3" | "h4" | "h5" | "h6" => {
                self.ensure_blank_line();
                let level = name[1..].parse::<usize>().unwrap_or(1);
                self.output.push_str(&"#".repeat(level));
                self.output.push(' ');
                self.render_children(node);
                self.ensure_blank_line();
            }
            "p" => {
                self.ensure_blank_line();
                self.render_children(node);
                self.ensure_blank_line();
            }
            "strong" | "b" => {
                self.output.push_str("**");
                self.render_children(node);
                self.output.push_str("**");
            }
            "em" | "i" => {
                self.output.push('*');
                self.render_children(node);
                self.output.push('*');
            }
            "a" => {
                let label = node_text(node);
                let href = attr_value(node, "href");
                if !label.is_empty() && !href.is_empty() {
                    self.output.push('[');
                    self.output.push_str(&label);
                    self.output.push_str("](");
                    self.output.push_str(href);
                    self.output.push(')');
                    return;
                }
                self.render_children(node);
            }
            "img" => {
                let src = attr_value(node, "src");
                if src.is_empty() {
                    return;
                }
                self.ensure_blank_line();
                self.output.push_str("![");
                self.output.push_str(attr_value(node, "alt"));
                self.output.push_str("](");
                self.output.push_str(src);
                self.output.push(')');
                self.ensure_blank_line();
            }
            "blockquote" => {
                self.ensure_blank_line();
                for line in node_text(node).lines() {
                    let line = line.trim();
                    if !line.is_empty() {
                        self.output.push_str("> ");
                        self.output.push_str(line);
                        self.output.push('\n');
                    }
                }
                self.ensure_blank_line();
            }
            "ul" => {
                self.ensure_blank_line();
                self.render_list_items(node, "-");
                self.ensure_blank_line();
            }
            "ol" => {
                self.ensure_blank_line();
                let mut index = 1;
                for child in node.children() {
                    if element_name(child) == Some("li") {
                        write!(self.output, "{index}. ")
                            .expect("writing to String should not fail");
                        self.render_children(child);
                        self.output.push('\n');
                        index += 1;
                    }
                }
                self.ensure_blank_line();
            }
            "li" => {
                self.output.push_str("- ");
                self.render_children(node);
                self.output.push('\n');
            }
            "code" => {
                self.output.push('`');
                self.render_children(node);
                self.output.push('`');
            }
            "pre" => {
                self.ensure_blank_line();
                self.output.push_str("```\n");
                self.output
                    .push_str(trim_outer_newlines(&preformatted_text(node)));
                self.output.push_str("\n```");
                self.ensure_blank_line();
            }
            "br" => self.output.push('\n'),
            _ => self.render_children(node),
        }
    }

    fn render_children(&mut self, node: NodeRef<'_, Node>) {
        for child in node.children() {
            self.render(child);
        }
    }

    fn render_list_items(&mut self, node: NodeRef<'_, Node>, marker: &str) {
        for child in node.children() {
            if element_name(child) == Some("li") {
                self.output.push_str(marker);
                self.output.push(' ');
                self.render_children(child);
                self.output.push('\n');
            }
        }
    }

    fn ensure_blank_line(&mut self) {
        if self.output.is_empty() || self.output.ends_with("\n\n") {
            return;
        }
        if self.output.ends_with('\n') {
            self.output.push('\n');
            return;
        }
        self.output.push_str("\n\n");
    }

    fn finish(self) -> String {
        self.output.trim().to_string()
    }
}

fn node_text(node: NodeRef<'_, Node>) -> String {
    let mut output = String::new();
    collect_text(node, &mut output);
    collapse_inline_whitespace(&output)
}

fn preformatted_text(node: NodeRef<'_, Node>) -> String {
    let mut output = String::new();
    collect_preformatted_text(node, &mut output);
    output
}

fn collect_text(node: NodeRef<'_, Node>, output: &mut String) {
    match node.value() {
        Node::Text(text) => output.push_str(text.deref()),
        Node::Element(element) if element.name() == "br" => output.push('\n'),
        _ => {
            for child in node.children() {
                collect_text(child, output);
            }
        }
    }
}

fn collect_preformatted_text(node: NodeRef<'_, Node>, output: &mut String) {
    match node.value() {
        Node::Text(text) => output.push_str(text.deref()),
        _ => {
            for child in node.children() {
                collect_preformatted_text(child, output);
            }
        }
    }
}

fn attr_value<'a>(node: NodeRef<'a, Node>, name: &str) -> &'a str {
    match node.value() {
        Node::Element(element) => element.attr(name).unwrap_or(""),
        _ => "",
    }
}

fn collect_image_assets(node: NodeRef<'_, Node>, assets: &mut Vec<HtmlImageAsset>) {
    if element_name(node) == Some("img") {
        let source_url = attr_value(node, "src").trim();
        if !source_url.is_empty() {
            let alt = attr_value(node, "alt").trim();
            assets.push(HtmlImageAsset {
                source_url: source_url.to_string(),
                alt: (!alt.is_empty()).then(|| alt.to_string()),
            });
        }
    }

    for child in node.children() {
        collect_image_assets(child, assets);
    }
}

fn collect_lossy_warnings(node: NodeRef<'_, Node>, warnings: &mut BTreeSet<String>) {
    if let Node::Element(element) = node.value() {
        let name = element.name();
        if is_unsupported_element(name) {
            warnings.insert(format!(
                "unsupported HTML element <{name}> may be dropped or sanitized"
            ));
        }
        for (attr_name, attr_value) in element.attrs() {
            if attr_name.to_ascii_lowercase().starts_with("on") {
                warnings.insert(format!(
                    "event handler attribute {attr_name} may be dropped or sanitized"
                ));
            }
            if is_url_attribute(attr_name) && has_unsafe_url_scheme(attr_value) {
                warnings.insert(format!(
                    "unsafe URL in {attr_name} attribute may be dropped or sanitized"
                ));
            }
        }
    }

    for child in node.children() {
        collect_lossy_warnings(child, warnings);
    }
}

fn element_name(node: NodeRef<'_, Node>) -> Option<&str> {
    match node.value() {
        Node::Element(element) => Some(element.name()),
        _ => None,
    }
}

fn is_block_element(name: &str) -> bool {
    matches!(
        name,
        "article"
            | "blockquote"
            | "div"
            | "figcaption"
            | "figure"
            | "h1"
            | "h2"
            | "h3"
            | "h4"
            | "h5"
            | "h6"
            | "li"
            | "p"
            | "section"
    )
}

fn is_non_content_element(name: &str) -> bool {
    matches!(name, "script" | "style" | "template" | "noscript")
}

fn is_unsupported_element(name: &str) -> bool {
    matches!(
        name,
        "audio"
            | "button"
            | "canvas"
            | "embed"
            | "form"
            | "iframe"
            | "input"
            | "link"
            | "meta"
            | "object"
            | "script"
            | "select"
            | "style"
            | "svg"
            | "textarea"
            | "video"
    )
}

fn is_url_attribute(name: &str) -> bool {
    matches!(name, "href" | "src" | "poster" | "xlink:href")
}

fn has_unsafe_url_scheme(value: &str) -> bool {
    let value = value.trim_start().to_ascii_lowercase();
    value.starts_with("javascript:")
}

fn collapse_inline_whitespace(value: &str) -> String {
    value.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn trim_outer_newlines(value: &str) -> &str {
    value.trim_matches(['\n', '\r'])
}
