---
name: git-pr
description: Draft accurate Git pull request titles and descriptions from the current branch diff. Use when creating, updating, reviewing, or polishing PR metadata, especially when the user asks for a PR title, PR description, pull request body, merge request body, or GitHub/GitLab PR wording.
---

# Git PR Writing

Draft PR titles and descriptions that describe the actual branch changes, their
motivation, and their verification status.

## Required Workflow

Follow this sequence before writing PR metadata:

1. Collect repository context:
   - `git status --short`
   - `git branch --show-current`
   - `git log --oneline -20`
2. Identify the intended base branch:
   - Prefer the branch named by the user.
   - Otherwise use the upstream PR base if visible from existing tooling.
   - Otherwise infer the default branch from `origin/HEAD`, `main`, or `master`.
   - If the base branch is ambiguous and affects the diff, ask the user.
3. Inspect the branch changes against the base:
   - `git diff --stat <base>...HEAD`
   - `git diff --name-status <base>...HEAD`
   - `git diff --find-renames --find-copies <base>...HEAD`
4. Inspect relevant commits and changed files until you understand the user
   impact, implementation shape, and verification needs.
5. Draft the PR title and description using the rules below.
6. If the user asked you to create or update the PR, show the title and body and
   wait for explicit approval before running write operations such as
   `gh pr create`, `gh pr edit`, or GitLab equivalents.

## Hard Rules

- Treat the branch diff as the source of truth. Do not describe unrelated,
  unstaged, or uncommitted work unless the user explicitly asks for a draft from
  the working tree.
- Do not invent tests, screenshots, issue links, performance numbers, or product
  outcomes.
- If verification was not run, write `Not run (reason).`
- Keep PR metadata focused on reviewer understanding, not implementation trivia.
- Do not create, edit, merge, close, or push a PR without explicit user approval.
- Use English PR headings and descriptions by default. Do not use Chinese labels
  unless the user explicitly requests localization.

## PR Title Convention

Use this conventional format for every PR title:

```text
<type>(<scope>): <subject>
```

Examples:

```text
feat(auth): implement OAuth 2.0 login flow
fix(api): resolve token refresh race condition
refactor(core): simplify event dispatch pipeline
perf(cache): optimize Redis serialization path
docs(readme): update deployment instructions
test(queue): add retry mechanism coverage
chore(ci): migrate workflow to GitHub Actions
```

### Common Types

| Type     | Purpose                         |
| -------- | ------------------------------- |
| feat     | New feature                     |
| fix      | Bug fix                         |
| refactor | Internal code restructuring     |
| perf     | Performance optimization        |
| docs     | Documentation updates           |
| style    | Formatting / lint-only changes  |
| test     | Tests and validation            |
| build    | Build system or dependency work |
| ci       | CI/CD pipeline changes          |
| chore    | Miscellaneous maintenance       |
| revert   | Revert previous commit/PR       |

### Writing Principles

- Be precise and information-dense.
- Use imperative mood: `add`, `fix`, `remove`, `optimize`.
- Prefer technical nouns over vague wording.
- Keep under about 72 characters.
- Focus on what changed, not implementation details.
- Avoid filler words such as `some`, `various`, `better`, or `update stuff`.

### High-Quality Examples

```text
feat(editor): support multi-platform markdown rendering
fix(upload): prevent duplicate asset submission
refactor(storage): decouple provider initialization
perf(worker): reduce memory allocation during parsing
build(docker): enable multi-arch image generation
ci(release): automate semantic version publishing
```

### Weak vs Strong

Weak:

```text
fix bug
update code
improve system
change api
```

Strong:

```text
fix(parser): handle malformed frontmatter input
refactor(auth): isolate session validation logic
perf(search): reduce database round trips
```

## Required PR Content

Every complete PR draft must include these content areas:

- `Title`: a conventional PR title using `<type>(<scope>): <subject>`.
- `Feature Description` or `Change Description`: choose the heading that best
  matches the diff, then explain what the PR does and how it is used or
  observed.
- `Implementation Approach`: briefly explain the technical choice or core
  implementation logic.
- `Testing`: explain how to verify the feature/change works.

Use this default structure unless the repository has an existing PR template. If
a template exists, preserve its headings and fill these four content areas into
the closest matching sections.

```markdown
Title: <type>(<scope>): <subject>

## Feature Description
- ...

## Implementation Approach
- ...

## Testing
- ...
```

Use `## Change Description` instead of `## Feature Description` when the PR
mainly modifies, fixes, optimizes, documents, tests, or refactors existing
behavior.

Add optional sections only when they help reviewers:

- `## Why` for context that is not obvious from the diff.
- `## Screenshots` for visible UI changes.
- `## Risks` for migrations, compatibility changes, data changes, or uncertain
  behavior.
- `## Follow-ups` for known work intentionally left out of scope.
- `## Related` for issues, tickets, docs, or prior PRs.

## Section Guidance

### Feature Description or Change Description

- Use 1-4 bullets.
- Use `Feature Description` for new user-facing capabilities or workflows.
- Use `Change Description` for modifications, fixes, optimizations,
  documentation updates, tests, refactors, or removals.
- Explain the purpose, behavior, and usage or observable effect of the PR.
- Start with the most important user-facing or reviewer-relevant behavior.
- Mention removed behavior, config changes, API changes, migrations, or usage
  changes when present.

### Implementation Approach

- Use 1-3 bullets.
- Briefly explain the core implementation logic.
- Mention important technical choices, reused libraries, data flow, or module
  boundaries when they matter to review.
- Keep low-level code details out unless they affect behavior or risk.

### Testing

- List commands or manual checks that were actually run.
- Explain how each check verifies the feature/change works.
- Include meaningful results, not just tool names.
- If checks failed, include the failure and current status.
- If nothing was run, use `Not run (reason).`

### Optional Context

- Use `## Why` to explain the problem, user need, or maintenance gap when that
  context is not obvious from the description section.
- Use `## Screenshots` for visible UI changes, and include before/after
  screenshots only when they are actually available.
- Use `## Risks` to call out reviewer attention areas, rollout concerns,
  migration needs, or compatibility impact.
- Omit optional sections for routine low-risk changes.

## Output Format

When drafting PR metadata for the user, use this format:

```markdown
Title: <type>(<scope>): <subject>

## Feature Description
- ...

## Implementation Approach
- ...

## Testing
- ...
```

If the user asks for only a title or only a description, return only the
requested piece.
