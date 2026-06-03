---
name: git-commit
description: Draft atomic Angular-style Git commit messages from staged changes and execute `git commit` safely after explicit approval. Use for commit message generation, staged-diff review for atomic scope and wording, and preferred-language previews of English commit messages for non-English users.
---

# Git Commit Workflow

Generate accurate, atomic Angular-style Git commit messages from staged
changes only.

## Required Workflow

Follow this sequence exactly:

1. Collect context with:
   - `git status`
   - staged raw patch command:
     `GIT_PAGER=cat git --no-pager diff --staged --no-ext-diff --no-textconv --unified=5`
   - `git branch --show-current`
   - `git log --oneline -10`
2. If there are no staged changes, inspect `git status --short` and the
   unstaged raw patch command:
   `GIT_PAGER=cat git --no-pager diff --no-ext-diff --no-textconv --unified=5`.
   Run `git add .` once only when the entire worktree clearly represents one
   atomic change. If the worktree mixes unrelated changes or cannot be judged
   from the visible diff, stop and ask the user to stage the first intended
   atomic subset.
3. If there were already staged changes in step 1, do not run `git add`. Keep
   the workflow limited to the current staged scope.
4. Stop immediately if there are still no staged changes after the single
   allowed `git add .`. Ask the user to stage files first.
5. Analyze the staged diff precisely:
   - Treat the raw patch command above as the only source of truth for staged
     scope. Do not rely on external diff, textconv, or pager formatting.
   - If you need to inspect a single staged path, reuse the same command shape
     and append `-- <path>` instead of falling back to bare `git diff --staged`.
   - Identify additions, deletions, and behavior impact.
   - Decide whether the staged changes are atomic before drafting any message.
   - Infer the most accurate `type(scope): subject`.
6. If the staged diff is not atomic, do not draft a commit message. Explain the
   smallest sensible split and ask the user to stage one atomic subset.
7. Draft the commit message in English using the format rules below.
8. If `{USR_PREFERRED_LANGUAGE}` is not English, prepare a
   `{USR_PREFERRED_LANGUAGE}` preview that fully matches the English message.
9. Present the result using the output rules below and wait for explicit approval.
10. After approval, execute the commit directly with `git commit -m` arguments:
   - Use one `-m` argument for the title and one `-m` argument for each body
     paragraph.
   - Add an optional footer as the final `-m` argument only when applicable.
   - Do not create `commit_message.txt` or any other temporary message file.

## User Preferred Language

`{USR_PREFERRED_LANGUAGE}` means the first language in macOS `AppleLanguages`.
Read it with `defaults read -g AppleLanguages` and use the first list entry. If
the current agent environment cannot read that value, write in the language the
user is already using in the current conversation.

Treat English language variants as English. For English users, do not add a
second preview block that repeats the English commit message.

## Hard Rules

- Run `git add .` only when the initial staged diff is empty, only once, and
  only when all unstaged and untracked changes clearly belong to one atomic
  commit.
- Do not run `git add` when staged changes already exist.
- Do not run `git push`.
- Do not commit without explicit user authorization.
- Do not include any non-English preview text in the actual `git commit`
  command.
- Do not describe unstaged or unrelated changes.
- Do not draft or execute a commit for non-atomic staged changes.
- Do not create `commit_message.txt` or any other temporary message file.
- Do not hide mixed intent behind broad subjects such as `update files`,
  `misc changes`, `cleanup`, or `apply fixes`.
- If `{USR_PREFERRED_LANGUAGE}` is not English, keep that preview accurate and
  complete.
- A commit message is incomplete unless it includes a body explaining what changed and why.
- Every commit message is incomplete unless the body uses exactly three natural
  paragraphs.
- The three body paragraphs must cover the current context, the main change,
  and the resulting impact in that order.
- Do not chain `git commit` together with file creation or cleanup in a single shell command.

## Atomic Scope Rules

An atomic commit contains one coherent intent that can be reviewed, reverted,
and described independently. Prefer the smallest practical staged file set;
one or two files is often ideal, but file count alone does not determine
atomicity.

Treat staged changes as atomic only when all of these are true:

- They solve one problem, add one capability, or make one maintenance change.
- They keep staged files to the minimum set required for that commit to stand
  on its own.
- They fit one primary `type(scope): subject` without vague wording.
- They can be reverted together without undoing an unrelated improvement.
- Every staged file is strictly required for the same change. If files can land
  independently, split them even when they affect the same feature or influence
  each other.
- Tests, docs, examples, fixtures, and snapshots are separate commit intents.
  Do not bundle them with production changes even when they directly describe
  or verify the primary change.

Stop and ask for a smaller staged subset when any of these are true:

- The staged diff mixes unrelated features, fixes, refactors, docs, formatting,
  dependency updates, or test-only changes.
- The staged files touch separate modules for unrelated reasons.
- The staged diff includes production code plus tests, docs, examples, fixtures,
  or snapshots.
- Any staged file is related only by convenience, shared feature area, or
  downstream influence rather than being required for the same change to stand
  on its own.
- The best subject would need `and`, `/`, `misc`, `various`, or multiple scopes
  to be honest.
- Generated files, lockfiles, snapshots, or formatting churn appear without a
  clear tie to the primary change.
- A revert would reasonably need to keep part of the staged diff.

When a split is needed, name the smallest next commit first. Do not propose a
full release-sized plan unless the user asks for it.

## Commit Execution Rules

- Execute approved messages directly with separate `-m` arguments:
  `git commit -m 'type(scope): subject' -m 'First body paragraph.' -m 'Second body paragraph.' -m 'Third body paragraph.'`
