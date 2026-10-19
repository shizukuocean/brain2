# Build generated embed inputs locally; no precompiled binaries are published.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Push-Location $repoRoot
try {
    $savedGoos = $env:GOOS
    $savedGoarch = $env:GOARCH
    try {
        $env:GOARCH = 'amd64'
        $env:GOOS = 'windows'
        go build -trimpath -o internal/agentconnect/assets/brain2-mcp-host-windows-amd64.exe ./cmd/brain2-mcp-host
        if ($LASTEXITCODE -ne 0) { throw 'Windows MCP host build failed' }
        $env:GOOS = 'linux'
        go build -trimpath -o internal/agentconnect/assets/brain2-mcp-host-linux-amd64 ./cmd/brain2-mcp-host
        if ($LASTEXITCODE -ne 0) { throw 'Linux MCP host build failed' }
    } finally {
        $env:GOOS = $savedGoos
        $env:GOARCH = $savedGoarch
    }
    Push-Location frontend
    try {
        npm.cmd ci --ignore-scripts --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency install failed' }
        npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw 'Manager frontend build failed' }
        npm.cmd run build:launcher
        if ($LASTEXITCODE -ne 0) { throw 'Launcher frontend build failed' }
    } finally { Pop-Location }
} finally { Pop-Location }
