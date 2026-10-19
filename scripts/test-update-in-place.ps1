param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot,

    [Parameter(Mandatory = $true)]
    [string]$PreviousManagerInstaller,

    [Parameter(Mandatory = $true)]
    [string]$PreviousHelperInstaller,

    [Parameter(Mandatory = $true)]
    [string]$PreviousLauncherInstaller
)

$ErrorActionPreference = 'Stop'
$ReleaseRoot = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$PreviousManagerInstaller = (Resolve-Path -LiteralPath $PreviousManagerInstaller).Path
$PreviousHelperInstaller = (Resolve-Path -LiteralPath $PreviousHelperInstaller).Path
$PreviousLauncherInstaller = (Resolve-Path -LiteralPath $PreviousLauncherInstaller).Path
$manifest = Get-Content -Raw -Encoding UTF8 -LiteralPath (Join-Path $ReleaseRoot 'manifest-v1.json') | ConvertFrom-Json
$artifactsRoot = Join-Path $ReleaseRoot 'artifacts'

function Get-InstallerVersion {
    param(
        [string]$InstallerPath,
        [string]$ProductPrefix
    )
    $name = [IO.Path]::GetFileName($InstallerPath)
    $pattern = '^' + [Regex]::Escape($ProductPrefix) + '-(?<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?)-windows-x64\.exe$'
    if ($name -notmatch $pattern) {
        throw "Previous installer filename does not expose a valid version: $name"
    }
    return [string]$Matches.version
}

$previousManagerVersion = Get-InstallerVersion $PreviousManagerInstaller 'brain2-manager'
$previousHelperVersion = Get-InstallerVersion $PreviousHelperInstaller 'dev-helper'
$previousLauncherVersion = Get-InstallerVersion $PreviousLauncherInstaller 'meow-launcher'

function Quote-Argument {
    param([string]$Value)
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Invoke-Installer {
    param(
        [string]$Installer,
        [string]$ProductId,
        [string]$Version,
        [string]$Target,
        [string]$Receipt
    )
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $Installer
    $arguments = @(
        '--meow-product', $ProductId,
        '--meow-version', $Version,
        '--meow-transaction', ('in-place-' + [Guid]::NewGuid().ToString('N')),
        '--meow-receipt', $Receipt,
        '--meow-target-executable', $Target
    )
    $startInfo.Arguments = (($arguments | ForEach-Object { Quote-Argument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.EnvironmentVariables['MEOW_LAUNCHER_MACHINE_ROOT'] = $script:MeowTestMachineRoot
    $startInfo.EnvironmentVariables['MEOW_DESKTOP_SHORTCUT_TEST_ROOT'] = $script:MeowTestDesktopRoot
    $process = [System.Diagnostics.Process]::Start($startInfo)
    if ($null -eq $process) { throw "Could not start $Installer" }
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw "$Installer exited with code $($process.ExitCode)" }
    $result = Get-Content -Raw -Encoding UTF8 -LiteralPath $Receipt | ConvertFrom-Json
    if ($result.status -ne 'installed' -or $result.product_id -ne $ProductId -or $result.version -ne $Version) {
        throw "Unexpected receipt for $ProductId $Version"
    }
    return $result
}

function Get-CurrentArtifact {
    param([string]$ProductId)
    $release = @($manifest.products | Where-Object { $_.product_id -eq $ProductId })
    if ($release.Count -ne 1) { throw "Missing current release for $ProductId" }
    $fileName = [Uri]::UnescapeDataString(([Uri]$release[0].download_url).Segments[-1])
    $path = Join-Path $artifactsRoot $fileName
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Missing current artifact: $path" }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant() -ne $release[0].sha256) {
        throw "Current artifact hash mismatch for $ProductId"
    }
    return [pscustomobject]@{ Release = $release[0]; Path = $path }
}

$testRoot = Join-Path $ReleaseRoot ('e2e-update-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss'))
$script:MeowTestMachineRoot = Join-Path $testRoot 'machine'
$script:MeowTestDesktopRoot = Join-Path $script:MeowTestMachineRoot 'desktop'
$receiptRoot = Join-Path $testRoot 'receipts'
New-Item -ItemType Directory -Force -Path $receiptRoot | Out-Null

function Get-ShortcutTarget {
    param([string]$ShortcutPath)
    if (-not (Test-Path -LiteralPath $ShortcutPath -PathType Leaf)) { throw "Desktop shortcut missing: $ShortcutPath" }
    $shell = New-Object -ComObject WScript.Shell
    return [string]$shell.CreateShortcut($ShortcutPath).TargetPath
}

function Wait-ExactProcess {
    param([string]$ExecutablePath, [int]$TimeoutSeconds = 15)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $match = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $ExecutablePath, [StringComparison]::OrdinalIgnoreCase) })
        if ($match.Count -eq 1) { return $match[0] }
        if ($match.Count -gt 1) { throw "More than one process is running from $ExecutablePath" }
        Start-Sleep -Milliseconds 200
    } until ((Get-Date) -ge $deadline)
    throw "Process did not start from $ExecutablePath"
}

