[CmdletBinding()]
param(
    [switch]$Worker,

    [ValidateRange(30, 600)]
    [int]$TimeoutSeconds = 120
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-SigningStage([string]$Message) {
    Write-Host "[windows-signing] $Message"
}

$requiredVariables = @(
    'WINDOWS_SIGNING_PFX_BASE64',
    'CODE_SIGNING_EXPORT_PASSWORD',
    'WINDOWS_SIGNING_CERT_SHA256',
    'GITHUB_ENV'
)
foreach ($variableName in $requiredVariables) {
    $value = [Environment]::GetEnvironmentVariable($variableName)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "Missing required release variable: $variableName"
    }
}

if (-not $Worker) {
    Write-SigningStage "Starting isolated non-interactive import with a $TimeoutSeconds-second timeout."
    $pwshPath = (Get-Process -Id $PID).Path
    $scriptPathArgument = '"' + $PSCommandPath.Replace('"', '\"') + '"'
    $arguments = @(
        '-NoLogo',
        '-NoProfile',
        '-NonInteractive',
        '-File',
        $scriptPathArgument,
        '-Worker'
    )
    $process = Start-Process `
        -FilePath $pwshPath `
        -ArgumentList $arguments `
        -NoNewWindow `
        -PassThru
    try {
        if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
            $process.Kill($true)
            throw "Windows signing identity import exceeded $TimeoutSeconds seconds."
        }
        if ($process.ExitCode -ne 0) {
            throw "Windows signing identity import failed with exit code $($process.ExitCode)."
        }
    }
    finally {
        if (-not $process.HasExited) {
            $process.Kill($true)
        }
        $process.Dispose()
    }
    Write-SigningStage 'Isolated import completed.'
    return
}

function Normalize-Fingerprint([string]$Value) {
    return ($Value.ToUpperInvariant() -replace '[^0-9A-F]', '')
}

$runnerTemp = if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
    [System.IO.Path]::GetTempPath()
}
else {
    $env:RUNNER_TEMP
}
$signingDirectory = Join-Path $runnerTemp ("codex-tweaks-signing-" + [guid]::NewGuid().ToString('N'))
$pfxPath = Join-Path $signingDirectory 'zgccrui-windows-code-signing.pfx'
$certificatePath = Join-Path $signingDirectory 'zgccrui-windows-code-signing.cer'
New-Item -ItemType Directory -Force -Path $signingDirectory | Out-Null

Write-SigningStage 'Decoding the encrypted certificate container.'
$encodedPfx = $env:WINDOWS_SIGNING_PFX_BASE64 -replace '\s', ''
[System.IO.File]::WriteAllBytes($pfxPath, [Convert]::FromBase64String($encodedPfx))

