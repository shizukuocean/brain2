param(
    [Parameter(Mandatory = $true)]
    [string]$BaseUrl,

    [Parameter(Mandatory = $true)]
    [string]$ManifestUrl,

    [string]$HelperRepository = 'C:\Example\DevHelper',
    [string]$ManagerVersion = '0.6.7-preview.1',
    [string]$LauncherVersion = '0.6.7-preview.1',
    [string]$HelperVersion = '0.3.1-preview.1',
    [string]$ManagerReleaseNotes = 'Adds in-app updates handled by Meow Launcher with optional restart.',
    [string]$LauncherReleaseNotes = 'Adds unified product updates and non-resident one-shot Launcher self-update.',
    [string]$HelperReleaseNotes = 'Adds in-app updates handled by Meow Launcher with optional restart.',
    [string]$ReleaseNotesPath = '',
    [string]$PreviousManagerInstaller = '',
    [string]$PreviousHelperInstaller = '',
    [string]$PreviousLauncherInstaller = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
if ([string]::IsNullOrWhiteSpace($ReleaseNotesPath)) {
    $ReleaseNotesPath = Join-Path $repoRoot 'docs\product\release-notes-preview-v1.json'
}
$baseUri = $null
if (-not [Uri]::TryCreate($BaseUrl, [UriKind]::Absolute, [ref]$baseUri) -or $baseUri.Scheme -ne 'https') {
    throw 'BaseUrl must be an absolute HTTPS URL.'
}
if ($baseUri.Host -eq 'example.invalid' -or $baseUri.Host.EndsWith('.invalid')) {
    throw 'Release staging cannot use a placeholder .invalid host.'
}
if (-not [string]::IsNullOrEmpty($baseUri.UserInfo)) {
    throw 'BaseUrl cannot embed credentials or tokens.'
}
$manifestUri = $null
if (-not [Uri]::TryCreate($ManifestUrl, [UriKind]::Absolute, [ref]$manifestUri) -or $manifestUri.Scheme -ne 'https') {
    throw 'ManifestUrl must be an absolute HTTPS URL.'
}
if ($manifestUri.Host -eq 'example.invalid' -or $manifestUri.Host.EndsWith('.invalid')) {
    throw 'Release staging cannot use a placeholder ManifestUrl host.'
}
if (-not [string]::IsNullOrEmpty($manifestUri.UserInfo)) {
    throw 'ManifestUrl cannot embed credentials or tokens.'
}
$normalizedBaseUrl = $baseUri.AbsoluteUri.TrimEnd('/')
$normalizedManifestUrl = $manifestUri.AbsoluteUri
if ($normalizedManifestUrl.StartsWith($normalizedBaseUrl + '/', [StringComparison]::OrdinalIgnoreCase) -or $manifestUri.AbsolutePath -match '(?i)/releases/download/') {
    throw 'ManifestUrl must be a permanent channel URL, not a version-specific release asset URL.'
}

$previewRoot = Join-Path $repoRoot 'dist\update-preview'
$releaseRoot = Join-Path $previewRoot ('stage-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8))
& (Join-Path $repoRoot 'scripts\build-update-preview.ps1') `
    -HelperRepository $HelperRepository `
    -ManagerVersion $ManagerVersion `
    -LauncherVersion $LauncherVersion `
    -HelperVersion $HelperVersion `
    -BaseUrl $baseUri.AbsoluteUri.TrimEnd('/') `
    -ManifestUrl $manifestUri.AbsoluteUri `
    -ManagerReleaseNotes $ManagerReleaseNotes `
    -LauncherReleaseNotes $LauncherReleaseNotes `
    -HelperReleaseNotes $HelperReleaseNotes `
    -ReleaseNotesPath $ReleaseNotesPath `
    -ReleaseRoot $releaseRoot
if ($LASTEXITCODE -ne 0) { throw 'Update release build failed.' }

if (-not (Test-Path -LiteralPath (Join-Path $releaseRoot 'manifest-v1.json') -PathType Leaf)) {
    throw "The build did not create the expected release directory: $releaseRoot"
}
$release = Get-Item -LiteralPath $releaseRoot

& (Join-Path $repoRoot 'scripts\test-update-preview.ps1') -ReleaseRoot $release.FullName
if ($LASTEXITCODE -ne 0) { throw 'Release end-to-end validation failed.' }
$previousInstallers = @(@($PreviousManagerInstaller, $PreviousHelperInstaller, $PreviousLauncherInstaller) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($previousInstallers.Count -notin @(0, 3)) {
    throw 'PreviousManagerInstaller, PreviousHelperInstaller and PreviousLauncherInstaller must be supplied together.'
}
if (-not [string]::IsNullOrWhiteSpace($PreviousManagerInstaller)) {
    & (Join-Path $repoRoot 'scripts\test-update-in-place.ps1') `
        -ReleaseRoot $release.FullName `
        -PreviousManagerInstaller $PreviousManagerInstaller `
        -PreviousHelperInstaller $PreviousHelperInstaller `
        -PreviousLauncherInstaller $PreviousLauncherInstaller
    if ($LASTEXITCODE -ne 0) { throw 'In-place update validation failed.' }
}
& (Join-Path $repoRoot 'scripts\audit-update-release.ps1') -ReleaseRoot $release.FullName -Publish
if ($LASTEXITCODE -ne 0) { throw 'Release publish gate failed.' }

$publishRoot = Join-Path $release.FullName 'publish'
New-Item -ItemType Directory -Force -Path $publishRoot | Out-Null
Copy-Item -LiteralPath (Join-Path $release.FullName 'manifest-v1.json') -Destination (Join-Path $publishRoot 'manifest-v1.json')
Copy-Item -Path (Join-Path $release.FullName 'artifacts\*.exe') -Destination $publishRoot

$publishFiles = @(Get-ChildItem -File -LiteralPath $publishRoot | Sort-Object Name)
$hashLines = @($publishFiles | ForEach-Object { '{0}  {1}' -f (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant(), $_.Name })
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllLines((Join-Path $publishRoot 'SHA256SUMS.txt'), $hashLines, $utf8WithoutBom)

[ordered]@{
    ok = $true
    staged_only = $true
    uploaded = $false
    release_root = $release.FullName
    publish_root = $publishRoot
    remote_manifest_url = $manifestUri.AbsoluteUri
    files = @(Get-ChildItem -File -LiteralPath $publishRoot | Sort-Object Name | ForEach-Object { $_.FullName })
} | ConvertTo-Json -Depth 4
