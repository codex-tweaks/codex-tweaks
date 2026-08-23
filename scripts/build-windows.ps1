[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$')]
    [string]$Version,

    [ValidateRange(1, 2147483647)]
    [int]$BuildNumber = 1,

    [ValidateSet('win-x64', 'win-arm64')]
    [string[]]$RuntimeIdentifiers = @('win-x64', 'win-arm64')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$project = Join-Path $root 'windows/CodexTweaks.Windows/CodexTweaks.Windows.csproj'
$backend = Join-Path $root 'backend'
$artifactRoot = Join-Path $root 'artifacts/windows'

Push-Location $backend
try {
    & go run ./cmd/contractgen -root .. -check
    if ($LASTEXITCODE -ne 0) { throw 'Generated Presentation Contract files are stale.' }
}
finally {
    Pop-Location
}

foreach ($rid in $RuntimeIdentifiers) {
    $goArch = if ($rid -eq 'win-arm64') { 'arm64' } else { 'amd64' }
    $publish = Join-Path $artifactRoot "$rid/publish"
    if (Test-Path $publish) {
        Remove-Item -Recurse -Force $publish
    }
    New-Item -ItemType Directory -Force -Path $publish | Out-Null

    & dotnet publish $project `
        --configuration Release `
        --runtime $rid `
        --self-contained true `
        -p:Version=$Version `
        -p:CodexTweaksBuildNumber=$BuildNumber `
        --output $publish
    if ($LASTEXITCODE -ne 0) { throw "WinUI publish failed: $rid" }

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = 'windows'
        $env:GOARCH = $goArch
        $env:CGO_ENABLED = '0'
        Push-Location $backend
        try {
            & go build `
                -trimpath `
                -ldflags "-s -w -X main.version=$Version" `
                -o (Join-Path $publish 'codex-tweaks-backend.exe') `
                ./cmd/codex-tweaks-backend
            if ($LASTEXITCODE -ne 0) { throw "Go sidecar build failed: $rid" }
        }
        finally {
            Pop-Location
        }
    }
    finally {
        $env:GOOS = $previousGoos
        $env:GOARCH = $previousGoarch
        $env:CGO_ENABLED = $previousCgo
    }

    Copy-Item -Recurse -Force `
        (Join-Path $root 'app/Resources/Tweaks') `
        (Join-Path $publish 'Tweaks')
    Copy-Item -Recurse -Force `
        (Join-Path $root 'Skills') `
        (Join-Path $publish 'Skills')

    Write-Host "Built $rid -> $publish"
}
