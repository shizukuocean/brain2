param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot,

    [switch]$Publish,

    [switch]$VerifyRemoteArtifacts,

    [string]$RemoteManifestUrl = ''
)

$ErrorActionPreference = 'Stop'
$ReleaseRoot = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$manifestPath = Join-Path $ReleaseRoot 'manifest-v1.json'
$artifactsRoot = Join-Path $ReleaseRoot 'artifacts'

function Get-RegularFile {
    param([string]$Path, [string]$Label)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Label is missing: $Path"
    }
    $item = Get-Item -Force -LiteralPath $Path
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "$Label cannot be a symbolic link or reparse point: $Path"
    }
    return $item
}

Get-RegularFile -Path $manifestPath -Label 'Update manifest' | Out-Null
$manifest = Get-Content -Raw -Encoding UTF8 -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.schema_version -ne 'meow-update-manifest/v1') {
    throw 'Update manifest schema_version is invalid.'
}
$declaredManifestUri = $null
if (-not [Uri]::TryCreate([string]$manifest.manifest_url, [UriKind]::Absolute, [ref]$declaredManifestUri) -or $declaredManifestUri.Scheme -ne 'https') {
    throw 'Update manifest manifest_url must be an absolute HTTPS URL.'
}
if (-not [string]::IsNullOrEmpty($declaredManifestUri.UserInfo)) {
    throw 'Update manifest manifest_url cannot embed credentials or tokens.'
}
if ($Publish -and ($declaredManifestUri.Host -eq 'example.invalid' -or $declaredManifestUri.Host.EndsWith('.invalid'))) {
    throw 'Publish gate rejected the placeholder manifest host.'
}

$products = @($manifest.products)
if ($products.Count -ne 3) {
    throw 'The release must contain exactly three product records.'
}
$expectedTypes = @{
    'brain2-manager' = 'meow-bundle'
    'dev-helper' = 'meow-bundle'
    'meow-launcher' = 'meow-bundle-one-shot'
}
$seen = @{}
$results = @()

foreach ($release in $products) {
    $productId = [string]$release.product_id
    if (-not $expectedTypes.ContainsKey($productId) -or $seen.ContainsKey($productId)) {
        throw "Unexpected or duplicate product_id: $productId"
    }
    $seen[$productId] = $true
    if ($release.installer_type -ne $expectedTypes[$productId]) {
        throw "Unexpected installer_type for $productId."
    }
    if ($release.channel -notin @('preview', 'stable')) {
        throw "Unsupported release channel for $productId."
    }
    if ([string]$release.sha256 -notmatch '^[a-f0-9]{64}$') {
        throw "Invalid SHA-256 for $productId."
    }
    foreach ($versionField in @('version', 'minimum_launcher_version', 'minimum_supported_version')) {
        if ([string]$release.$versionField -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$') {
            throw "Invalid $versionField for $productId."
        }
    }

    $downloadUri = $null
    if (-not [Uri]::TryCreate([string]$release.download_url, [UriKind]::Absolute, [ref]$downloadUri) -or $downloadUri.Scheme -ne 'https') {
        throw "download_url must be absolute HTTPS for $productId."
    }
    if (-not [string]::IsNullOrEmpty($downloadUri.UserInfo)) {
        throw "download_url cannot embed credentials or tokens for $productId."
    }
    if ($Publish -and ($downloadUri.Host -eq 'example.invalid' -or $downloadUri.Host.EndsWith('.invalid'))) {
        throw "Publish gate rejected the placeholder download host for $productId."
    }

    $artifactName = [Uri]::UnescapeDataString($downloadUri.Segments[-1])
    if ([IO.Path]::GetFileName($artifactName) -ne $artifactName -or [string]::IsNullOrWhiteSpace($artifactName)) {
        throw "download_url does not identify one artifact file for $productId."
    }
    $artifactPath = Join-Path $artifactsRoot $artifactName
    Get-RegularFile -Path $artifactPath -Label "$productId artifact" | Out-Null
    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $artifactPath).Hash.ToLowerInvariant()
    if ($actualHash -ne [string]$release.sha256) {
        throw "Artifact hash mismatch for $productId."
    }

    $signatureRequired = [bool]$release.signature.required
    if ($release.channel -eq 'stable' -and (-not $signatureRequired -or $release.signature.method -ne 'authenticode')) {
        throw "Stable release must require Authenticode for $productId."
    }
    $signatureStatus = 'not-required'
    if ($signatureRequired) {
        $publisher = [string]$release.signature.publisher
        if ([string]::IsNullOrWhiteSpace($publisher)) {
            throw "Signature publisher is missing for $productId."
        }
        $signature = Get-AuthenticodeSignature -LiteralPath $artifactPath
        if ($signature.Status -ne [Management.Automation.SignatureStatus]::Valid -or $null -eq $signature.SignerCertificate) {
            throw "Authenticode signature is not valid for ${productId}: $($signature.Status)"
        }
        if ($signature.SignerCertificate.Subject -notlike "*$publisher*") {
            throw "Authenticode publisher does not match the manifest for $productId."
        }
        $signatureStatus = 'valid'
    }

    $results += [ordered]@{
        product_id = $productId
        version = [string]$release.version
        channel = [string]$release.channel
        artifact = $artifactPath
        sha256 = $actualHash
        signature = $signatureStatus
    }
}

