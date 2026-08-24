[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

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

$encodedPfx = $env:WINDOWS_SIGNING_PFX_BASE64 -replace '\s', ''
[System.IO.File]::WriteAllBytes($pfxPath, [Convert]::FromBase64String($encodedPfx))

$keyStorageFlags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::PersistKeySet `
    -bor [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::UserKeySet `
    -bor [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::Exportable
$certificate = [System.Security.Cryptography.X509Certificates.X509Certificate2]::new(
    $pfxPath,
    $env:CODE_SIGNING_EXPORT_PASSWORD,
    $keyStorageFlags
)
if (-not $certificate.HasPrivateKey) {
    throw 'The Windows signing PFX does not contain a private key.'
}

$actualSha256 = Normalize-Fingerprint ($certificate.GetCertHashString(
    [System.Security.Cryptography.HashAlgorithmName]::SHA256
))
$expectedSha256 = Normalize-Fingerprint $env:WINDOWS_SIGNING_CERT_SHA256
if ($expectedSha256.Length -ne 64 -or $actualSha256 -ne $expectedSha256) {
    throw "Windows signing certificate SHA-256 mismatch. Expected $expectedSha256; actual $actualSha256."
}

$securePassword = ConvertTo-SecureString $env:CODE_SIGNING_EXPORT_PASSWORD -AsPlainText -Force
$importedCertificate = Import-PfxCertificate `
    -FilePath $pfxPath `
    -CertStoreLocation 'Cert:\CurrentUser\My' `
    -Password $securePassword `
    -Exportable
if ($null -eq $importedCertificate -or -not $importedCertificate.HasPrivateKey) {
    throw 'The Windows signing certificate could not be imported with its private key.'
}

[System.IO.File]::WriteAllBytes(
    $certificatePath,
    $certificate.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
)
Import-Certificate `
    -FilePath $certificatePath `
    -CertStoreLocation 'Cert:\CurrentUser\Root' `
    | Out-Null

$thumbprint = Normalize-Fingerprint $importedCertificate.Thumbprint
if ([string]::IsNullOrWhiteSpace($thumbprint)) {
    throw 'The imported Windows signing certificate has no SHA-1 thumbprint.'
}

Add-Content -Path $env:GITHUB_ENV -Value "WINDOWS_SIGNING_THUMBPRINT=$thumbprint"
Add-Content -Path $env:GITHUB_ENV -Value "VPK_SIGN_PARAMS=/sha1 $thumbprint /s My /fd SHA256"

Write-Host "Windows signing identity imported and SHA-256 verified: $actualSha256"
