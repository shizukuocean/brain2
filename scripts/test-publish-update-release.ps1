param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot,

    [Parameter(Mandatory = $true)]
    [string]$Tag,

    [string]$Repository = 'shizukuocean/meow-releases'
)

$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'publish-update-release.ps1'
$bytes = [IO.File]::ReadAllBytes($script)
if (@($bytes | Where-Object { $_ -ge 128 }).Count -ne 0) {
    throw 'The publish script must remain ASCII-compatible with Windows PowerShell 5.'
}
$scriptText = [Text.Encoding]::ASCII.GetString($bytes)
foreach ($requiredProbePart in @('Assert-PublishedAssetReachable', 'curl.exe', '--head', '--retry 4', '--retry-all-errors')) {
    if (-not $scriptText.Contains($requiredProbePart)) {
        throw "The publish script is missing resilient asset probing: $requiredProbePart"
    }
}

$beforeRelease = @(& gh release list --repo $Repository --limit 1 --json tagName,publishedAt | ConvertFrom-Json)[0]
$beforeCommit = & gh api "repos/$Repository/commits?path=channels/preview/manifest-v1.json" --jq '.[0].sha'

$preflightText = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -ReleaseRoot $ReleaseRoot -Tag $Tag
if ($LASTEXITCODE -ne 0) { throw 'Publish preflight failed.' }
$preflight = ($preflightText -join "`n") | ConvertFrom-Json
if (-not $preflight.ok -or $preflight.mode -ne 'preflight' -or $preflight.published -ne $false) {
    throw 'Preflight did not fail closed.'
}

$oldPreference = $ErrorActionPreference
try {
    $ErrorActionPreference = 'Continue'
    $negativeText = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -ReleaseRoot $ReleaseRoot -Tag $Tag -Publish -ConfirmTag 'INTENTIONALLY-WRONG' 2>&1
    $negativeExit = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $oldPreference
}
if ($negativeExit -eq 0 -or ($negativeText -join "`n") -notmatch 'ConfirmTag') {
    throw 'Publish confirmation guard did not reject a mismatched tag.'
}

$afterRelease = @(& gh release list --repo $Repository --limit 1 --json tagName,publishedAt | ConvertFrom-Json)[0]
$afterCommit = & gh api "repos/$Repository/commits?path=channels/preview/manifest-v1.json" --jq '.[0].sha'
if ($beforeRelease.tagName -ne $afterRelease.tagName -or $beforeRelease.publishedAt -ne $afterRelease.publishedAt -or $beforeCommit -ne $afterCommit) {
    throw 'A preflight-only test changed remote release state.'
}

[ordered]@{
    ok = $true
    preflight = $true
    confirmation_guard = $true
    remote_unchanged = $true
    latest_release = $afterRelease.tagName
    channel_commit = $afterCommit
} | ConvertTo-Json -Depth 4
