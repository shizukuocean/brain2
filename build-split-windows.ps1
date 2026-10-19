$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$frontendRoot = Join-Path $repoRoot 'frontend'
$distRoot = Join-Path $repoRoot 'dist'

Set-Location $frontendRoot
& npm.cmd run build
& npm.cmd run build:launcher

Set-Location $repoRoot
New-Item -ItemType Directory -Force -Path $distRoot | Out-Null

$managerOutput = Join-Path $distRoot 'Brain2 Manager v0.6.6 WIP.exe'
$launcherOutput = Join-Path $distRoot 'Meow Launcher v0.6.6 WIP.exe'
$managerIcon = Join-Path $repoRoot 'assets\brain2-manager-icon.png'
$managerResource = Join-Path $repoRoot 'manager_icon_windows_amd64.syso'
$launcherIcon = Join-Path $repoRoot 'assets\meow-launcher-icon.png'
$launcherResource = Join-Path $repoRoot 'launcher_icon_windows_amd64.syso'

if (Test-Path -LiteralPath $managerResource) {
    throw "Refusing to overwrite existing Manager resource: $managerResource"
}
& go run .\cmd\meow-icon-resource --input $managerIcon --output $managerResource
if ($LASTEXITCODE -ne 0) { throw 'Manager icon resource generation failed.' }
try {
    & go build -tags 'desktop,production' -trimpath -ldflags '-s -w -H=windowsgui' -o $managerOutput .
    if ($LASTEXITCODE -ne 0) { throw 'Brain2 Manager build failed.' }
} finally {
    Remove-Item -LiteralPath $managerResource -Force -ErrorAction SilentlyContinue
}
if (Test-Path -LiteralPath $launcherResource) {
    throw "Refusing to overwrite existing Launcher resource: $launcherResource"
}
& go run .\cmd\meow-icon-resource --input $launcherIcon --output $launcherResource
if ($LASTEXITCODE -ne 0) { throw 'Launcher icon resource generation failed.' }
try {
    & go build -tags 'launcher,desktop,production' -trimpath -ldflags '-s -w -H=windowsgui' -o $launcherOutput .
    if ($LASTEXITCODE -ne 0) { throw 'Meow Launcher build failed.' }
} finally {
    Remove-Item -LiteralPath $launcherResource -Force -ErrorAction SilentlyContinue
}

Write-Host "Built $launcherOutput"
Write-Host "Built $managerOutput"
Write-Host 'Launcher is the unified Meow product installer; this preview also resolves Brain2 Manager when both executables share an install directory.'
