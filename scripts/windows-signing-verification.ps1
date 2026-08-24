$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Normalize-WindowsSigningFingerprint([string]$Value) {
    return ($Value.ToUpperInvariant() -replace '[^0-9A-F]', '')
}

function Assert-PinnedSelfSignedChain(
    [System.Security.Cryptography.X509Certificates.X509Certificate2]$SignerCertificate,
    [string]$Path
) {
    $systemChain = [System.Security.Cryptography.X509Certificates.X509Chain]::new()
    try {
        $systemChain.ChainPolicy.RevocationMode = `
            [System.Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck
        $systemChain.ChainPolicy.VerificationFlags = `
            [System.Security.Cryptography.X509Certificates.X509VerificationFlags]::NoFlag
        $systemChainBuilt = $systemChain.Build($SignerCertificate)
        $systemChainStatuses = @($systemChain.ChainStatus | ForEach-Object Status)
        Write-Host "[windows-signing-verify] System chain statuses: $($systemChainStatuses -join ', ')"
        if ($systemChainBuilt `
            -or $systemChainStatuses.Count -ne 1 `
            -or $systemChainStatuses[0] -ne `
                [System.Security.Cryptography.X509Certificates.X509ChainStatusFlags]::UntrustedRoot) {
            throw "Authenticode reported an untrusted signature, but the system chain failure was not exactly UntrustedRoot for $Path."
        }
    }
    finally {
        $systemChain.Dispose()
    }

    $customChain = [System.Security.Cryptography.X509Certificates.X509Chain]::new()
    try {
        $customChain.ChainPolicy.TrustMode = `
            [System.Security.Cryptography.X509Certificates.X509ChainTrustMode]::CustomRootTrust
        [void]$customChain.ChainPolicy.CustomTrustStore.Add($SignerCertificate)
        $customChain.ChainPolicy.RevocationMode = `
            [System.Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck
        $customChain.ChainPolicy.VerificationFlags = `
            [System.Security.Cryptography.X509Certificates.X509VerificationFlags]::NoFlag
        if (-not $customChain.Build($SignerCertificate)) {
            $chainErrors = @($customChain.ChainStatus | ForEach-Object Status) -join ', '
            throw "The pinned self-signed Authenticode chain is invalid for $Path`: $chainErrors"
        }
    }
    finally {
        $customChain.Dispose()
    }
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
            Assert-PinnedSelfSignedChain $signature.SignerCertificate $Path
            return
        }
        ([System.Management.Automation.SignatureStatus]::UnknownError) {
            Assert-PinnedSelfSignedChain $signature.SignerCertificate $Path
            return
        }
        default {
            throw "Authenticode signature failed strict validation for $Path`: $($signature.Status) $($signature.StatusMessage)"
        }
    }
}
