#!/usr/bin/env sh

set -eu

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "Not inside a git repository." >&2
  exit 1
}

cd "$repo_root"

if ! git show-ref --verify --quiet refs/heads/main; then
  echo "Local branch 'main' does not exist." >&2
  exit 1
fi

current_branch="$(git branch --show-current)"
if [ "$current_branch" != "main" ]; then
  git switch main
fi

git for-each-ref --format='%(refname:short)' refs/heads \
  | while IFS= read -r branch; do
      if [ "$branch" != "main" ]; then
        git branch -D "$branch"
      fi
    done

