param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot
)

$ErrorActionPreference = 'Stop'
$ReleaseRoot = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$manifestPath = Join-Path $ReleaseRoot 'manifest-v1.json'
$artifactsRoot = Join-Path $ReleaseRoot 'artifacts'
$payloadsRoot = Join-Path $ReleaseRoot 'payloads'
$toolsRoot = Join-Path $ReleaseRoot 'tools'
if (-not (Test-Path -LiteralPath $manifestPath)) { throw 'Preview manifest is missing.' }
$manifest = Get-Content -Raw -Encoding UTF8 -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.schema_version -ne 'meow-update-manifest/v1' -or $manifest.products.Count -ne 3) {
    throw 'Preview manifest identity is invalid.'
}
$manifestUri = $null
if (-not [Uri]::TryCreate([string]$manifest.manifest_url, [UriKind]::Absolute, [ref]$manifestUri) -or $manifestUri.Scheme -ne 'https' -or -not [string]::IsNullOrEmpty($manifestUri.UserInfo)) {
    throw 'Preview manifest_url must be an absolute credential-free HTTPS URL.'
}

function Get-Release {
    param([string]$ProductId)
    $release = @($manifest.products | Where-Object { $_.product_id -eq $ProductId })
    if ($release.Count -ne 1) { throw "Manifest must contain exactly one $ProductId release." }
    return $release[0]
}

function Get-ArtifactPath {
    param($Release)
    $name = [Uri]::UnescapeDataString(([Uri]$Release.download_url).Segments[-1])
    $path = Join-Path $artifactsRoot $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Artifact missing: $name" }
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
    if ($actual -ne $Release.sha256) { throw "Artifact hash mismatch: $name" }
    return $path
}

