$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -trimpath -ldflags '-s -w -H=windowsgui' -o 'dist/Meow Launcher v0.4.3.exe' ./cmd/meow-launcher
Write-Host 'Built dist/Meow Launcher v0.4.3.exe'