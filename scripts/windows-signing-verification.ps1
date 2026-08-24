$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Normalize-WindowsSigningFingerprint([string]$Value) {
    return ($Value.ToUpperInvariant() -replace '[^0-9A-F]', '')
}

function Assert-AuthenticodeSigner([string]$Path, [string]$ExpectedSha256) {
    if ([string]::IsNullOrWhiteSpace($ExpectedSha256)) {
        return
    }
    if (-not (Test-Path $Path -PathType Leaf)) {
        throw "Missing signed Windows artifact: $Path"
    }

    $signature = Get-AuthenticodeSignature -FilePath $Path
    Write-Host "[windows-signing-verify] Authenticode status for $(Split-Path -Leaf $Path): $($signature.Status)"
    if ($null -eq $signature.SignerCertificate) {
        throw "Authenticode signer certificate is missing for $Path"
    }

    $actualSha256 = Normalize-WindowsSigningFingerprint (
        $signature.SignerCertificate.GetCertHashString(
            [System.Security.Cryptography.HashAlgorithmName]::SHA256
        )
    )
    if ($actualSha256 -ne $ExpectedSha256) {
        throw "Authenticode signer mismatch for $Path. Expected $ExpectedSha256; actual $actualSha256."
    }

    switch ($signature.Status) {
        ([System.Management.Automation.SignatureStatus]::Valid) {
            return
        }
        ([System.Management.Automation.SignatureStatus]::NotTrusted) {
            $chain = [System.Security.Cryptography.X509Certificates.X509Chain]::new()
            try {
                $chain.ChainPolicy.TrustMode = `
                    [System.Security.Cryptography.X509Certificates.X509ChainTrustMode]::CustomRootTrust
                [void]$chain.ChainPolicy.CustomTrustStore.Add($signature.SignerCertificate)
                $chain.ChainPolicy.RevocationMode = `
                    [System.Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck
                $chain.ChainPolicy.VerificationFlags = `
                    [System.Security.Cryptography.X509Certificates.X509VerificationFlags]::NoFlag
                if (-not $chain.Build($signature.SignerCertificate)) {
                    $chainErrors = @($chain.ChainStatus | ForEach-Object Status) -join ', '
                    throw "The pinned self-signed Authenticode chain is invalid for $Path`: $chainErrors"
                }
            }
            finally {
                $chain.Dispose()
            }
            return
        }
        default {
            throw "Authenticode signature failed strict validation for $Path`: $($signature.Status) $($signature.StatusMessage)"
        }
    }
}
