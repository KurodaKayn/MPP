---
name: git-branch
description: Generate industry-standard Git branch names based on issue context, task descriptions, or user requests. Adheres to conventions used by high-star open-source projects.
---

# Git Branch Naming Convention

Generate standardized, readable, and trackable Git branch names based on task descriptions, issues, or user intent.

## Required Workflow

1. **Understand Context**: Ask the user for the task description, issue link, or feature they are working on if not fully provided.
2. **Determine Type**: Categorize the work (e.g., feature, bug, refactor) into standard prefixes.
3. **Draft Options**: Generate 2-3 branch name options following the industry standards outlined below.
4. **Include Commands**: Provide the exact `git checkout -b <branch-name>` command for easy execution.
5. **Execution**: If the user approves or requests it, execute the branch creation command.

## Industry Standards & Conventions

Major open-source projects (like Vue, React, Angular, Kubernetes) and enterprise engineering teams follow structured formats for branch naming. This ensures readability, context, and integration with CI/CD and issue tracking tools.

### 1. General Structure

The most widely adopted pattern utilizes hierarchical grouping using slashes (`/`):

```text
<type>/[<issue-id>-]<short-description>
```

Alternatively, for team environments requiring explicit ownership:

```text
<author>/<type>/[<issue-id>-]<short-description>
```

### 2. Semantic Types (Prefixes)

Match the branch type to semantic commit conventions (such as Angular's commit guidelines):

- `feat/`: A new feature or capability.
- `fix/` (or `bugfix/`): A bug fix.
- `hotfix/`: An urgent, critical fix applied directly to a production/main branch.
- `chore/`: Routine tasks, dependency updates, tooling, or configuration changes.
- `docs/`: Documentation updates.
- `refactor/`: Code structure changes that neither fix a bug nor add a feature.
- `test/`: Adding missing tests or correcting existing tests.
- `perf/`: Code changes that improve performance.
- `ci/`: Changes to CI/CD pipelines (e.g., GitHub Actions, GitLab CI).
- `style/`: Code styling changes (formatting, linting rules).

*Note: The `/` acts as a directory separator in most Git clients (like SourceTree, GitKraken, or VS Code), visually grouping branches by type.*

### 3. Formatting Rules

- **Strictly Lowercase**: Branch names must be exclusively lowercase to prevent case-collision issues on case-insensitive filesystems (like macOS and Windows).
- **Kebab-Case Separators**: Use hyphens (`-`) to separate words. **Never** use spaces, underscores (`_`), or CamelCase.
- **Concise Description**: Use 3 to 6 descriptive words. Focus on the *intent* of the branch.
- **No Trailing Separators**: Avoid ending the branch name with a hyphen or slash.
- **Alphanumeric Only**: Stick to `a-z`, `0-9`, `/`, and `-`. Avoid special characters like `!`, `@`, `#`, `?`, or quotes.

### 4. Issue Tracker Integration

If the user mentions an issue ID (e.g., from Jira, GitHub Issues, Linear), it must be integrated into the branch name. This enables automatic PR linking and tracking.

- **Format**: `<type>/<issue-id>-<short-description>`
- **Example**: `feat/PROJ-123-add-oauth-login`
- **Example**: `fix/456-crash-on-startup`

## Examples

### ✅ Good (Industry Standard)
- `feat/add-payment-gateway`
- `fix/TICKET-892-null-pointer-login`
- `docs/update-readme-api-endpoints`
- `hotfix/fix-prod-db-connection`
- `chore/upgrade-webpack-v5`
- `kuroda/feat/user-profile-page`

### ❌ Bad (Avoid)
- `fix_login` *(Uses underscore, missing semantic prefix)*
- `Feature-Add-Login` *(Uses uppercase, uses hyphen instead of slash for type)*
- `bug/123` *(Not descriptive enough)*
- `feat/add login form` *(Contains spaces)*
- `kuroda's-branch` *(Uninformative, uses special characters)*

## Interaction Guidelines

- If the user provides a vague description (e.g., "create a branch for login"), output a standard name like `feat/login-system` but politely ask if there is an associated issue ticket number to include.
- Always provide the full CLI command, e.g., `` `git checkout -b feat/login-system` ``.
- If the user's preferred language is Chinese, present your explanations and suggestions in Chinese, but keep the branch names themselves strictly in English alphanumeric characters.