- Quote each `-m` argument safely for the current shell. If the message
  contains single quotes, escape them correctly or use double quotes only when
  shell expansion cannot change the message.
- Do not create, write, read, or remove a temporary commit message file.
- Treat `git commit` as the only step that needs repository write access.
- If `git commit` fails with sandbox-style permission errors such as `Operation not permitted` while creating `.git/index.lock`, immediately rerun the same direct `git commit -m ...` command with the required escalation instead of retrying the same non-privileged command.
- When the environment is known to block writes under `.git`, prefer requesting the needed escalation for `git commit` directly after the user approves the message.

## Angular-Style Commit Format

Use this structure for every commit:

```text
type(scope): subject

First body paragraph explaining the current context or motivation.

Second body paragraph explaining the main change.

Third body paragraph explaining the result or impact.

Optional footer for breaking changes or special notes when applicable.
```

Apply these formatting rules:

- Use the standard title form: `type(scope): subject`.
- Write the `subject` as an imperative summary, start it with a lowercase letter, and do not end it with a period.
- Keep the title at or below 80 characters.
- Keep the full commit message under 600 characters.
- Make the title narrow enough that it describes only the staged atomic change.
- The `body` is required for every commit and must use exactly three short
  paragraphs separated by one blank line.
- The first paragraph should explain the current issue, context, or motivation.
- The second paragraph should explain the main change and how it responds to
  that context.
- The third paragraph should explain the result, impact, or risk reduction.
- Do not use explicit labels such as `Problem:`, `Change:`, or `Summary:`.
- Keep each paragraph concise, usually one sentence and at most two when
  needed.
- Focus on behavior and intent rather than low-level implementation minutiae.
- Use the optional `footer` only for breaking changes or special considerations.

### Three-Paragraph Body Structure

Every commit must use three short natural paragraphs in this order:

1. Describe the current background, problem, or motivation for the commit.
2. Describe the main change and how it responds to that context.
3. Summarize the result, user impact, compatibility effect, or reduced risk.

### Breaking Changes

- Mark breaking changes with `!`, with a `BREAKING CHANGE:` footer, or with
  both when the title should signal the break immediately and the footer needs
  to explain migration impact.
- Use `!` after the type or scope in the title when the incompatible change
  should be visible at a glance.
- Use a `BREAKING CHANGE:` footer when migration work, removed behavior, or
  compatibility impact needs explicit explanation.
- Keep the full commit structure intact for breaking changes: title, three
  body paragraphs, and then the optional footer. The footer explains the
  compatibility impact, but it does not replace the required body paragraphs.

## Commit Type Guidance

Choose the narrowest commit `type` that matches the staged diff. If no single
type honestly covers the staged diff, treat the diff as non-atomic unless the
extra files directly support the primary change.

- `feat`: introduce user-facing behavior or a new capability. Focus the three
  paragraphs on the missing capability, the feature added, and the user-facing
  benefit or rollout impact.
- `fix`: correct a bug, regression, or broken behavior. Focus the three
  paragraphs on the broken behavior, the fix approach, and the restored
  outcome or reduced risk.
- `docs`: update documentation only. Focus the three paragraphs on the reader
  gap, the documentation update, and the clarity or maintenance benefit.
- `style`: apply formatting or non-functional code style updates. Focus the
  three paragraphs on the readability issue, the style cleanup, and the
  consistency benefit.
- `refactor`: improve internal structure without changing behavior. Focus the
  three paragraphs on the structural pain point, the code reorganization, and
  the maintainability gain while preserving behavior.
- `perf`: improve performance or reduce resource usage. Focus the three
  paragraphs on the bottleneck, the optimization, and the measured or expected
  efficiency gain.
- `test`: add or adjust tests without changing production behavior. Focus the
  three paragraphs on the coverage gap, the test update, and the regression
  protection gained.
- `build`: change dependencies, packaging, or build configuration. Focus the
  three paragraphs on the build or dependency context, the configuration
  change, and the resulting build or release impact.
- `ci`: update CI workflows or automation pipelines. Focus the three
  paragraphs on the pipeline issue, the workflow change, and the resulting
  reliability or maintenance improvement.
- `chore`: make routine maintenance changes that do not fit other types. Focus
  the three paragraphs on the maintenance need, the housekeeping change, and
  the resulting repository health benefit.
- `revert`: roll back a previous change. Focus the three paragraphs on why the
  earlier change must be undone, what is being reverted, and the restored or
  stabilized state afterward.

Choose `scope` from the touched module, feature, service, or component name whenever possible. Prefer specific scopes such as `openai`, `screenshot`, or `settings` over broad labels like `app` or `misc`.

## Output and Approval Rules

- Present the result and wait for explicit approval.
- If `{USR_PREFERRED_LANGUAGE}` is English, use this exact format:

```
{English commit message}
```

- If `{USR_PREFERRED_LANGUAGE}` is not English, use this exact format:

```
{English commit message}
```

```
{{USR_PREFERRED_LANGUAGE} preview}
```

- Keep the `{USR_PREFERRED_LANGUAGE}` preview aligned with the English message
  in paragraph count, paragraph order, and meaning for every commit type.
- Do not create `commit_message.txt` or run `git commit` before explicit approval.
- Put only the English title, body paragraphs, and optional footer into the
  actual `git commit -m ...` command.

## Example

This example shows the required complete format for a compliant commit message.

```
fix(screenshot): defer overlay capture until view appears

Overlay capture started before the view hierarchy was stable, which caused a startup race and could crash screenshot translation.

Move screenshot capture out of the overlay initializer and begin it only after the view appears and layout is ready.

This restores stable screenshot translation startup and prevents the layout conflicts caused by the race.
```
