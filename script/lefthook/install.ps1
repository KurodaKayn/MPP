$ErrorActionPreference = "Stop"

$repoRoot = git rev-parse --show-toplevel 2>$null
if (-not $repoRoot) {
    Write-Error "Not inside a Git repository."
    exit 1
}

Set-Location $repoRoot

if (-not (Test-Path "lefthook.yml")) {
    Write-Error "lefthook.yml not found in repository root: $repoRoot"
    exit 1
}

$localHooksPath = git config --get --local core.hooksPath 2>$null
$globalHooksPath = git config --get --global core.hooksPath 2>$null

$installArgs = @("install")
if ($localHooksPath -or $globalHooksPath) {
    git config --local core.hooksPath ".git/hooks"
    $installArgs = @("install", "--force")
}

function Invoke-Lefthook {
    param([string[]]$Arguments)

    if (Get-Command lefthook -ErrorAction SilentlyContinue) {
        & lefthook @Arguments
        return
    }

    if (Get-Command pnpm -ErrorAction SilentlyContinue) {
        & pnpm dlx lefthook @Arguments
        return
    }

    Write-Error "lefthook is not installed and pnpm is not available. Install lefthook or pnpm first."
    exit 1
}

Invoke-Lefthook -Arguments $installArgs

Write-Host "Lefthook initialized for $repoRoot"
