# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
# Runs only against a disposable CI extraction; never uses an installed DBeaver.
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = $OutputEncoding
[Console]::OutputEncoding = $OutputEncoding

$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
Set-Location $root
$build = Join-Path $root 'build'
New-Item -ItemType Directory -Force $build | Out-Null
$report = Join-Path $build 'ci-report.txt'
Set-Content -LiteralPath $report -Value 'DBeaver CE 26.2.2 / Windows x64 integration' -Encoding utf8
$stage = 'initialization'
$gui = $null
$temp = Join-Path ([IO.Path]::GetTempPath()) ('bridge CI 日本語 ' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $temp | Out-Null

function Report([string] $message) {
    Add-Content -LiteralPath $report -Value $message -Encoding utf8
    Write-Host $message
}

function Start-Java([string[]] $Arguments) {
    $info = [Diagnostics.ProcessStartInfo]::new()
    $info.FileName = Join-Path $env:JAVA_HOME 'bin/java.exe'
    $info.UseShellExecute = $false
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.Environment['DBEAVER_DATA'] = Join-Path $temp 'data'
    $info.ArgumentList.Add('-Duser.home=' + (Join-Path $temp 'user-home'))
    foreach ($arg in $Arguments) { $info.ArgumentList.Add($arg) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $info
    if (-not $process.Start()) { throw 'Java process could not start' }
    # Drain both streams asynchronously. Never upload or print DBeaver output:
    # a future DBeaver version could include private instance details in logs.
    $stdout = $process.StandardOutput.ReadToEndAsync()
    $stderr = $process.StandardError.ReadToEndAsync()
    return @{ Process = $process; Stdout = $stdout; Stderr = $stderr }
}

try {
    $stage = 'download and checksum'
    $zip = Join-Path $temp 'dbeaver.zip'
    $uri = 'https://github.com/dbeaver/dbeaver/releases/download/26.2.2/dbeaver-ce-26.2.2-windows-x86_64.zip'
    $expected = 'ea6f4c7cc864d15fc5ce64e4142e1c785acf2699b198bf9a3b0e9db93adb909c'
    Invoke-WebRequest -Uri $uri -UserAgent 'openai-codex/1.0' -OutFile $zip -TimeoutSec 300
    if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) {
        throw 'DBeaver archive checksum mismatch'
    }
    $stage = 'archive extraction'
    Expand-Archive -LiteralPath $zip -DestinationPath (Join-Path $temp 'application')
    $stage = 'application discovery'
    $executables = @(Get-ChildItem (Join-Path $temp 'application') -Recurse -Filter dbeaver.exe)
    if ($executables.Count -ne 1) { throw 'Expected exactly one extracted DBeaver executable' }
    $dbeaverHome = $executables[0].DirectoryName
    Report 'PASS: pinned official DBeaver download SHA-256'

    $stage = 'plugin build'
    & go run ./scripts/prepare-target.go -dbeaver-home $dbeaverHome -java (Join-Path $env:JAVA_HOME 'bin/java.exe')
    & go run ./scripts/build-plugin.go
    & go run ./scripts/ci/verify-site.go
    $cli = Join-Path $temp 'CLI 配置/dbeaver-format.exe'
    New-Item -ItemType Directory (Split-Path $cli) | Out-Null
    Push-Location cli
    try { & go build -o $cli ./cmd/dbeaver-format } finally { Pop-Location }
    Report 'PASS: Tycho plugin and native Windows CLI build'

    $stage = 'p2 installation'
    $launcher = @(Get-ChildItem (Join-Path $dbeaverHome 'plugins') -Filter 'org.eclipse.equinox.launcher_*.jar')
    if ($launcher.Count -ne 1) { throw 'Expected exactly one Equinox launcher' }
    $site = (Resolve-Path 'plugin/repository/target/formatter-bridge-update-site-0.1.0-SNAPSHOT.zip').Path
    $repository = 'jar:' + ([Uri]::new($site)).AbsoluteUri + '!/'
    $install = Start-Java -Arguments @('-jar', $launcher[0].FullName,
        '-nosplash', '-application', 'org.eclipse.equinox.p2.director',
        '-repository', $repository,
        '-installIU', 'com.github.lemonadern.dbeaver.formatter.bridge.feature.feature.group',
        '-destination', $dbeaverHome, '-profile', 'DefaultProfile',
        '-configuration', (Join-Path $dbeaverHome 'configuration'),
        '-data', (Join-Path $temp 'director-workspace'))
    try {
        if (-not $install.Process.WaitForExit(180000)) { throw 'p2 installation timed out' }
        if ($install.Process.ExitCode -ne 0) { throw 'p2 installation failed' }
    } finally {
        if (-not $install.Process.HasExited) { $install.Process.Kill($true); $install.Process.WaitForExit() }
        $install.Process.Dispose()
    }
    Report 'PASS: p2 update-site ZIP installed into disposable DBeaver'

    $stage = 'GUI startup and readiness'
    $workspace = Join-Path $temp 'workspace 空白'
    # The extracted application and its p2-managed configuration are disposable.
    # The workspace and user.home are isolated from the CI account as well.
    $gui = Start-Java -Arguments @('-jar', $launcher[0].FullName, '-nosplash',
        '-configuration', (Join-Path $dbeaverHome 'configuration'), '-data', $workspace)
    $ready = $false
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while ($timer.Elapsed.TotalSeconds -lt 120) {
        if ($gui.Process.HasExited) { throw 'DBeaver GUI exited before readiness' }
        # Never parse, print or upload the RPC registry. The CLI handles it.
        $sql = 'select 1'
        $oldNative = $PSNativeCommandUseErrorActionPreference
        $PSNativeCommandUseErrorActionPreference = $false
        try {
            $null = $sql | & $cli --workspace $workspace --timeout 2s 2>$null
            if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        } finally { $PSNativeCommandUseErrorActionPreference = $oldNative }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw 'DBeaver bridge readiness timed out' }
    Report 'PASS: real Windows DBeaver GUI and bridge readiness'

    $stage = 'real bridge integration'
    & go run ./scripts/test-bridge.go --cli $cli --workspace $workspace
    Report 'PASS: bridge Unicode, LF/CRLF, empty input, failures and concurrency'

    $stage = 'PowerShell UTF-8 stdin'
    $sql = "select '日本語😀' as greeting,'C:\temp\new.sql' as path from example where id=1;"
    $formatted = ($sql | & $cli --workspace $workspace) -join "`n"
    if (-not $formatted.Contains("'日本語😀'")) { throw 'PowerShell Unicode literal changed' }
    if (-not $formatted.Contains("'C:\temp\new.sql'")) { throw 'PowerShell path literal changed' }
    Report 'PASS: PowerShell UTF-8 stdin and literal preservation'

    $stage = 'Git Bash UTF-8 stdin'
    $env:DBEAVER_BRIDGE_CLI = $cli
    $env:DBEAVER_BRIDGE_WORKSPACE = $workspace
    $bash = Join-Path $env:ProgramFiles 'Git/bin/bash.exe'
    & $bash --noprofile --norc ./scripts/ci/git-bash-stdin.sh
    Report 'PASS: Git Bash UTF-8 stdin and literal preservation'
    Report 'PASS: all Windows integration stages completed'
} catch {
    # Keep failure artifacts useful without exposing raw app output or requests.
    Report ('FAIL: ' + $stage + '; line=' + $_.InvocationInfo.ScriptLineNumber + '; type=' + $_.Exception.GetType().FullName)
    throw ('Windows integration failed during ' + $stage + '; see sanitized ci-report.txt')
} finally {
    if ($null -ne $gui) {
        if (-not $gui.Process.HasExited) { $gui.Process.Kill($true); $gui.Process.WaitForExit() }
        $gui.Process.Dispose()
    }
    Remove-Item Env:DBEAVER_BRIDGE_CLI -ErrorAction SilentlyContinue
    Remove-Item Env:DBEAVER_BRIDGE_WORKSPACE -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
