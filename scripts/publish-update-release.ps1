param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot,

    [Parameter(Mandatory = $true)]
    [string]$Tag,

    [string]$Repository = 'shizukuocean/meow-releases',

    [switch]$Publish,

    [string]$ConfirmTag = '',

    [int]$PagesTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
$ReleaseRoot = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$publishRoot = Join-Path $ReleaseRoot 'publish'
$manifestPath = Join-Path $publishRoot 'manifest-v1.json'
$checksumsPath = Join-Path $publishRoot 'SHA256SUMS.txt'
$channelPath = 'channels/preview/manifest-v1.json'

function Require-RegularFile {
    param([string]$Path, [string]$Label)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "$Label is missing: $Path" }
    $item = Get-Item -Force -LiteralPath $Path
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "$Label cannot be a link: $Path" }
    return $item
}

function Invoke-GitHub {
    param([string[]]$Arguments, [switch]$AllowFailure)
	$previousPreference = $ErrorActionPreference
	try {
		$ErrorActionPreference = 'Continue'
		$output = & gh @Arguments 2>&1
		$exitCode = $LASTEXITCODE
	} finally {
		$ErrorActionPreference = $previousPreference
	}
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "gh $($Arguments -join ' ') failed: $($output -join [Environment]::NewLine)"
    }
    return [pscustomobject]@{ ExitCode = $exitCode; Output = @($output) }
}

function Update-GitHubContent {
    param([string]$Path, [string]$Message, [string]$Content, [string]$SHA)
    $requestPath = Join-Path ([IO.Path]::GetTempPath()) ('meow-github-content-' + [Guid]::NewGuid().ToString('N') + '.json')
    try {
        $request = @{ message = $Message; content = $Content; sha = $SHA; branch = 'main' } | ConvertTo-Json -Compress
        [IO.File]::WriteAllText($requestPath, $request, (New-Object Text.UTF8Encoding($false)))
        Invoke-GitHub -Arguments @('api', '--method', 'PUT', "repos/$Repository/contents/$Path", '--input', $requestPath) | Out-Null
    } finally {
        Remove-Item -Force -LiteralPath $requestPath -ErrorAction SilentlyContinue
    }
}

function Assert-PublishedAssetReachable {
    param([string]$Url)
    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if ($null -eq $curl) { throw 'curl.exe is required to verify anonymous release asset access.' }
    $previousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $curl.Source --fail --silent --show-error --location --head --retry 4 --retry-delay 2 --retry-all-errors --connect-timeout 15 --max-time 120 --output NUL $Url 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($exitCode -ne 0) {
        throw "Published asset is not reachable after retries: $Url`n$($output -join [Environment]::NewLine)"
    }
}

if ($Repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw 'Repository must be owner/name.' }
if ($Tag -notmatch '^updates-preview-[0-9]{4}-[0-9]{2}-[0-9]{2}\.[0-9]+$') { throw 'Tag does not match the Preview release convention.' }
if ($PagesTimeoutSeconds -lt 30 -or $PagesTimeoutSeconds -gt 900) { throw 'PagesTimeoutSeconds must be between 30 and 900.' }

Require-RegularFile -Path $manifestPath -Label 'Publish manifest' | Out-Null
Require-RegularFile -Path $checksumsPath -Label 'Publish checksums' | Out-Null
$manifest = Get-Content -Raw -Encoding UTF8 -LiteralPath $manifestPath | ConvertFrom-Json
$expectedBase = "https://github.com/$Repository/releases/download/$Tag/"
$artifacts = @()
foreach ($product in @($manifest.products)) {
    $download = [Uri][string]$product.download_url
    if (-not $download.AbsoluteUri.StartsWith($expectedBase, [StringComparison]::Ordinal)) {
        throw "Manifest download URL does not use tag $Tag for $($product.product_id)."
    }
    $name = [Uri]::UnescapeDataString($download.Segments[-1])
    $path = Join-Path $publishRoot $name
    Require-RegularFile -Path $path -Label "$($product.product_id) artifact" | Out-Null
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant() -ne [string]$product.sha256) {
        throw "Artifact hash mismatch for $($product.product_id)."
    }
    $artifacts += $path
}

