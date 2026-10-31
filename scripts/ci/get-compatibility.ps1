# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
# Dot-source this helper from a disposable CI test.
function Get-CompatibilityAsset([string] $Root, [string] $Component, [string] $Destination) {
    $definition = Get-Content -LiteralPath (Join-Path $Root 'releases/compatibility.json') -Raw | ConvertFrom-Json
    if ($definition.schema -ne 1 -or $Component -notin @('plugin', 'cli', 'vscode')) { throw 'Invalid compatibility definition' }
    $asset = $definition.$Component
    if ($asset.version -notmatch '^\d+\.\d+\.\d+$' -or $asset.sha256 -notmatch '^[0-9a-f]{64}$' -or
        $asset.url -notmatch '^https://github\.com/lemonadern/dbeaver-formatter-bridge/releases/download/') {
        throw 'Invalid pinned compatibility asset'
    }
    Invoke-WebRequest -Uri $asset.url -UserAgent 'openai-codex/1.0' -OutFile $Destination -TimeoutSec 120
    if ((Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant() -ne $asset.sha256) { throw 'Compatibility asset checksum mismatch' }
    return $asset.version
}