function Stop-ExactProcess {
    param([string]$ExecutablePath)
    $matches = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $ExecutablePath, [StringComparison]::OrdinalIgnoreCase) })
    foreach ($match in $matches) { Stop-Process -Id $match.ProcessId -Force }
}

function Invoke-LauncherInstaller {
    param(
        [string]$Installer,
        [string]$Version,
        [string]$Target,
        [string]$Receipt,
        [string]$TransactionID,
        [int]$WaitPID,
        [switch]$StopWaitProcess
    )
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $Installer
    $arguments = @(
        '--meow-product', 'meow-launcher',
        '--meow-version', $Version,
        '--meow-transaction', $TransactionID,
        '--meow-receipt', $Receipt,
        '--meow-target-executable', $Target,
        '--meow-wait-pid', [string]$WaitPID,
        '--meow-return', 'restart',
        '--meow-restart-launcher'
    )
    $startInfo.Arguments = (($arguments | ForEach-Object { Quote-Argument $_ }) -join ' ')
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.EnvironmentVariables['MEOW_LAUNCHER_MACHINE_ROOT'] = $script:MeowTestMachineRoot
    $startInfo.EnvironmentVariables['MEOW_DESKTOP_SHORTCUT_TEST_ROOT'] = $script:MeowTestDesktopRoot
    $process = [System.Diagnostics.Process]::Start($startInfo)
    if ($null -eq $process) { throw "Could not start $Installer" }
    if ($StopWaitProcess) {
        Start-Sleep -Milliseconds 500
        if ($process.HasExited) { throw 'Launcher migration installer exited before the old Launcher stopped.' }
        Stop-Process -Id $WaitPID -Force
    }
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw "$Installer exited with code $($process.ExitCode)" }
    $result = Get-Content -Raw -Encoding UTF8 -LiteralPath $Receipt | ConvertFrom-Json
    if ($result.status -ne 'installed' -or $result.product_id -ne 'meow-launcher' -or $result.version -ne $Version -or -not $result.restarted) {
        throw "Unexpected Launcher receipt for $Version"
    }
    return $result
}

$managerCurrent = Get-CurrentArtifact 'brain2-manager'
$managerTarget = Join-Path $testRoot 'manager\Brain2 Manager.exe'
$managerOldReceipt = Join-Path $receiptRoot 'manager-old.json'
$managerNewReceipt = Join-Path $receiptRoot 'manager-new.json'
Invoke-Installer $PreviousManagerInstaller 'brain2-manager' $previousManagerVersion $managerTarget $managerOldReceipt | Out-Null
$managerOldHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $managerTarget).Hash
Set-Content -Encoding UTF8 -NoNewline -LiteralPath (Join-Path (Split-Path -Parent $managerTarget) 'user-note.txt') -Value 'preserve-manager'
Invoke-Installer $managerCurrent.Path 'brain2-manager' $managerCurrent.Release.version $managerTarget $managerNewReceipt | Out-Null
$managerNewHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $managerTarget).Hash
if ($managerOldHash -eq $managerNewHash) { throw 'Manager executable was not replaced.' }
if ((Get-Content -Raw -LiteralPath (Join-Path (Split-Path -Parent $managerTarget) 'user-note.txt')) -ne 'preserve-manager') { throw 'Manager user file was not preserved.' }
$managerShortcut = Join-Path $script:MeowTestDesktopRoot 'Brain2 Manager.lnk'
if ((Get-ShortcutTarget $managerShortcut) -ne $managerTarget) { throw 'Manager desktop shortcut does not point to the updated executable.' }