Write-SigningStage 'Loading the PFX into the current-user key store.'
$keyStorageFlags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::PersistKeySet `
    -bor [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::UserKeySet
$certificate = [System.Security.Cryptography.X509Certificates.X509Certificate2]::new(
    $pfxPath,
    $env:CODE_SIGNING_EXPORT_PASSWORD,
    $keyStorageFlags
)
if (-not $certificate.HasPrivateKey) {
    throw 'The Windows signing PFX does not contain a private key.'
}
Write-SigningStage 'PFX private key is available.'

$actualSha256 = Normalize-Fingerprint ($certificate.GetCertHashString(
    [System.Security.Cryptography.HashAlgorithmName]::SHA256
))
$expectedSha256 = Normalize-Fingerprint $env:WINDOWS_SIGNING_CERT_SHA256
if ($expectedSha256.Length -ne 64 -or $actualSha256 -ne $expectedSha256) {
    throw "Windows signing certificate SHA-256 mismatch. Expected $expectedSha256; actual $actualSha256."
}
Write-SigningStage 'Certificate SHA-256 fingerprint is verified.'

$thumbprint = Normalize-Fingerprint $certificate.Thumbprint
if ([string]::IsNullOrWhiteSpace($thumbprint)) {
    throw 'The Windows signing certificate has no SHA-1 thumbprint.'
}

Write-SigningStage 'Adding the signing identity to CurrentUser/My without certificate cmdlets.'
$myStore = [System.Security.Cryptography.X509Certificates.X509Store]::new(
    [System.Security.Cryptography.X509Certificates.StoreName]::My,
    [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
)
try {
    $myStore.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
    $myStore.Add($certificate)
}
finally {
    $myStore.Close()
    $myStore.Dispose()
}

Write-SigningStage 'Verifying the persisted signing identity and private key.'
$myStore = [System.Security.Cryptography.X509Certificates.X509Store]::new(
    [System.Security.Cryptography.X509Certificates.StoreName]::My,
    [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
)
try {
    $myStore.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
    $storedCertificates = $myStore.Certificates.Find(
        [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
        $thumbprint,
        $false
    )
    $storedCertificate = @($storedCertificates | Where-Object HasPrivateKey)[0]
    if ($null -eq $storedCertificate) {
        throw 'The Windows signing identity was not persisted with its private key.'
    }
}
finally {
    $myStore.Close()
    $myStore.Dispose()
}

Write-SigningStage 'Exporting the public certificate for trust-store import.'
[System.IO.File]::WriteAllBytes(
    $certificatePath,
    $certificate.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
)

Write-SigningStage 'Adding the public certificate to CurrentUser/Root with non-interactive certutil.'
$certutilPath = (Get-Command certutil.exe -ErrorAction Stop).Source
$certificatePathArgument = '"' + $certificatePath.Replace('"', '\"') + '"'
$certutilProcess = Start-Process `
    -FilePath $certutilPath `
    -ArgumentList @('-user', '-f', '-silent', '-addstore', 'Root', $certificatePathArgument) `
    -NoNewWindow `
    -PassThru `
    -Wait
try {
    if ($certutilProcess.ExitCode -ne 0) {
        throw "certutil failed to add the Windows signing certificate to CurrentUser/Root with exit code $($certutilProcess.ExitCode)."
    }
}
finally {
    $certutilProcess.Dispose()
}

Write-SigningStage 'Verifying the trusted public certificate in CurrentUser/Root.'
$rootStore = [System.Security.Cryptography.X509Certificates.X509Store]::new(
    [System.Security.Cryptography.X509Certificates.StoreName]::Root,
    [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
)
try {
    $rootStore.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
    $trustedCertificates = $rootStore.Certificates.Find(
        [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
        $thumbprint,
        $false
    )
    if ($trustedCertificates.Count -eq 0) {
        throw 'The Windows signing certificate was not persisted in CurrentUser/Root.'
    }
}
finally {
    $rootStore.Close()
    $rootStore.Dispose()
}

Write-SigningStage 'Verifying the offline trust chain.'
$chain = [System.Security.Cryptography.X509Certificates.X509Chain]::new()
try {
    $chain.ChainPolicy.RevocationMode = `
        [System.Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck
    $chain.ChainPolicy.VerificationFlags = `
        [System.Security.Cryptography.X509Certificates.X509VerificationFlags]::NoFlag
    if (-not $chain.Build($storedCertificate)) {
        $chainErrors = @($chain.ChainStatus | ForEach-Object Status) -join ', '
        throw "The Windows signing certificate is not trusted after import: $chainErrors"
    }
}
finally {
    $chain.Dispose()
}

Add-Content -Path $env:GITHUB_ENV -Value "WINDOWS_SIGNING_THUMBPRINT=$thumbprint"
Add-Content -Path $env:GITHUB_ENV -Value "VPK_SIGN_PARAMS=/sha1 $thumbprint /s My /fd SHA256"

Write-SigningStage 'Signing identity import, private-key persistence, fingerprint, and trust checks passed.'