function Quote-Argument {
    param([string]$Value)
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Invoke-HiddenProcess {
    param([string]$FilePath, [string[]]$Arguments)
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $FilePath
    $startInfo.Arguments = (($Arguments | ForEach-Object { Quote-Argument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.EnvironmentVariables['MEOW_LAUNCHER_MACHINE_ROOT'] = $script:MeowTestMachineRoot
    $startInfo.EnvironmentVariables['MEOW_DESKTOP_SHORTCUT_TEST_ROOT'] = $script:MeowTestDesktopRoot
    $process = [System.Diagnostics.Process]::Start($startInfo)
    if ($null -eq $process) { throw "Could not start $FilePath" }
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw "$FilePath exited with code $($process.ExitCode)" }
}

function Invoke-MeowInstaller {
    param($Release, [string]$ArtifactPath, [string]$TargetExecutable, [string]$ReceiptPath, [int]$WaitPID = 0, [switch]$RestartLauncher)
    $arguments = @(
        '--meow-product', [string]$Release.product_id,
        '--meow-version', [string]$Release.version,
        '--meow-transaction', ('e2e-' + [Guid]::NewGuid().ToString('N')),
        '--meow-receipt', $ReceiptPath,
        '--meow-target-executable', $TargetExecutable
    )
    if ($WaitPID -gt 0) { $arguments += @('--meow-wait-pid', [string]$WaitPID) }
    if ($RestartLauncher) { $arguments += @('--meow-return', 'restart', '--meow-restart-launcher') }
    Invoke-HiddenProcess -FilePath $ArtifactPath -Arguments $arguments
    $receipt = Get-Content -Raw -Encoding UTF8 -LiteralPath $ReceiptPath | ConvertFrom-Json
    if ($receipt.status -ne 'installed' -or $receipt.product_id -ne $Release.product_id -or $receipt.version -ne $Release.version) {
        throw "Installer receipt failed for $($Release.product_id)."
    }
    return $receipt
}

$testRoot = Join-Path $ReleaseRoot (Join-Path 'e2e' ([DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss')))
$script:MeowTestMachineRoot = Join-Path $testRoot 'machine'
$script:MeowTestDesktopRoot = Join-Path $script:MeowTestMachineRoot 'desktop'
New-Item -ItemType Directory -Force -Path $testRoot | Out-Null

function Get-ShortcutTarget {
    param([string]$ShortcutPath)
    if (-not (Test-Path -LiteralPath $ShortcutPath -PathType Leaf)) { throw "Desktop shortcut missing: $ShortcutPath" }
    $shell = New-Object -ComObject WScript.Shell
    return [string]$shell.CreateShortcut($ShortcutPath).TargetPath
}

$managerRelease = Get-Release 'brain2-manager'
$managerArtifact = Get-ArtifactPath $managerRelease
$managerTarget = Join-Path $testRoot 'Brain2 Manager\Brain2 Manager.exe'
$managerReceiptPath = Join-Path $testRoot 'receipts\manager.json'
$managerReceipt = Invoke-MeowInstaller -Release $managerRelease -ArtifactPath $managerArtifact -TargetExecutable $managerTarget -ReceiptPath $managerReceiptPath
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $managerTarget).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $payloadsRoot 'brain2-manager\Brain2 Manager.exe')).Hash) {
    throw 'Installed Brain2 Manager does not match the packaged payload.'
}
if ((Get-ShortcutTarget (Join-Path $script:MeowTestDesktopRoot 'Brain2 Manager.lnk')) -ne $managerTarget) {
    throw 'Installed Brain2 Manager desktop shortcut points to the wrong executable.'
}

$helperRelease = Get-Release 'dev-helper'
$helperArtifact = Get-ArtifactPath $helperRelease
$helperBuildInfoPath = Join-Path $payloadsRoot 'dev-helper\resources\build-info.json'
$helperBuildInfo = Get-Content -Raw -Encoding UTF8 -LiteralPath $helperBuildInfoPath | ConvertFrom-Json
if ($helperBuildInfo.schema_version -ne 'dev-helper-build-info/v1' -or $helperBuildInfo.version -ne $helperRelease.version -or $helperBuildInfo.update_manifest_url -ne $manifest.manifest_url) {
    throw 'Dev Helper packaged build information does not match the release.'
}
$helperTarget = Join-Path $testRoot 'Dev Helper\Dev Helper.exe'
$helperReceiptPath = Join-Path $testRoot 'receipts\helper.json'
$helperReceipt = Invoke-MeowInstaller -Release $helperRelease -ArtifactPath $helperArtifact -TargetExecutable $helperTarget -ReceiptPath $helperReceiptPath
foreach ($relative in @('Dev Helper.exe', 'runtime\helper-backend.exe', 'resources\build-info.json')) {
    $installed = Join-Path (Split-Path -Parent $helperTarget) $relative
    $payload = Join-Path (Join-Path $payloadsRoot 'dev-helper') $relative
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $installed).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $payload).Hash) {
        throw "Installed Dev Helper file differs: $relative"
    }
}
if ((Get-ShortcutTarget (Join-Path $script:MeowTestDesktopRoot 'Dev Helper.lnk')) -ne $helperTarget) {
    throw 'Installed Dev Helper desktop shortcut points to the wrong executable.'
}
$helperSmokeReceipt = Join-Path $testRoot 'receipts\helper-smoke.json'
$helperSmokeProfile = Join-Path $testRoot 'helper-smoke-profile'
Invoke-HiddenProcess -FilePath $helperTarget -Arguments @(
    ('--user-data-dir=' + $helperSmokeProfile),
    '--smoke-test',
    '--smoke-receipt', $helperSmokeReceipt,
    '--data-dir', (Join-Path $testRoot 'helper-smoke-data')
)
$helperSmoke = Get-Content -Raw -Encoding UTF8 -LiteralPath $helperSmokeReceipt | ConvertFrom-Json
if (-not $helperSmoke.ok -or -not $helperSmoke.packaged -or $helperSmoke.applicationVersion -ne $helperRelease.version -or -not $helperSmoke.updateSourceConfigured) {
    throw 'Installed Dev Helper did not load its packaged version/update source.'
}

