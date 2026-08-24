[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Normalize-Fingerprint([string]$Value) {
    return ($Value.ToUpperInvariant() -replace '[^0-9A-F]', '')
}

function Remove-CertificateFromStore(
    [System.Security.Cryptography.X509Certificates.StoreName]$StoreName,
    [string]$Thumbprint
) {
    $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
        $StoreName,
        [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
    )
    try {
        $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
        $matches = $store.Certificates.Find(
            [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
            $Thumbprint,
            $false
        )
        foreach ($match in $matches) {
            $store.Remove($match)
        }
    }
    finally {
        $store.Close()
        $store.Dispose()
    }
}

function Get-CertificateFromStore(
    [System.Security.Cryptography.X509Certificates.StoreName]$StoreName,
    [string]$Thumbprint,
    [bool]$RequirePrivateKey
) {
    $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
        $StoreName,
        [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
    )
    try {
        $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
        $matches = $store.Certificates.Find(
            [System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint,
            $Thumbprint,
            $false
        )
        return @($matches | Where-Object { -not $RequirePrivateKey -or $_.HasPrivateKey }) |
            Select-Object -First 1
    }
    finally {
        $store.Close()
        $store.Dispose()
    }
}

$root = Split-Path -Parent $PSScriptRoot
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) (
    'codex-tweaks-signing-smoke-' + [guid]::NewGuid().ToString('N')
)
$pfxPath = Join-Path $temporaryRoot 'signing-smoke.pfx'
$githubEnvironmentPath = Join-Path $temporaryRoot 'github-env'
$signedExecutablePath = Join-Path $temporaryRoot 'codex-tweaks-backend-signed.exe'
$tamperedExecutablePath = Join-Path $temporaryRoot 'codex-tweaks-backend-tampered.exe'
. "$PSScriptRoot/windows-signing-verification.ps1"
$password = [Convert]::ToBase64String(
    [System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)
)
$securePassword = ConvertTo-SecureString $password -AsPlainText -Force
$thumbprint = ''
$environmentNames = @(
    'WINDOWS_SIGNING_PFX_BASE64',
    'CODE_SIGNING_EXPORT_PASSWORD',
    'WINDOWS_SIGNING_CERT_SHA256',
    'GITHUB_ENV',
    'RUNNER_TEMP'
)
$previousEnvironment = @{}
foreach ($name in $environmentNames) {
    $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
}

New-Item -ItemType Directory -Force -Path $temporaryRoot | Out-Null
try {
    Write-Host '[windows-signing-smoke] Generating an ephemeral code-signing certificate.'
    $generatedCertificate = New-SelfSignedCertificate `
        -Type CodeSigningCert `
        -Subject 'CN=Codex Tweaks CI Signing Smoke' `
        -CertStoreLocation 'Cert:\CurrentUser\My' `
        -KeyAlgorithm RSA `
        -KeyLength 2048 `
        -HashAlgorithm SHA256 `
        -KeyExportPolicy Exportable `
        -NotAfter (Get-Date).AddDays(1)
    $thumbprint = Normalize-Fingerprint $generatedCertificate.Thumbprint
    if ([string]::IsNullOrWhiteSpace($thumbprint) -or -not $generatedCertificate.HasPrivateKey) {
        throw 'The smoke-test certificate was not created with a private key.'
    }

    [System.IO.File]::WriteAllBytes(
        $pfxPath,
        $generatedCertificate.Export(
            [System.Security.Cryptography.X509Certificates.X509ContentType]::Pfx,
            $password
        )
    )
    $sha256 = Normalize-Fingerprint ($generatedCertificate.GetCertHashString(
        [System.Security.Cryptography.HashAlgorithmName]::SHA256
    ))

    Write-Host '[windows-signing-smoke] Removing generation-time certificate-store entries.'
    Remove-CertificateFromStore `
        ([System.Security.Cryptography.X509Certificates.StoreName]::My) `
        $thumbprint

    $env:WINDOWS_SIGNING_PFX_BASE64 = [Convert]::ToBase64String(
        [System.IO.File]::ReadAllBytes($pfxPath)
    )
    $env:CODE_SIGNING_EXPORT_PASSWORD = $password
    $env:WINDOWS_SIGNING_CERT_SHA256 = $sha256
    $env:GITHUB_ENV = $githubEnvironmentPath
    $env:RUNNER_TEMP = $temporaryRoot

    Write-Host '[windows-signing-smoke] Running the production import script.'
    & "$PSScriptRoot/import-windows-signing-certificate.ps1" -TimeoutSeconds 30

    $myCertificate = Get-CertificateFromStore `
        ([System.Security.Cryptography.X509Certificates.StoreName]::My) `
        $thumbprint `
        $true
    if ($null -eq $myCertificate) {
        throw 'The production import did not persist the private key in CurrentUser/My.'
    }
    $githubEnvironment = @(Get-Content $githubEnvironmentPath)
    if ($githubEnvironment -notcontains "WINDOWS_SIGNING_THUMBPRINT=$thumbprint") {
        throw 'The production import did not publish the expected signing thumbprint.'
    }

    Write-Host '[windows-signing-smoke] Signing and verifying a copied test PE.'
    $sourceExecutable = Join-Path $root 'artifacts/windows/win-x64/publish/codex-tweaks-backend.exe'
    if (-not (Test-Path $sourceExecutable -PathType Leaf)) {
        throw "Missing Windows smoke-test executable: $sourceExecutable"
    }
    Copy-Item $sourceExecutable $signedExecutablePath

    $signTool = Get-ChildItem `
        (Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin') `
        -Filter 'signtool.exe' `
        -File `
        -Recurse `
        | Where-Object FullName -Match '\\x64\\signtool\.exe$' `
        | Sort-Object FullName -Descending `
        | Select-Object -First 1
    if ($null -eq $signTool) {
        throw 'SignTool was not found on the Windows runner.'
    }
    & $signTool.FullName sign /sha1 $thumbprint /s My /fd SHA256 $signedExecutablePath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "SignTool failed with exit code $LASTEXITCODE."
    }

    Assert-AuthenticodeSigner $signedExecutablePath $sha256

    Write-Host '[windows-signing-smoke] Tampering with the signed PE for a negative integrity test.'
    Copy-Item $signedExecutablePath $tamperedExecutablePath
    $tamperedBytes = [System.IO.File]::ReadAllBytes($tamperedExecutablePath)
    if ($tamperedBytes.Length -le 1024) {
        throw 'The smoke-test PE is unexpectedly small.'
    }
    $tamperedBytes[1024] = $tamperedBytes[1024] -bxor 1
    [System.IO.File]::WriteAllBytes($tamperedExecutablePath, $tamperedBytes)
    $tamperedSignature = Get-AuthenticodeSignature -FilePath $tamperedExecutablePath
    Write-Host "[windows-signing-smoke] Tampered Authenticode status: $($tamperedSignature.Status)"
    if ($tamperedSignature.Status -ne [System.Management.Automation.SignatureStatus]::HashMismatch) {
        throw "The tampered PE did not produce HashMismatch: $($tamperedSignature.Status)."
    }
    $strictVerifierRejectedTampering = $false
    try {
        Assert-AuthenticodeSigner $tamperedExecutablePath $sha256
    }
    catch {
        $strictVerifierRejectedTampering = $true
    }
    if (-not $strictVerifierRejectedTampering) {
        throw 'The production Authenticode verifier accepted the tampered PE.'
    }
    Write-Host '[windows-signing-smoke] Import, private key, signing, signer, and tamper checks passed.'
}
finally {
    if (-not [string]::IsNullOrWhiteSpace($thumbprint)) {
        Remove-CertificateFromStore `
            ([System.Security.Cryptography.X509Certificates.StoreName]::My) `
            $thumbprint
    }
    foreach ($name in $environmentNames) {
        [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name])
    }
    if (Test-Path $temporaryRoot) {
        Remove-Item -Recurse -Force $temporaryRoot
    }
}