$allowed = @('manifest-v1.json', 'SHA256SUMS.txt') + @($artifacts | ForEach-Object { Split-Path -Leaf $_ })
$unexpected = @(Get-ChildItem -LiteralPath $publishRoot -File | Where-Object { $_.Name -notin $allowed })
if ($unexpected.Count -ne 0 -or @(Get-ChildItem -LiteralPath $publishRoot -File).Count -ne 5) {
    throw 'Publish directory must contain exactly the manifest, checksums and three declared installers.'
}

$repo = (Invoke-GitHub -Arguments @('repo', 'view', $Repository, '--json', 'visibility,url')).Output -join "`n" | ConvertFrom-Json
if ($repo.visibility -ne 'PUBLIC') { throw 'Download repository must be public.' }
$existing = Invoke-GitHub -Arguments @('release', 'view', $Tag, '--repo', $Repository, '--json', 'tagName') -AllowFailure
if ($existing.ExitCode -eq 0) { throw "Release tag already exists: $Tag" }
& (Join-Path $PSScriptRoot 'audit-update-release.ps1') -ReleaseRoot $ReleaseRoot -Publish | Out-Null

$summary = [ordered]@{
    ok = $true
    mode = if ($Publish) { 'publish' } else { 'preflight' }
    repository = $Repository
    tag = $Tag
    release_root = $ReleaseRoot
    publish_root = $publishRoot
    channel_path = $channelPath
    assets = @($artifacts | ForEach-Object { Split-Path -Leaf $_ }) + 'SHA256SUMS.txt'
}
if (-not $Publish) {
    $summary.published = $false
    $summary | ConvertTo-Json -Depth 5
    return
}
if ($ConfirmTag -cne $Tag) { throw 'Publishing requires -ConfirmTag to exactly match -Tag.' }