$helperCurrent = Get-CurrentArtifact 'dev-helper'
$helperTarget = Join-Path $testRoot 'helper\Dev Helper.exe'
$helperOldReceipt = Join-Path $receiptRoot 'helper-old.json'
$helperNewReceipt = Join-Path $receiptRoot 'helper-new.json'
Invoke-Installer $PreviousHelperInstaller 'dev-helper' $previousHelperVersion $helperTarget $helperOldReceipt | Out-Null
$helperOldHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $helperTarget).Hash
Set-Content -Encoding UTF8 -NoNewline -LiteralPath (Join-Path (Split-Path -Parent $helperTarget) 'user-note.txt') -Value 'preserve-helper'
Invoke-Installer $helperCurrent.Path 'dev-helper' $helperCurrent.Release.version $helperTarget $helperNewReceipt | Out-Null
$helperNewHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $helperTarget).Hash
if ($helperOldHash -eq $helperNewHash) { throw 'Helper executable was not replaced.' }
if ((Get-Content -Raw -LiteralPath (Join-Path (Split-Path -Parent $helperTarget) 'user-note.txt')) -ne 'preserve-helper') { throw 'Helper user file was not preserved.' }
$helperShortcut = Join-Path $script:MeowTestDesktopRoot 'Dev Helper.lnk'
if ((Get-ShortcutTarget $helperShortcut) -ne $helperTarget) { throw 'Helper desktop shortcut does not point to the updated executable.' }
$helperBuildInfo = Get-Content -Raw -Encoding UTF8 -LiteralPath (Join-Path (Split-Path -Parent $helperTarget) 'resources\build-info.json') | ConvertFrom-Json
if ($helperBuildInfo.version -ne $helperCurrent.Release.version -or $helperBuildInfo.update_manifest_url -ne $manifest.manifest_url) {
    throw 'Updated Helper build information does not match the release.'
}

