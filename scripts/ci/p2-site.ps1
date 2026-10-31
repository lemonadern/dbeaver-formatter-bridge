# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
# Installs and updates only an isolated copy made by the Go integration test.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string] $DBeaverHome,
    [string] $LegacyZIP
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('p2-site-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $scratch | Out-Null
$previousHome = $env:P2_DBEAVER_HOME
$previousZIP = $env:P2_LEGACY_ZIP
try {
    if (-not $LegacyZIP) {
        . (Join-Path $PSScriptRoot 'get-compatibility.ps1')
        $LegacyZIP = Join-Path $scratch 'legacy-plugin.zip'
        Get-CompatibilityAsset $root 'plugin' $LegacyZIP | Out-Null
    }
    $env:P2_DBEAVER_HOME = (Resolve-Path -LiteralPath $DBeaverHome).Path
    $env:P2_LEGACY_ZIP = (Resolve-Path -LiteralPath $LegacyZIP).Path
    Push-Location (Join-Path $root 'tools/release')
    try {
        & go test ./internal/p2site -run '^TestP2DirectorIntegration$' -count=1 -timeout=12m -v
        if ($LASTEXITCODE -ne 0) { throw 'p2 HTTP install and update integration failed' }
    } finally { Pop-Location }
} finally {
    $env:P2_DBEAVER_HOME = $previousHome
    $env:P2_LEGACY_ZIP = $previousZIP
    Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue
}
