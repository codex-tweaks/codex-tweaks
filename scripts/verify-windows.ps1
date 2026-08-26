[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$')]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [ValidateSet('stable', 'beta')]
    [string]$Channel,

    [ValidateSet('win-x64', 'win-arm64')]
    [string[]]$RuntimeIdentifiers = @('win-x64', 'win-arm64'),

    [switch]$RequirePackages,

    [string]$ExpectedSigningCertificateSha256 = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$artifactRoot = Join-Path $root 'artifacts/windows'
. "$PSScriptRoot/windows-signing-verification.ps1"

$expectedSigningSha256 = Normalize-WindowsSigningFingerprint $ExpectedSigningCertificateSha256
if (-not [string]::IsNullOrWhiteSpace($expectedSigningSha256) -and $expectedSigningSha256.Length -ne 64) {
    throw 'ExpectedSigningCertificateSha256 must be a complete SHA-256 fingerprint.'
}

function Assert-PackageAuthenticodeSignatures(
    [string]$PackagePath,
    [string]$ExpectedSha256
) {
    if ([string]::IsNullOrWhiteSpace($ExpectedSha256)) {
        return
    }

    $extractRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("codex-tweaks-nupkg-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $extractRoot | Out-Null
    try {
        [System.IO.Compression.ZipFile]::ExtractToDirectory($PackagePath, $extractRoot)
        foreach ($executableName in @('CodexTweaks.Windows.exe', 'codex-tweaks-backend.exe')) {
            $matches = @(Get-ChildItem $extractRoot -Recurse -File -Filter $executableName)
            if ($matches.Count -ne 1) {
                throw "Expected exactly one $executableName in $PackagePath; found $($matches.Count)."
            }
            Assert-AuthenticodeSigner $matches[0].FullName $ExpectedSha256
        }
    }
    finally {
        if (Test-Path $extractRoot) {
            Remove-Item -Recurse -Force $extractRoot
        }
    }
}

function Get-PeMachine([string]$Path) {
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $reader = [System.IO.BinaryReader]::new($stream)
        try {
            $stream.Position = 0x3c
            $peOffset = $reader.ReadInt32()
            $stream.Position = $peOffset
            if ($reader.ReadUInt32() -ne 0x00004550) {
                throw "Invalid PE signature: $Path"
            }
            return $reader.ReadUInt16()
        }
        finally {
            $reader.Dispose()
        }
    }
    finally {
        $stream.Dispose()
    }
}

function Assert-PeMachine([string]$Path, [uint16]$Expected) {
    if (-not (Test-Path $Path -PathType Leaf)) {
        throw "Missing Windows executable: $Path"
    }
    $actual = Get-PeMachine $Path
    if ($actual -ne $Expected) {
        throw ('Unexpected PE machine for {0}: expected 0x{1:X4}, actual 0x{2:X4}' -f $Path, $Expected, $actual)
    }
}

function Invoke-BackendSmoke([string]$Backend, [string]$Publish, [string]$ExpectedVersion) {
    $versionStartInfo = New-Object System.Diagnostics.ProcessStartInfo
    $versionStartInfo.FileName = $Backend
    $versionStartInfo.Arguments = '--version'
    $versionStartInfo.UseShellExecute = $false
    $versionStartInfo.CreateNoWindow = $true
    $versionStartInfo.RedirectStandardOutput = $true
    $versionStartInfo.RedirectStandardError = $true
    $versionProcess = New-Object System.Diagnostics.Process
    $versionProcess.StartInfo = $versionStartInfo
    [void]$versionProcess.Start()
    $actualVersion = $versionProcess.StandardOutput.ReadToEnd().Trim()
    $versionError = $versionProcess.StandardError.ReadToEnd().Trim()
    $versionProcess.WaitForExit()
    if ($versionProcess.ExitCode -ne 0 -or $actualVersion -ne $ExpectedVersion) {
        if (-not [string]::IsNullOrWhiteSpace($versionError)) {
            throw "Backend version check failed: $versionError"
        }
        throw "Backend version mismatch: expected $ExpectedVersion, actual $actualVersion"
    }

    $sessionRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("codex-tweaks-smoke-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $sessionRoot | Out-Null
    try {
        $requests = @(
            (@{ id = 1; method = 'ping' } | ConvertTo-Json -Compress),
            (@{
                id = 2
                method = 'initialize'
                params = @{
                    applicationSupportDirectory = Join-Path $sessionRoot 'support'
                    cacheDirectory = Join-Path $sessionRoot 'cache'
                    bundledPackagesDirectory = Join-Path $Publish 'Tweaks/packages'
                    currentVersion = $ExpectedVersion
                    buildNumber = '1'
                }
            } | ConvertTo-Json -Compress -Depth 8),
            (@{ id = 3; method = 'shutdown' } | ConvertTo-Json -Compress)
        )
        $utf8NoBOM = New-Object System.Text.UTF8Encoding($false)
        $startInfo = New-Object System.Diagnostics.ProcessStartInfo
        $startInfo.FileName = $Backend
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        $startInfo.RedirectStandardInput = $true
        $startInfo.RedirectStandardOutput = $true
        $startInfo.RedirectStandardError = $true
        $startInfo.StandardOutputEncoding = $utf8NoBOM
        $startInfo.StandardErrorEncoding = $utf8NoBOM
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $startInfo
        [void]$process.Start()
        foreach ($request in $requests) {
            $process.StandardInput.WriteLine($request)
        }
        $process.StandardInput.Close()
        $output = $process.StandardOutput.ReadToEnd()
        $errorOutput = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) {
            throw "Backend RPC smoke exited with status $($process.ExitCode): $errorOutput"
        }
        $responses = @($output -split '\r?\n' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object {
            $_ | ConvertFrom-Json
        })
        $rpcResponses = @($responses | Where-Object { $null -ne $_.PSObject.Properties['id'] })
        $ping = $rpcResponses | Where-Object { $_.id -eq 1 } | Select-Object -First 1
        $initialize = $rpcResponses | Where-Object { $_.id -eq 2 } | Select-Object -First 1
        $shutdown = $rpcResponses | Where-Object { $_.id -eq 3 } | Select-Object -First 1
        if ($null -eq $ping -or $ping.result.backend -ne 'go' -or $ping.result.protocolVersion -ne 9) {
            throw 'Backend ping response did not satisfy protocol v9.'
        }
        if ($null -eq $initialize -or $initialize.result.protocolVersion -ne 9) {
            throw 'Backend initialize response did not satisfy protocol v9.'
        }
        $shutdownHasError = $null -ne $shutdown `
            -and $shutdown.PSObject.Properties.Name -contains 'error' `
            -and $null -ne $shutdown.error
        if ($null -eq $shutdown -or $shutdownHasError) {
            throw 'Backend shutdown response was not successful.'
        }
    }
    finally {
        if (Test-Path $sessionRoot) {
            Remove-Item -Recurse -Force $sessionRoot
        }
    }
}

$expectedReleaseAssets = @()
foreach ($rid in $RuntimeIdentifiers) {
    $architecture = if ($rid -eq 'win-arm64') { 'arm64' } else { 'x64' }
    $downloadArchitecture = if ($rid -eq 'win-arm64') { 'arm64' } else { 'x86_64' }
    $expectedMachine = if ($rid -eq 'win-arm64') { [uint16]0xAA64 } else { [uint16]0x8664 }
    $publish = Join-Path $artifactRoot "$rid/publish"
    $frontend = Join-Path $publish 'CodexTweaks.Windows.exe'
    $backend = Join-Path $publish 'codex-tweaks-backend.exe'
    $xamlResources = Join-Path $publish 'CodexTweaks.Windows.pri'
    $appIcon = Join-Path $publish 'Assets/CodexTweaks.ico'
    $appLogo = Join-Path $publish 'Assets/CodexTweaks.png'

    Assert-PeMachine $frontend $expectedMachine
    Assert-PeMachine $backend $expectedMachine
    if (-not (Test-Path $xamlResources -PathType Leaf)) {
        throw "WinUI XAML resource index is missing: $rid"
    }
    if (-not (Test-Path $appIcon -PathType Leaf)) {
        throw "Windows application icon is missing: $rid"
    }
    if (-not (Test-Path $appLogo -PathType Leaf)) {
        throw "Windows application logo is missing: $rid"
    }
    if (-not (Test-Path (Join-Path $publish 'Tweaks/packages') -PathType Container)) {
        throw "Bundled tweak packages are missing: $rid"
    }
    if (-not (Test-Path (Join-Path $publish 'Skills') -PathType Container)) {
        throw "Bundled Skills are missing: $rid"
    }

    $hostArchitecture = $env:PROCESSOR_ARCHITECTURE
    $canExecute = ($rid -eq 'win-x64' -and $hostArchitecture -eq 'AMD64') `
        -or ($rid -eq 'win-arm64' -and $hostArchitecture -eq 'ARM64')
    if ($canExecute) {
        Invoke-BackendSmoke $backend $publish $Version
    }
    else {
        Write-Host "Skipped native RPC execution for $rid on $hostArchitecture; PE architecture was verified."
    }

    if ($RequirePackages) {
        $channelName = "win-$architecture-$Channel"
        $releases = Join-Path $artifactRoot "$rid/releases"
        $setup = Join-Path $releases "Codex-Tweaks-v${Version}-windows-Setup-${downloadArchitecture}.exe"
        if (-not (Test-Path $setup -PathType Leaf)) {
            throw "Missing versioned Velopack installer: $setup"
        }
        $fullPackages = @(Get-ChildItem $releases -Filter '*-full.nupkg' -File)
        if ($fullPackages.Count -ne 1) {
            throw "Expected exactly one Velopack full package for $rid, found $($fullPackages.Count)."
        }
        $feeds = @(Get-ChildItem $releases -Filter "releases.$channelName.json" -File)
        if ($feeds.Count -ne 1) {
            throw "Missing Velopack channel feed releases.$channelName.json"
        }
        Assert-AuthenticodeSigner $setup $expectedSigningSha256
        Assert-PackageAuthenticodeSignatures $fullPackages[0].FullName $expectedSigningSha256
        $expectedReleaseAssets += @(
            (Split-Path -Leaf $setup),
            $fullPackages[0].Name,
            $feeds[0].Name
        )
    }

    Write-Host "Verified $rid publish output."
}

if ($RequirePackages) {
    $stagedRelease = Join-Path $artifactRoot 'release'
    if (-not (Test-Path $stagedRelease -PathType Container)) {
        throw "Missing staged Windows release directory: $stagedRelease"
    }
    $expected = @($expectedReleaseAssets | Sort-Object)
    $actual = @(Get-ChildItem $stagedRelease -File | ForEach-Object Name | Sort-Object)
    if (($actual -join "`n") -ne ($expected -join "`n")) {
        throw "Unexpected staged Windows assets. Expected: $($expected -join ', '); actual: $($actual -join ', ')"
    }
    Write-Host "Verified exact staged Windows release asset set ($($actual.Count) files)."
}