$launcherCurrent = Get-CurrentArtifact 'meow-launcher'
$launcherLegacyRoot = Join-Path $script:MeowTestMachineRoot 'legacy-launcher'
$launcherLegacyTarget = Join-Path $launcherLegacyRoot 'Meow Launcher.exe'
$launcherCanonicalTarget = Join-Path $script:MeowTestMachineRoot 'Meow System\Launcher\Meow Launcher.exe'
$launcherOldReceipt = Join-Path $receiptRoot 'launcher-old.json'
$oldWait = Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile', '-Command', 'Start-Sleep -Milliseconds 700' -PassThru -WindowStyle Hidden
Invoke-LauncherInstaller -Installer $PreviousLauncherInstaller -Version $previousLauncherVersion -Target $launcherLegacyTarget `
    -Receipt $launcherOldReceipt -TransactionID ('legacy-install-' + [Guid]::NewGuid().ToString('N')) -WaitPID $oldWait.Id | Out-Null
$oldLauncherProcess = Wait-ExactProcess $launcherLegacyTarget
Set-Content -Encoding UTF8 -NoNewline -LiteralPath (Join-Path $launcherLegacyRoot 'user-note.txt') -Value 'preserve-launcher'

$launcherTransaction = 'legacy-migration-' + [Guid]::NewGuid().ToString('N')
$launcherNewReceipt = Join-Path $receiptRoot 'launcher-new.json'
$historyPath = Join-Path $script:MeowTestMachineRoot 'updates\state.json'
$history = [ordered]@{
    schema_version = 'meow-update-state/v1'
    products = [ordered]@{
        'meow-launcher' = [ordered]@{
            product_id = 'meow-launcher'
            installed_version = $previousLauncherVersion
            attempted_version = [string]$launcherCurrent.Release.version
            status = 'self_update_installer_started'
            message = 'legacy Launcher started the one-shot installer'
            transaction_id = $launcherTransaction
            installer_pid = 1
            receipt_path = $launcherNewReceipt
            executable_path = $launcherLegacyTarget
            updated_at = [DateTime]::UtcNow.ToString('o')
        }
    }
    updated_at = [DateTime]::UtcNow.ToString('o')
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $historyPath) | Out-Null
[IO.File]::WriteAllText($historyPath, ($history | ConvertTo-Json -Depth 8), (New-Object Text.UTF8Encoding($false)))
$launcherReceipt = Invoke-LauncherInstaller -Installer $launcherCurrent.Path -Version ([string]$launcherCurrent.Release.version) `
    -Target $launcherLegacyTarget -Receipt $launcherNewReceipt -TransactionID $launcherTransaction `
    -WaitPID $oldLauncherProcess.ProcessId -StopWaitProcess
if (-not [string]::Equals([string]$launcherReceipt.executable_path, $launcherCanonicalTarget, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Migrating Launcher installer did not report the canonical executable.'
}
$newLauncherProcess = Wait-ExactProcess $launcherCanonicalTarget
try {
    $cleanupDeadline = (Get-Date).AddSeconds(15)
    do {
        $historyResult = Get-Content -Raw -Encoding UTF8 -LiteralPath $historyPath | ConvertFrom-Json
        $migrationCompleted = $historyResult.products.'meow-launcher'.migration_status -eq 'completed'
        $legacyRemoved = -not (Test-Path -LiteralPath $launcherLegacyTarget)
        if ($migrationCompleted -and $legacyRemoved) { break }
        Start-Sleep -Milliseconds 200
    } until ((Get-Date) -ge $cleanupDeadline)
    if (-not $migrationCompleted -or -not $legacyRemoved) { throw 'Restarted Launcher did not complete legacy ownership cleanup.' }
    if ((Get-Content -Raw -LiteralPath (Join-Path $launcherLegacyRoot 'user-note.txt')) -ne 'preserve-launcher') { throw 'Launcher migration removed an unowned user file.' }
    if (-not (Test-Path -LiteralPath $launcherCanonicalTarget -PathType Leaf)) { throw 'Canonical Launcher executable is missing after migration.' }
    if ((Get-ShortcutTarget (Join-Path $script:MeowTestDesktopRoot 'Meow Launcher.lnk')) -ne $launcherCanonicalTarget) { throw 'Launcher desktop shortcut does not point to the canonical executable.' }
} finally {
    Stop-ExactProcess $launcherCanonicalTarget
}

[ordered]@{
    ok = $true
    test_root = $testRoot
    manager = [ordered]@{
        from = $previousManagerVersion
        to = [string]$managerCurrent.Release.version
        same_target = $managerTarget
        executable_replaced = $true
        desktop_shortcut = $true
        user_file_preserved = $true
    }
    helper = [ordered]@{
        from = $previousHelperVersion
        to = [string]$helperCurrent.Release.version
        same_target = $helperTarget
        executable_replaced = $true
        desktop_shortcut = $true
        user_file_preserved = $true
    }
    launcher = [ordered]@{
        from = $previousLauncherVersion
        to = [string]$launcherCurrent.Release.version
        legacy_target = $launcherLegacyTarget
        canonical_target = $launcherCanonicalTarget
        old_owned_files_removed = $true
        desktop_shortcut = $true
        user_file_preserved = $true
        restarted = $true
    }
} | ConvertTo-Json -Depth 5