$remoteVerified = $false
if (-not [string]::IsNullOrWhiteSpace($RemoteManifestUrl)) {
    if (-not $Publish) { throw 'Remote verification requires the -Publish gate.' }
    $remoteUri = $null
    if (-not [Uri]::TryCreate($RemoteManifestUrl, [UriKind]::Absolute, [ref]$remoteUri) -or $remoteUri.Scheme -ne 'https') {
        throw 'RemoteManifestUrl must be an absolute HTTPS URL.'
    }
    if (-not [string]::IsNullOrEmpty($remoteUri.UserInfo)) {
        throw 'RemoteManifestUrl cannot embed credentials or tokens.'
    }
    if ($remoteUri.Host -eq 'example.invalid' -or $remoteUri.Host.EndsWith('.invalid')) {
        throw 'Remote verification rejected the placeholder manifest host.'
    }
    if ($remoteUri.AbsoluteUri -ne $declaredManifestUri.AbsoluteUri) {
        throw 'RemoteManifestUrl must exactly match manifest_url declared by the release.'
    }
    $remoteRoot = Join-Path ([IO.Path]::GetTempPath()) ('meow-release-remote-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force -Path $remoteRoot | Out-Null
    try {
        $remoteManifestPath = Join-Path $remoteRoot 'manifest-v1.json'
        Invoke-WebRequest -UseBasicParsing -Uri $remoteUri.AbsoluteUri -OutFile $remoteManifestPath -TimeoutSec 60
        if ((Get-FileHash -Algorithm SHA256 -LiteralPath $remoteManifestPath).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash) {
            throw 'The public manifest does not match the locally audited manifest.'
        }
        if ($VerifyRemoteArtifacts) {
            foreach ($release in $products) {
                $artifactUri = [Uri][string]$release.download_url
                $artifactName = [Uri]::UnescapeDataString($artifactUri.Segments[-1])
                $remoteArtifactPath = Join-Path $remoteRoot $artifactName
                & "$env:SystemRoot\System32\curl.exe" --fail --location --silent --show-error --proto '=https' --proto-redir '=https' --retry 5 --retry-all-errors --continue-at - --output $remoteArtifactPath $artifactUri.AbsoluteUri
                if ($LASTEXITCODE -ne 0) { throw "Public artifact download failed for $($release.product_id)." }
                $remoteHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $remoteArtifactPath).Hash.ToLowerInvariant()
                if ($remoteHash -ne [string]$release.sha256) {
                    throw "Public artifact hash mismatch for $($release.product_id)."
                }
            }
        }
        $remoteVerified = $true
    } finally {
        Remove-Item -Recurse -Force -LiteralPath $remoteRoot -ErrorAction SilentlyContinue
    }
}

[ordered]@{
    ok = $true
    publish_gate = [bool]$Publish
    release_root = $ReleaseRoot
    manifest = $manifestPath
    remote_manifest_url = $RemoteManifestUrl
    remote_verified = $remoteVerified
    remote_artifacts_verified = [bool]$VerifyRemoteArtifacts
    products = $results
} | ConvertTo-Json -Depth 5
