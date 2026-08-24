[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$')]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [ValidateSet('stable', 'beta')]
    [string]$Channel,

    [ValidateSet('win-x64', 'win-arm64')]
    [string[]]$RuntimeIdentifiers = @('win-x64', 'win-arm64')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$artifactRoot = Join-Path $root 'artifacts/windows'
$stagedRelease = Join-Path $artifactRoot 'release'
$icon = Join-Path $root 'windows/CodexTweaks.Windows/Assets/CodexTweaks.ico'

Push-Location $root
try {
    & dotnet tool restore
    if ($LASTEXITCODE -ne 0) { throw 'Unable to restore the pinned Velopack CLI.' }

    $releaseAssets = @()
    foreach ($rid in $RuntimeIdentifiers) {
        $architecture = if ($rid -eq 'win-arm64') { 'arm64' } else { 'x64' }
        $downloadArchitecture = if ($rid -eq 'win-arm64') { 'arm64' } else { 'x86_64' }
        $packId = "com.crzhichen.CodexTweaks.$architecture"
        $publish = Join-Path $artifactRoot "$rid/publish"
        $releases = Join-Path $artifactRoot "$rid/releases"
        if (-not (Test-Path (Join-Path $publish 'CodexTweaks.Windows.exe'))) {
            throw "Missing $rid publish output. Run scripts/build-windows.ps1 first."
        }
        if (Test-Path $releases) {
            Remove-Item -Recurse -Force $releases
        }
        New-Item -ItemType Directory -Force -Path $releases | Out-Null

        $arguments = @(
            'pack',
            '--packId', $packId,
            '--packVersion', $Version,
            '--packDir', $publish,
            '--mainExe', 'CodexTweaks.Windows.exe',
            '--packTitle', 'Codex Tweaks',
            '--packAuthors', 'cr-zhichen',
            '--icon', $icon,
            '--runtime', $rid,
            '--channel', "win-$architecture-$Channel",
            '--outputDir', $releases,
            '--shortcuts', 'StartMenuRoot'
        )

        if (-not [string]::IsNullOrWhiteSpace($env:VPK_AZURE_TRUSTED_SIGN_FILE)) {
            $arguments += @('--azureTrustedSignFile', $env:VPK_AZURE_TRUSTED_SIGN_FILE)
        }
        elseif (-not [string]::IsNullOrWhiteSpace($env:VPK_SIGN_TEMPLATE)) {
            $arguments += @('--signTemplate', $env:VPK_SIGN_TEMPLATE)
        }

        & dotnet tool run vpk -- @arguments
        if ($LASTEXITCODE -ne 0) { throw "Velopack packaging failed: $rid" }
        $channelName = "win-$architecture-$Channel"
        $generatedSetup = Join-Path $releases "$packId-$channelName-Setup.exe"
        $versionedSetup = Join-Path $releases "Codex-Tweaks-v${Version}-windows-Setup-${downloadArchitecture}.exe"
        if (-not (Test-Path $generatedSetup)) {
            throw "Velopack did not create the expected installer: $generatedSetup"
        }
        Move-Item -Force $generatedSetup $versionedSetup

        $fullPackages = @(Get-ChildItem $releases -Filter '*-full.nupkg' -File)
        if ($fullPackages.Count -ne 1) {
            throw "Expected exactly one Velopack full package for $rid, found $($fullPackages.Count)."
        }
        $feed = Join-Path $releases "releases.$channelName.json"
        if (-not (Test-Path $feed -PathType Leaf)) {
            throw "Missing Velopack channel feed: $feed"
        }
        $releaseAssets += @($versionedSetup, $fullPackages[0].FullName, $feed)
        Write-Host "Packaged $rid / $Channel -> $releases"
    }

    if (Test-Path $stagedRelease) {
        Remove-Item -Recurse -Force $stagedRelease
    }
    New-Item -ItemType Directory -Force -Path $stagedRelease | Out-Null
    foreach ($assetPath in $releaseAssets) {
        $fileName = Split-Path -Leaf $assetPath
        $destination = Join-Path $stagedRelease $fileName
        if (Test-Path $destination) {
            throw "Duplicate staged release asset: $fileName"
        }
        Copy-Item $assetPath $destination
    }
    Write-Host "Staged Windows release assets -> $stagedRelease"
}
finally {
    Pop-Location
}
