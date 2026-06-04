use std::sync::OnceLock;

use regex::Regex;

pub(super) const SHORT_TEXT_MAX_WEIGHT: usize = 280;
const HTTP_URL_WEIGHT: usize = 23;
pub(super) const SHORT_TEXT_WEIGHT_RULES: TextWeightRules = TextWeightRules {
    url_weight: HTTP_URL_WEIGHT,
};

pub(super) fn join_title_and_body_text(title: &str, text: &str) -> String {
    [title.trim(), text.trim()]
        .into_iter()
        .filter(|part| !part.is_empty())
        .collect::<Vec<_>>()
        .join("\n\n")
}

pub(super) fn text_with_fallback<'a>(
    text: &'a str,
    title: &'a str,
    source_content: &'a str,
) -> &'a str {
    [text, title, source_content]
        .into_iter()
        .map(str::trim)
        .find(|part| !part.is_empty())
        .unwrap_or("")
}

pub(super) fn truncate_weighted_text_with_ellipsis(
    value: &str,
    limit: usize,
    rules: TextWeightRules,
) -> String {
    if weighted_text_len(value, rules) <= limit {
        return value.to_string();
    }
    const SUFFIX: &str = "...";
    let budget = limit.saturating_sub(SUFFIX.len());
    format!(
        "{}{}",
        truncate_text_by_weight(value, budget, rules).trim_end(),
        SUFFIX
    )
}

fn truncate_text_by_weight(value: &str, limit: usize, rules: TextWeightRules) -> String {
    let mut used = 0;
    let mut output = String::new();
    for ch in value.chars() {
        let weight = weighted_char_len(ch, rules);
        if used + weight > limit {
            break;
        }
        output.push(ch);
        used += weight;
    }
    output
}

#[derive(Debug, Clone, Copy)]
pub(super) struct TextWeightRules {
    url_weight: usize,
}

fn weighted_text_len(value: &str, rules: TextWeightRules) -> usize {
    let mut length = 0;
    let mut last = 0;
    for matched in http_url_pattern().find_iter(value) {
        length += weighted_text_segment_len(&value[last..matched.start()], rules);
        length += rules.url_weight;
        last = matched.end();
    }
    length + weighted_text_segment_len(&value[last..], rules)
}

fn weighted_text_segment_len(value: &str, rules: TextWeightRules) -> usize {
    value.chars().map(|ch| weighted_char_len(ch, rules)).sum()
}

fn weighted_char_len(ch: char, _rules: TextWeightRules) -> usize {
    if ch.is_ascii() || is_latin(ch) { 1 } else { 2 }
}

fn is_latin(ch: char) -> bool {
    matches!(
        ch as u32,
        0x00c0..=0x024f | 0x1e00..=0x1eff | 0x2c60..=0x2c7f | 0xa720..=0xa7ff | 0xab30..=0xab6f
    )
}

fn http_url_pattern() -> &'static Regex {
    static URL_PATTERN: OnceLock<Regex> = OnceLock::new();
    URL_PATTERN.get_or_init(|| Regex::new(r#"https?://[^\s<>"']+"#).expect("valid URL regex"))
}
