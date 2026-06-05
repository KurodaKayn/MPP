---
name: git-pr
description: Draft concise Git PR titles and descriptions from committed branch diff. Keep metadata focused on actual changes, motivation, implementation, and verification.
---

# Git PR Writing

Draft PR metadata from committed branch diff. Do not invent context or verification.

## Required Workflow

1. Inspect context:
   - `git status --short`
   - `git branch --show-current`
   - `git log --oneline -20`
2. Find base branch:
   - Use user-provided base first.
   - Else use upstream PR base if tooling exposes it.
   - Else use `origin/HEAD`, `main`, or `master`.
   - Ask only if base ambiguity changes diff.
3. Inspect branch diff:
   - `git diff --stat <base>...HEAD`
   - `git diff --name-status <base>...HEAD`
   - `git diff --find-renames --find-copies <base>...HEAD`
4. Read relevant commits/files until behavior, implementation, and verification are clear.
5. Draft title/body.
6. Create/edit PR only after explicit approval.

## Hard Rules

- Branch diff is source of truth.
- Ignore unrelated unstaged/uncommitted work unless user asks for working-tree draft.
- Do not invent tests, screenshots, issue links, metrics, or outcomes.
- If verification was not run, write `Not run (reason).`
- Keep reviewer-focused. Skip implementation trivia.
- Do not create, edit, merge, close, or push PR without explicit approval.
- Use English headings by default unless user requests localization.

## Title

Format:

```text
<type>(<scope>): <subject>
```

Rules:

- Use narrowest type/scope matching branch diff.
- Imperative subject: `add`, `fix`, `remove`, `optimize`.
- Keep about <= 72 chars.
- Avoid vague words: `update`, `improve`, `misc`, `various`, `stuff`.

Types: `feat`, `fix`, `refactor`, `perf`, `docs`, `style`, `test`, `build`, `ci`, `chore`, `revert`.

## Body

Default structure:

```markdown
Title: <type>(<scope>): <subject>

## Feature Description
- ...

## Implementation Approach
- ...

## Testing
- ...
```

Use `## Change Description` instead of `## Feature Description` for fixes, refactors, docs, tests, build, CI, chores, or removals.

Section rules:

- Description: 1-4 bullets. State purpose, behavior, observable effect.
- Implementation: 1-3 bullets. Explain core approach, important boundaries, reused libs.
- Testing: list only checks actually run and result. If none: `Not run (reason).`

Optional sections only when useful:

- `## Why`
- `## Screenshots`
- `## Risks`
- `## Follow-ups`
- `## Related`

If user asks only title or only body, return only requested piece.
