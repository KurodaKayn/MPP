---
name: git-commit
description: Draft and execute atomic Angular-style Git commits from staged changes only. Enforce strict split between dependency, test, docs, and business/code changes.
---

# Git Commit Workflow

Draft English Angular-style commit messages from staged changes. Commit only after explicit approval.

## Required Workflow

1. Inspect context:
   - `git status --short`
   - `git branch --show-current`
   - `git log --oneline -10`
   - `GIT_PAGER=cat git --no-pager diff --staged --no-ext-diff --no-textconv --unified=5`
2. If nothing staged, inspect unstaged diff once:
   - `GIT_PAGER=cat git --no-pager diff --no-ext-diff --no-textconv --unified=5`
   - Run `git add .` only when all visible changes are obviously one atomic commit.
   - If mixed, stop. Ask user to stage first atomic subset.
3. Decide atomicity before writing message.
4. If staged diff is non-atomic, do not draft commit. Name smallest next staged subset.
5. Draft English commit message only.
6. Wait for explicit approval.
7. Run approved commit with direct `git commit -m ...` args only.

## Atomicity Gate

A commit is atomic only when every staged file is required for same single intent.

Use this strict test:

- Could file A land without file B? If yes -> split.
- Could file A be reverted while keeping file B? If yes -> split.
- Does title need `and`, `/`, multiple scopes, or vague words? If yes -> split.
- Are dependency/build changes mixed with business code? Split.
- Are tests mixed with business code? Split.
- Are docs/examples mixed with code? Split.
- Are formatting/lint-only changes mixed with behavior? Split.
- Are generated files, lockfiles, snapshots, or fixtures present? Keep only if they are mandatory result of same atomic change; otherwise split.

Default to smaller commits. One or two files is ideal when possible. File count is not proof, but unrelated convenience grouping is rejected.

## Hard Rules

- Do not commit without explicit user approval.
- Do not run `git push`.
- Do not describe unstaged or unrelated changes.
- Do not commit non-atomic staged changes.
- Do not hide mixed work behind `update`, `cleanup`, `misc`, `various`, or `apply changes`.
- Do not create temp commit message files.
- Commit drafts, previews, and actual `git commit` messages must be English only.
- Do not include Chinese or any localized translation in commit output.
- Body is required: exactly three short natural paragraphs.

## Commit Format

```text
type(scope): subject

Context or motivation.

Main change.

Result or impact.

Optional footer.
```

Rules:

- Title: `type(scope): subject`.
- Subject: imperative, lowercase first word, no trailing period.
- Title <= 80 chars.
- Full message < 600 chars.
- Body: exactly three short paragraphs, no labels like `Problem:` or `Change:`.
- Footer only for breaking changes or special notes.

## Types

- `feat`: new user-facing capability
- `fix`: bug/regression fix
- `docs`: docs only
- `style`: formatting/lint-only
- `refactor`: internal structure, no behavior change
- `perf`: performance/resource improvement
- `test`: tests only
- `build`: dependencies/package/build config
- `ci`: CI/CD
- `chore`: maintenance with no narrower type
- `revert`: rollback

Scope should be narrow module/feature/component name. Avoid `app`, `misc`, `core` unless truly accurate.

## Output

All users:

```text
type(scope): subject

Context or motivation.

Main change.

Result or impact.
```

After approval, execute:

```sh
git commit -m 'type(scope): subject' -m 'Context or motivation.' -m 'Main change.' -m 'Result or impact.'
```
