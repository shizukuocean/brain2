param(
    [string]$HelperRepository = 'C:\Example\DevHelper',
    [string]$ManagerVersion = '0.6.7-preview.1',
    [string]$LauncherVersion = '0.6.7-preview.1',
    [string]$HelperVersion = '0.3.1-preview.1',
    [string]$BaseUrl = 'https://downloads.example.invalid/meow-preview',
    [string]$ManifestUrl = '',
    [string]$ManagerReleaseNotes = 'Adds in-app updates handled by Meow Launcher with optional restart.',
    [string]$LauncherReleaseNotes = 'Adds unified product updates and non-resident one-shot Launcher self-update.',
    [string]$HelperReleaseNotes = 'Adds in-app updates handled by Meow Launcher with optional restart.',
    [string]$ReleaseNotesPath = '',
    [string]$ReleaseRoot = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$frontendRoot = Join-Path $repoRoot 'frontend'
$timestamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss')
if ([string]::IsNullOrWhiteSpace($ReleaseRoot)) {
    $releaseRoot = Join-Path $repoRoot (Join-Path 'dist\update-preview' $timestamp)
} else {
    $releaseRoot = [IO.Path]::GetFullPath($ReleaseRoot)
}
if (Test-Path -LiteralPath $releaseRoot) {
    throw "ReleaseRoot already exists and will not be overwritten: $releaseRoot"
}
$toolsRoot = Join-Path $releaseRoot 'tools'
$payloadRoot = Join-Path $releaseRoot 'payloads'
$artifactsRoot = Join-Path $releaseRoot 'artifacts'

$parsedBaseUrl = [Uri]$BaseUrl
if ($parsedBaseUrl.Scheme -ne 'https' -or [string]::IsNullOrWhiteSpace($parsedBaseUrl.Host)) {
    throw 'BaseUrl must be an absolute HTTPS URL.'
}
if (-not [string]::IsNullOrEmpty($parsedBaseUrl.UserInfo)) {
    throw 'BaseUrl cannot embed credentials or tokens.'
}
if ([string]::IsNullOrWhiteSpace($ManifestUrl)) {
    $ManifestUrl = $BaseUrl.TrimEnd('/') + '/manifest-v1.json'
}
$parsedManifestUrl = [Uri]$ManifestUrl
if ($parsedManifestUrl.Scheme -ne 'https' -or [string]::IsNullOrWhiteSpace($parsedManifestUrl.Host)) {
    throw 'ManifestUrl must be an absolute HTTPS URL.'
}
if (-not [string]::IsNullOrEmpty($parsedManifestUrl.UserInfo)) {
    throw 'ManifestUrl cannot embed credentials or tokens.'
}
$ManifestUrl = $parsedManifestUrl.AbsoluteUri
$versionPattern = '^\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$'
foreach ($versionEntry in ([ordered]@{ ManagerVersion = $ManagerVersion; LauncherVersion = $LauncherVersion; HelperVersion = $HelperVersion }).GetEnumerator()) {
    if ([string]$versionEntry.Value -notmatch $versionPattern) {
        throw "$($versionEntry.Key) has an invalid version format."
    }
}
if (-not [string]::IsNullOrWhiteSpace($ReleaseNotesPath)) {
    $resolvedNotesPath = (Resolve-Path -LiteralPath $ReleaseNotesPath).Path
    $notesDocument = Get-Content -Raw -Encoding UTF8 -LiteralPath $resolvedNotesPath | ConvertFrom-Json
    $ManagerReleaseNotes = [string]$notesDocument.brain2_manager
    $HelperReleaseNotes = [string]$notesDocument.dev_helper
    $LauncherReleaseNotes = [string]$notesDocument.meow_launcher
}
foreach ($notesEntry in ([ordered]@{ ManagerReleaseNotes = $ManagerReleaseNotes; LauncherReleaseNotes = $LauncherReleaseNotes; HelperReleaseNotes = $HelperReleaseNotes }).GetEnumerator()) {
    if ([string]::IsNullOrWhiteSpace([string]$notesEntry.Value)) {
        throw "$($notesEntry.Key) cannot be empty."
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $HelperRepository 'package.json'))) {
    throw "Dev Helper repository not found: $HelperRepository"
}

New-Item -ItemType Directory -Force -Path $toolsRoot, $payloadRoot, $artifactsRoot | Out-Null

Push-Location $frontendRoot
try {
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Brain2 Manager frontend build failed.' }
    & npm.cmd run build:launcher
    if ($LASTEXITCODE -ne 0) { throw 'Meow Launcher frontend build failed.' }
} finally {
    Pop-Location
}

