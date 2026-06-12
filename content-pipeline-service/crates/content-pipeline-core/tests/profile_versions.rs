use std::collections::BTreeSet;
use std::path::Path;

use content_pipeline_core::{supported_draft_profiles, supported_media_profiles};

#[test]
fn draft_profile_version_doc_lists_registered_profiles() {
    let documented = documented_profiles("## Current Draft Profiles");
    let registered = supported_draft_profiles()
        .iter()
        .map(|profile| profile.profile.to_string())
        .collect::<BTreeSet<_>>();

    assert_eq!(documented, registered);
}

#[test]
fn media_profile_version_doc_lists_registered_profiles() {
    let documented = documented_profiles("## Current Media Profiles");
    let registered = supported_media_profiles()
        .iter()
        .map(|profile| profile.profile.to_string())
        .collect::<BTreeSet<_>>();

    assert_eq!(documented, registered);
}

fn documented_profiles(section_heading: &str) -> BTreeSet<String> {
    let document = std::fs::read_to_string(profile_versions_path())
        .expect("PROFILE_VERSIONS.md should be readable");
    let section = document
        .split(section_heading)
        .nth(1)
        .unwrap_or_else(|| panic!("{section_heading} should exist in PROFILE_VERSIONS.md"));

    section
        .lines()
        .skip_while(|line| !line.starts_with("| `"))
        .take_while(|line| !line.starts_with("## "))
        .filter_map(first_backticked_cell)
        .collect()
}

fn first_backticked_cell(line: &str) -> Option<String> {
    let mut parts = line.split('`');
    parts.next()?;
    parts.next().map(ToString::to_string)
}

fn profile_versions_path() -> impl AsRef<Path> {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("../../PROFILE_VERSIONS.md")
}