$oldChannel = Invoke-GitHub -Arguments @('api', "repos/$Repository/contents/$channelPath")
$oldChannelRecord = ($oldChannel.Output -join "`n") | ConvertFrom-Json
$oldChannelBytes = [Convert]::FromBase64String(([string]$oldChannelRecord.content -replace '\s', ''))
$oldChannelTemp = Join-Path ([IO.Path]::GetTempPath()) ('meow-channel-old-' + [Guid]::NewGuid().ToString('N') + '.json')
[IO.File]::WriteAllBytes($oldChannelTemp, $oldChannelBytes)
$channelChanged = $false
$releaseCreated = $false
try {
	$launcherReleases = @($manifest.products | Where-Object { $_.product_id -eq 'meow-launcher' })
	$launcherRelease = $launcherReleases[0]
	$title = 'Three-product update Preview ' + [string]$launcherRelease.version
    Invoke-GitHub -Arguments (@('release', 'create', $Tag, '--repo', $Repository, '--prerelease', '--title', $title, '--notes', 'Dev Helper, Brain2 Manager, and Meow Launcher update MVP Preview. Launcher owns installation, verification, and updates.') + $artifacts + $checksumsPath) | Out-Null
    $releaseCreated = $true

    foreach ($product in @($manifest.products)) {
        $url = [string]$product.download_url
        Assert-PublishedAssetReachable -Url $url
    }

    $newContent = [Convert]::ToBase64String([IO.File]::ReadAllBytes($manifestPath))
    $message = "publish $Tag preview channel manifest"
	Update-GitHubContent -Path $channelPath -Message $message -Content $newContent -SHA ([string]$oldChannelRecord.sha)
    $channelChanged = $true

    $deadline = (Get-Date).AddSeconds($PagesTimeoutSeconds)
    $remoteManifest = [Uri][string]$manifest.manifest_url
    $localHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash
    do {
        Start-Sleep -Seconds 3
        $probe = Join-Path ([IO.Path]::GetTempPath()) ('meow-channel-probe-' + [Guid]::NewGuid().ToString('N') + '.json')
        try {
            Invoke-WebRequest -UseBasicParsing -Uri ($remoteManifest.AbsoluteUri + '?published=' + [Uri]::EscapeDataString($Tag)) -OutFile $probe -TimeoutSec 30
            $ready = (Get-FileHash -Algorithm SHA256 -LiteralPath $probe).Hash -eq $localHash
        } catch { $ready = $false } finally { Remove-Item -Force -LiteralPath $probe -ErrorAction SilentlyContinue }
    } until ($ready -or (Get-Date) -ge $deadline)
    if (-not $ready) { throw 'GitHub Pages did not publish the exact channel manifest before timeout.' }

    & (Join-Path $PSScriptRoot 'audit-update-release.ps1') -ReleaseRoot $ReleaseRoot -Publish -RemoteManifestUrl $remoteManifest.AbsoluteUri -VerifyRemoteArtifacts | Out-Null
    $summary.published = $true
    $summary.remote_verified = $true
    $summary.release_url = "https://github.com/$Repository/releases/tag/$Tag"
    $summary | ConvertTo-Json -Depth 5
} catch {
	$canDeleteCreatedRelease = -not $channelChanged
    if ($channelChanged) {
        try {
			$currentResponse = Invoke-GitHub -Arguments @('api', "repos/$Repository/contents/$channelPath")
			$current = ($currentResponse.Output -join "`n") | ConvertFrom-Json
            $restoreContent = [Convert]::ToBase64String([IO.File]::ReadAllBytes($oldChannelTemp))
			Update-GitHubContent -Path $channelPath -Message "rollback failed $Tag preview channel" -Content $restoreContent -SHA ([string]$current.sha)
			$oldHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $oldChannelTemp).Hash
			$rollbackDeadline = (Get-Date).AddSeconds($PagesTimeoutSeconds)
			do {
				Start-Sleep -Seconds 3
				$rollbackProbe = Join-Path ([IO.Path]::GetTempPath()) ('meow-channel-rollback-' + [Guid]::NewGuid().ToString('N') + '.json')
				try {
					Invoke-WebRequest -UseBasicParsing -Uri ([string]$manifest.manifest_url + '?rollback=' + [Uri]::EscapeDataString($Tag)) -OutFile $rollbackProbe -TimeoutSec 30
					$canDeleteCreatedRelease = (Get-FileHash -Algorithm SHA256 -LiteralPath $rollbackProbe).Hash -eq $oldHash
				} catch { $canDeleteCreatedRelease = $false } finally { Remove-Item -Force -LiteralPath $rollbackProbe -ErrorAction SilentlyContinue }
			} until ($canDeleteCreatedRelease -or (Get-Date) -ge $rollbackDeadline)
			if (-not $canDeleteCreatedRelease) { Write-Warning 'The previous channel manifest was not visible before the rollback timeout.' }
		} catch {
			$canDeleteCreatedRelease = $false
			Write-Warning "Channel rollback also failed: $($_.Exception.Message)"
		}
    }
	if ($releaseCreated -and $canDeleteCreatedRelease) {
		try {
			Invoke-GitHub -Arguments @('release', 'delete', $Tag, '--repo', $Repository, '--cleanup-tag', '--yes') | Out-Null
		} catch {
			Write-Warning "Failed to remove the transaction-created $Tag pre-release: $($_.Exception.Message)"
		}
	} elseif ($releaseCreated) {
		Write-Warning "The transaction-created $Tag pre-release was preserved because channel rollback is not publicly confirmed."
	}
    throw
} finally {
    Remove-Item -Force -LiteralPath $oldChannelTemp -ErrorAction SilentlyContinue
}