$selfPayload = Join-Path $testRoot 'self-update-payload'
New-Item -ItemType Directory -Force -Path $selfPayload | Out-Null
$selfTargetPayload = Join-Path $selfPayload 'Meow Launcher.exe'
Copy-Item -LiteralPath (Join-Path $toolsRoot 'meow-package-builder.exe') -Destination $selfTargetPayload
$selfArtifact = Join-Path $testRoot 'meow-launcher-self-test.exe'
$selfVersion = '0.0.0-e2e'
Invoke-HiddenProcess -FilePath (Join-Path $toolsRoot 'meow-package-builder.exe') -Arguments @(
    '--runner', (Join-Path $toolsRoot 'meow-product-installer.exe'),
    '--payload', $selfPayload,
    '--output', $selfArtifact,
    '--product', 'meow-launcher',
    '--version', $selfVersion,
    '--entrypoint', 'Meow Launcher.exe',
    '--self-update'
)
$initialLauncherRoot = Join-Path $script:MeowTestMachineRoot 'Meow System\Launcher'
$initialMachineRoot = $script:MeowTestMachineRoot
New-Item -ItemType Directory -Force -Path $initialLauncherRoot | Out-Null
Invoke-HiddenProcess -FilePath $selfArtifact -Arguments @('--meow-initial-directory', $initialLauncherRoot)
$initialTarget = Join-Path $initialLauncherRoot 'Meow Launcher.exe'
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $initialTarget).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $selfTargetPayload).Hash) {
    throw 'Launcher first installation target does not match its packaged payload.'
}
$initialReceipts = @(Get-ChildItem -File -LiteralPath (Join-Path $initialMachineRoot 'updates\receipts') -Filter 'setup-*.json')
if ($initialReceipts.Count -ne 1) { throw 'Launcher first installation did not write exactly one setup receipt.' }
$initialReceipt = Get-Content -Raw -Encoding UTF8 -LiteralPath $initialReceipts[0].FullName | ConvertFrom-Json
if ($initialReceipt.status -ne 'installed' -or -not $initialReceipt.restarted) {
    throw 'Launcher first installation receipt did not confirm install and restart.'
}
if ((Get-ShortcutTarget (Join-Path $script:MeowTestDesktopRoot 'Meow Launcher.lnk')) -ne $initialTarget) {
    throw 'Initial Launcher desktop shortcut points to the wrong executable.'
}
$selfRelease = [PSCustomObject]@{ product_id = 'meow-launcher'; version = $selfVersion }
$selfTarget = Join-Path $testRoot 'Meow Launcher\Meow Launcher.exe'
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $selfTarget) | Out-Null
[System.IO.File]::WriteAllText($selfTarget, 'old launcher placeholder')
$sleepProcess = Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-Command', 'Start-Sleep -Milliseconds 600' -PassThru -WindowStyle Hidden
$selfReceiptPath = Join-Path $testRoot 'receipts\launcher-self.json'
$selfReceipt = Invoke-MeowInstaller -Release $selfRelease -ArtifactPath $selfArtifact -TargetExecutable $selfTarget -ReceiptPath $selfReceiptPath -WaitPID $sleepProcess.Id -RestartLauncher
if (-not $selfReceipt.restarted) { throw 'Launcher self-update receipt did not confirm restart.' }
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $selfTarget).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $selfTargetPayload).Hash) {
    throw 'Launcher self-update target does not match its packaged payload.'
}
if ((Get-ShortcutTarget (Join-Path $script:MeowTestDesktopRoot 'Meow Launcher.lnk')) -ne $selfTarget) {
    throw 'Launcher self-update did not refresh the desktop shortcut target.'
}

[ordered]@{
    ok = $true
    test_root = $testRoot
    manager = $managerReceipt.status
    helper = $helperReceipt.status
    launcher_first_install = $initialReceipt.status
    launcher_self_update = $selfReceipt.status
    launcher_restarted = [bool]$selfReceipt.restarted
    desktop_shortcuts = $true
    user_data_touched = $false
} | ConvertTo-Json
