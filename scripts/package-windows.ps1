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

    foreach ($rid in $RuntimeIdentifiers) {
        $architecture = if ($rid -eq 'win-arm64') { 'arm64' } else { 'x64' }
        $packId = "com.crzhichen.CodexTweaks.$architecture"
        $publish = Join-Path $artifactRoot "$rid/publish"
        $releases = Join-Path $artifactRoot "$rid/releases"
        if (-not (Test-Path (Join-Path $publish 'CodexTweaks.Windows.exe'))) {
            throw "Missing $rid publish output. Run scripts/build-windows.ps1 first."
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
        $versionedSetup = Join-Path $releases "$packId-$Version-$channelName-Setup.exe"
        if (-not (Test-Path $generatedSetup)) {
            throw "Velopack did not create the expected installer: $generatedSetup"
        }
        Move-Item -Force $generatedSetup $versionedSetup
        Write-Host "Packaged $rid / $Channel -> $releases"
    }

    if (Test-Path $stagedRelease) {
        Remove-Item -Recurse -Force $stagedRelease
    }
    New-Item -ItemType Directory -Force -Path $stagedRelease | Out-Null
    foreach ($rid in $RuntimeIdentifiers) {
        $releases = Join-Path $artifactRoot "$rid/releases"
        foreach ($file in Get-ChildItem $releases -File) {
            $destination = Join-Path $stagedRelease $file.Name
            if (Test-Path $destination) {
                throw "Duplicate staged release asset: $($file.Name)"
            }
            Copy-Item $file.FullName $destination
        }
    }
    $checksums = Get-ChildItem $stagedRelease -File | Sort-Object Name | ForEach-Object {
        $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($_.Name)"
    }
    Set-Content -Path (Join-Path $stagedRelease 'SHA256SUMS-Windows') -Value $checksums -Encoding utf8NoBOM
    Write-Host "Staged Windows release assets -> $stagedRelease"
}
finally {
    Pop-Location
}