$managerPayload = Join-Path $payloadRoot 'brain2-manager'
$launcherPayload = Join-Path $payloadRoot 'meow-launcher'
$helperPayload = Join-Path $payloadRoot 'dev-helper'
New-Item -ItemType Directory -Force -Path $managerPayload, $launcherPayload, $helperPayload | Out-Null

$versionLdflags = "-s -w -H=windowsgui -X meow-launcher/internal/buildinfo.ManagerVersion=$ManagerVersion -X meow-launcher/internal/buildinfo.ManagerDisplayVersion=$ManagerVersion -X meow-launcher/internal/buildinfo.LauncherVersion=$LauncherVersion -X meow-launcher/internal/buildinfo.LauncherDisplayVersion=$LauncherVersion -X meow-launcher/internal/buildinfo.UpdateManifestURL=$ManifestUrl"
$managerExecutable = Join-Path $managerPayload 'Brain2 Manager.exe'
$launcherExecutable = Join-Path $launcherPayload 'Meow Launcher.exe'
$installerRunner = Join-Path $toolsRoot 'meow-product-installer.exe'
$packageBuilder = Join-Path $toolsRoot 'meow-package-builder.exe'
$managerIcon = Join-Path $repoRoot 'assets\brain2-manager-icon.png'
$managerResource = Join-Path $repoRoot 'manager_icon_windows_amd64.syso'
$launcherIcon = Join-Path $repoRoot 'assets\meow-launcher-icon.png'
$launcherResource = Join-Path $repoRoot 'launcher_icon_windows_amd64.syso'

Push-Location $repoRoot
try {
    if (Test-Path -LiteralPath $managerResource) {
        throw "Refusing to overwrite existing Manager resource: $managerResource"
    }
    & go run .\cmd\meow-icon-resource --input $managerIcon --output $managerResource
    if ($LASTEXITCODE -ne 0) { throw 'Manager icon resource generation failed.' }
    try {
        & go build -tags 'desktop,production' -trimpath -ldflags $versionLdflags -o $managerExecutable .
        if ($LASTEXITCODE -ne 0) { throw 'Brain2 Manager executable build failed.' }
    } finally {
        Remove-Item -LiteralPath $managerResource -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $launcherResource) {
        throw "Refusing to overwrite existing Launcher resource: $launcherResource"
    }
    & go run .\cmd\meow-icon-resource --input $launcherIcon --output $launcherResource
    if ($LASTEXITCODE -ne 0) { throw 'Launcher icon resource generation failed.' }
    try {
        & go build -tags 'launcher,desktop,production' -trimpath -ldflags $versionLdflags -o $launcherExecutable .
        if ($LASTEXITCODE -ne 0) { throw 'Meow Launcher executable build failed.' }
    } finally {
        Remove-Item -LiteralPath $launcherResource -Force -ErrorAction SilentlyContinue
    }
    & go build -trimpath -ldflags '-s -w -H=windowsgui' -o $installerRunner .\cmd\meow-product-installer
    if ($LASTEXITCODE -ne 0) { throw 'Product installer runner build failed.' }
    & go build -trimpath -ldflags '-s -w' -o $packageBuilder .\cmd\meow-package-builder
    if ($LASTEXITCODE -ne 0) { throw 'Package builder build failed.' }
} finally {
    Pop-Location
}

$previousHelperBuildVersion = $env:DEV_HELPER_BUILD_VERSION
$previousHelperManifestUrl = $env:DEV_HELPER_UPDATE_MANIFEST_URL
$env:DEV_HELPER_BUILD_VERSION = $HelperVersion
$env:DEV_HELPER_UPDATE_MANIFEST_URL = $ManifestUrl
Push-Location $HelperRepository
try {
    & npm.cmd run desktop:wails:package
    if ($LASTEXITCODE -ne 0) { throw 'Dev Helper desktop package failed.' }
} finally {
    Pop-Location
    if ($null -eq $previousHelperBuildVersion) {
        Remove-Item Env:DEV_HELPER_BUILD_VERSION -ErrorAction SilentlyContinue
    } else {
        $env:DEV_HELPER_BUILD_VERSION = $previousHelperBuildVersion
    }
	if ($null -eq $previousHelperManifestUrl) {
		Remove-Item Env:DEV_HELPER_UPDATE_MANIFEST_URL -ErrorAction SilentlyContinue
	} else {
		$env:DEV_HELPER_UPDATE_MANIFEST_URL = $previousHelperManifestUrl
	}
}
$helperDesktopPackage = Join-Path $HelperRepository 'dist-wails'
foreach ($requiredHelperFile in @('Dev Helper.exe', 'runtime\helper-backend.exe', 'resources\build-info.json')) {
    if (-not (Test-Path -LiteralPath (Join-Path $helperDesktopPackage $requiredHelperFile) -PathType Leaf)) {
        throw "Dev Helper Wails package is missing: $requiredHelperFile"
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $helperDesktopPackage 'Dev Helper.exe'))) {
    throw 'Dev Helper packaged executable was not produced.'
}
Copy-Item -Path (Join-Path $helperDesktopPackage '*') -Destination $helperPayload -Recurse -Force

