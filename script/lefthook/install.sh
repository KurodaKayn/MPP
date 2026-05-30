#!/usr/bin/env sh

set -eu

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"
cd "$repo_root"

if [ ! -f "lefthook.yml" ]; then
  echo "lefthook.yml not found in repository root: $repo_root" >&2
  exit 1
fi

local_hooks_path="$(git config --get --local core.hooksPath || true)"
global_hooks_path="$(git config --get --global core.hooksPath || true)"

if [ -n "$local_hooks_path" ] || [ -n "$global_hooks_path" ]; then
  git config --local core.hooksPath .git/hooks
  install_args="install --force"
else
  install_args="install"
fi

run_lefthook() {
  if command -v lefthook >/dev/null 2>&1; then
    lefthook "$@"
    return
  fi

  if command -v pnpm >/dev/null 2>&1; then
    pnpm dlx lefthook "$@"
    return
  fi

  echo "lefthook is not installed and pnpm is not available." >&2
  echo "Install lefthook with 'brew install lefthook' or install pnpm first." >&2
  exit 1
}

# shellcheck disable=SC2086
run_lefthook $install_args

echo "Lefthook initialized for $repo_root"