$managerInstallerName = "brain2-manager-$ManagerVersion-windows-x64.exe"
$launcherInstallerName = "meow-launcher-$LauncherVersion-windows-x64.exe"
$helperInstallerName = "dev-helper-$HelperVersion-windows-x64.exe"
$managerInstaller = Join-Path $artifactsRoot $managerInstallerName
$launcherInstaller = Join-Path $artifactsRoot $launcherInstallerName
$helperInstaller = Join-Path $artifactsRoot $helperInstallerName

& $packageBuilder --runner $installerRunner --payload $managerPayload --output $managerInstaller --product brain2-manager --version $ManagerVersion --entrypoint 'Brain2 Manager.exe' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Brain2 Manager installer package failed.' }
& $packageBuilder --runner $installerRunner --payload $helperPayload --output $helperInstaller --product dev-helper --version $HelperVersion --entrypoint 'Dev Helper.exe' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Dev Helper installer package failed.' }
& $packageBuilder --runner $installerRunner --payload $launcherPayload --output $launcherInstaller --product meow-launcher --version $LauncherVersion --entrypoint 'Meow Launcher.exe' --self-update | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Meow Launcher one-shot installer package failed.' }

$publishedAt = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
function New-ReleaseRecord {
    param(
        [string]$ProductId,
        [string]$Version,
        [string]$Notes,
        [string]$InstallerName,
        [string]$InstallerPath,
        [string]$InstallerType,
        [string]$DirectoryName,
        [string]$ExecutablePath,
        [string]$MinimumSupportedVersion,
        [string]$RollbackStrategy
    )
    return [ordered]@{
        product_id = $ProductId
        channel = 'preview'
        version = $Version
        published_at = $publishedAt
        release_notes = $Notes
        download_url = ($BaseUrl.TrimEnd('/') + '/' + [Uri]::EscapeDataString($InstallerName))
        installer_type = $InstallerType
        access_policy = 'public'
        install_scope = 'per-user'
        install_directory_name = $DirectoryName
        executable_relative_path = $ExecutablePath
        sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $InstallerPath).Hash.ToLowerInvariant()
        signature = [ordered]@{ method = 'authenticode'; required = $false }
        minimum_launcher_version = '0.6.6'
        minimum_supported_version = $MinimumSupportedVersion
        mandatory = $false
        data_migration_version = 'none'
        rollback = [ordered]@{ strategy = $RollbackStrategy; previous_version_supported = $true; backup_required = $true }
    }
}

$manifest = [ordered]@{
    schema_version = 'meow-update-manifest/v1'
    manifest_url = $ManifestUrl
    generated_at = $publishedAt
    products = @(
        (New-ReleaseRecord -ProductId 'brain2-manager' -Version $ManagerVersion -Notes $ManagerReleaseNotes -InstallerName $managerInstallerName -InstallerPath $managerInstaller -InstallerType 'meow-bundle' -DirectoryName 'Brain2 Manager' -ExecutablePath 'Brain2 Manager.exe' -MinimumSupportedVersion '0.6.6' -RollbackStrategy 'bundle-transaction'),
        (New-ReleaseRecord -ProductId 'dev-helper' -Version $HelperVersion -Notes $HelperReleaseNotes -InstallerName $helperInstallerName -InstallerPath $helperInstaller -InstallerType 'meow-bundle' -DirectoryName 'Dev Helper' -ExecutablePath 'Dev Helper.exe' -MinimumSupportedVersion '0.3.0-p1' -RollbackStrategy 'bundle-transaction'),
        (New-ReleaseRecord -ProductId 'meow-launcher' -Version $LauncherVersion -Notes $LauncherReleaseNotes -InstallerName $launcherInstallerName -InstallerPath $launcherInstaller -InstallerType 'meow-bundle-one-shot' -DirectoryName 'Meow Launcher' -ExecutablePath 'Meow Launcher.exe' -MinimumSupportedVersion '0.6.6' -RollbackStrategy 'one-shot-bundle-restore')
    )
}
$manifestPath = Join-Path $releaseRoot 'manifest-v1.json'
$manifestJson = $manifest | ConvertTo-Json -Depth 8
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($manifestPath, $manifestJson + [Environment]::NewLine, $utf8WithoutBom)

[ordered]@{
    ok = $true
    release_root = $releaseRoot
    manifest = $manifestPath
    artifacts = @($managerInstaller, $helperInstaller, $launcherInstaller)
    signed = $false
    published = $false
} | ConvertTo-Json -Depth 4
