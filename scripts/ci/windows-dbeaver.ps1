# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
# Runs only against a disposable CI extraction; never uses an installed DBeaver.
[CmdletBinding()]
param([switch] $UseLegacyPlugin)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = $OutputEncoding
[Console]::OutputEncoding = $OutputEncoding

$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$version = ([IO.File]::ReadAllText((Join-Path $root 'plugin/VERSION'))).Trim()
if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid VERSION' }
Set-Location $root
. (Join-Path $PSScriptRoot 'get-compatibility.ps1')
$build = Join-Path $root 'build'
New-Item -ItemType Directory -Force $build | Out-Null
$report = Join-Path $build 'ci-report.txt'
Set-Content -LiteralPath $report -Value 'DBeaver CE 26.2.2 / Windows x64 integration' -Encoding utf8
$stage = 'initialization'
$gui = $null
$temp = Join-Path ([IO.Path]::GetTempPath()) ('bridge CI ' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $temp | Out-Null

function Report([string] $message) {
    Add-Content -LiteralPath $report -Value $message -Encoding utf8
    Write-Host $message
}

function Start-TestProcess([string[]] $Arguments, [switch] $NativeDBeaver) {
    $info = [Diagnostics.ProcessStartInfo]::new()
    $info.UseShellExecute = $false
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.WorkingDirectory = $dbeaverHome
    $info.Environment['DBEAVER_DATA'] = Join-Path $temp 'data'
    if ($NativeDBeaver) {
        # Use the product launcher, its bundled runtime and vendor DLL search paths.
        $info.FileName = Join-Path $dbeaverHome 'dbeaver.exe'
    } else {
        $info.FileName = Join-Path $env:JAVA_HOME 'bin/java.exe'
        $info.ArgumentList.Add('-Duser.home=' + (Join-Path $temp 'user-home'))
    }
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
    if (-not $UseLegacyPlugin) { & ./scripts/ci/p2-site.ps1 -DBeaverHome $dbeaverHome }
    $cli = Join-Path $temp 'CLI 配置/dbeaver-format.exe'
    New-Item -ItemType Directory (Split-Path $cli) | Out-Null
    Push-Location cli
    try { & go build -o $cli ./cmd/dbeaver-format } finally { Pop-Location }
    Report 'PASS: Tycho plugin and native Windows CLI build'

    $stage = 'p2 installation'
    $launcher = @(Get-ChildItem (Join-Path $dbeaverHome 'plugins') -Filter 'org.eclipse.equinox.launcher_*.jar')
    if ($launcher.Count -ne 1) { throw 'Expected exactly one Equinox launcher' }
    $site = (Resolve-Path ("plugin/repository/target/formatter-bridge-update-site-" + $version + ".zip")).Path
    if ($UseLegacyPlugin) {
        $site = Join-Path $temp 'legacy-plugin.zip'
        $legacyVersion = Get-CompatibilityAsset $root 'plugin' $site
        Report ('Compatibility: current CLI and VSIX against published plugin ' + $legacyVersion)
    }
    $repository = 'jar:' + ([Uri]::new($site)).AbsoluteUri + '!/'
    $install = Start-TestProcess -Arguments @('-jar', $launcher[0].FullName,
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

    $stage = 'active editor test fixture build'
    $fixture = Join-Path $temp 'active-editor-fixture.jar'
    & go run ./scripts/ci/build-active-editor-fixture.go -dbeaver-home $dbeaverHome -java-home $env:JAVA_HOME -output $fixture
    # Test-only bundle is appended only to this disposable installation, after the
    # distribution ZIP has been built and checked. It is never shipped to users.
    $bundlesInfo = Join-Path $dbeaverHome 'configuration/org.eclipse.equinox.simpleconfigurator/bundles.info'
    [IO.File]::AppendAllText($bundlesInfo,
        "`ncom.github.lemonadern.dbeaver.formatter.fixture,1.0.0," + ([Uri]::new($fixture)).AbsoluteUri + ",4,false`n",
        [Text.UTF8Encoding]::new($false))
    $fixtureResult = Join-Path $temp 'active-editor-result.txt'

    $stage = 'GUI startup and readiness'
    $workspace = Join-Path $temp 'workspace 空白'
    # Windows java.exe converts native arguments through the system ANSI code page.
    # Keep its own paths ASCII and pass the Unicode workspace as an encoded file URI.
    # The native Go CLI still receives the real Unicode filesystem path.
    # The extracted application and its p2-managed configuration are disposable.
    # The workspace and user.home are isolated from the CI account as well.
    # Amend only the disposable vendor ini, preserving all existing JVM options.
    $legacyFixture = if ($UseLegacyPlugin) { 'true' } else { 'false' }
    [IO.File]::AppendAllText((Join-Path $dbeaverHome 'dbeaver.ini'),
        "`n-Dbridge.fixture.legacy=" + $legacyFixture + "`n-Dproduct.config.disable=true`n-Duser.home=" + (Join-Path $temp 'user-home') + "`n-Dbridge.fixture.result=" + $fixtureResult + "`n-Dbridge.fixture.cli=" + ([Uri]::new($cli)).AbsoluteUri + "`n", [Text.UTF8Encoding]::new($false))
    $gui = Start-TestProcess -NativeDBeaver -Arguments @('-nosplash', '-consoleLog',
        '-data', ([Uri]::new($workspace)).AbsoluteUri)
    $ready = $false
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while ($timer.Elapsed.TotalSeconds -lt 120) {
        if ($gui.Process.HasExited) {
            Report ('DBeaver startup exit code: ' + $gui.Process.ExitCode)
            $startup = $gui.Stdout.GetAwaiter().GetResult() + "`n" + $gui.Stderr.GetAwaiter().GetResult()
            $logs = @(Get-ChildItem (Join-Path $dbeaverHome 'configuration') -Recurse -Filter '*.log' -ErrorAction SilentlyContinue)
            $workspaceLog = Join-Path $workspace '.metadata/.log'
            foreach ($log in $logs) { $startup += "`n" + (Get-Content -LiteralPath $log.FullName -Raw) }
            if (Test-Path -LiteralPath $workspaceLog) { $startup += "`n" + (Get-Content -LiteralPath $workspaceLog -Raw) }
            # Emit only bounded exception summaries, never raw app logs or registry files.
            $summaries = @($startup -split "`r?`n" | Where-Object { $_ -match '(Exception|Error):|^!MESSAGE|swt.*\.dll|[Cc]an.t load|in java.library.path|dependent libraries' } | Select-Object -Last 24)
            foreach ($line in $summaries) {
                $safe = $line -replace '(?i)(password|authorization|bearer)[^\r\n]*', '[redacted]'
                $safe = $safe -replace '[A-Za-z0-9_-]{32,}', '[redacted]'
                $safe = $safe -replace '[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]', ''
                if ($safe.Length -gt 500) { $safe = $safe.Substring(0, 500) }
                Report ('Startup diagnostic: ' + $safe)
            }
            throw 'DBeaver GUI exited before readiness'
        }
        # Wait for a UI dispatch checkpoint. Only the legacy compatibility
        # fixture warms EditorsUI; the current plugin must initialize it safely
        # through its first default-indent RPC without fixture assistance.
        if (-not (Test-Path -LiteralPath ($fixtureResult + '.ui-ready'))) {
            if (Test-Path -LiteralPath $fixtureResult) { throw 'Fixture failed before workbench UI readiness' }
            Start-Sleep -Seconds 1
            continue
        }
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

    $stage = 'active editor integration'
    $fixtureTimer = [Diagnostics.Stopwatch]::StartNew()
    while (-not (Test-Path -LiteralPath $fixtureResult)) {
        if ($gui.Process.HasExited) { throw 'DBeaver exited during active editor tests' }
        if ($fixtureTimer.Elapsed.TotalSeconds -gt 120) { throw 'Active editor fixture timed out' }
        Start-Sleep -Seconds 1
    }
    $fixtureLines = @(Get-Content -LiteralPath $fixtureResult -Encoding utf8)
    foreach ($line in $fixtureLines) { Report $line }
    if ($fixtureLines -contains 'PASS: active-editor integration completed' -and -not ($fixtureLines | Where-Object { $_ -like 'FAIL:*' })) {
        Report 'PASS: real Workbench active SQL tabs, per-connection preferences, GUI formatter comparison, errors, UI timeout and native CLI'
    } else {
        throw 'Active editor integration fixture failed'
    }

    # The fixture deliberately blocks the UI to exercise its timeout. Wait for
    # completion before issuing other integration requests.
    $stage = 'real bridge integration'
    & go run ./scripts/test-bridge.go --cli $cli --workspace $workspace
    Report 'PASS: bridge Unicode, LF/CRLF, empty input, failures and concurrency'

    $stage = 'installed VSIX and real DBeaver'
    $env:DBEAVER_BRIDGE_CLI = $cli
    $env:DBEAVER_BRIDGE_WORKSPACE = $workspace
    & npm --prefix extensions/vscode run test:vsix
    Report 'PASS: installed VSIX through PATH CLI to real DBeaver active-editor and global formatting'

    if (-not $UseLegacyPlugin) {
        $stage = 'published minimum CLI compatibility'
        $legacyArchive = Join-Path $temp 'legacy-cli.zip'
        $legacyCLIVersion = Get-CompatibilityAsset $root 'cli' $legacyArchive
        $legacyDirectory = Join-Path $temp 'legacy-cli'
        Expand-Archive -LiteralPath $legacyArchive -DestinationPath $legacyDirectory
        $legacyCLI = Join-Path $legacyDirectory 'dbeaver-format.exe'
        & go run ./scripts/test-bridge.go --cli $legacyCLI --workspace $workspace
        $env:DBEAVER_BRIDGE_CLI = $legacyCLI
        & npm --prefix extensions/vscode run test:vsix
        Report ('PASS: current plugin and VSIX with published minimum CLI ' + $legacyCLIVersion)
        $env:DBEAVER_BRIDGE_CLI = $cli

        $stage = 'published minimum VSIX compatibility'
        $legacyVSIX = Join-Path $temp 'legacy.vsix'
        $legacyVSIXVersion = Get-CompatibilityAsset $root 'vscode' $legacyVSIX
        $env:BRIDGE_TEST_VSIX = $legacyVSIX
        $env:BRIDGE_TEST_EXPECTED_VERSION = $legacyVSIXVersion
        try { & npm --prefix extensions/vscode run test:vsix } finally {
            Remove-Item Env:BRIDGE_TEST_VSIX -ErrorAction SilentlyContinue
            Remove-Item Env:BRIDGE_TEST_EXPECTED_VERSION -ErrorAction SilentlyContinue
        }
        Report ('PASS: current CLI and plugin with published minimum VSIX ' + $legacyVSIXVersion)
    }

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
    if ($null -ne $gui -and $gui.Process.HasExited) {
        Report ('DBeaver exit code: ' + $gui.Process.ExitCode)
        $diagnostics = $gui.Stdout.GetAwaiter().GetResult() + "`n" + $gui.Stderr.GetAwaiter().GetResult()
        $appLog = Join-Path $workspace '.metadata/.log'
        if (Test-Path -LiteralPath $appLog) { $diagnostics += "`n" + (Get-Content -LiteralPath $appLog -Raw) }
        $lines = @($diagnostics -split "`r?`n" | Where-Object { $_ -match '(Exception|Error):|^!MESSAGE|fatal error|EXCEPTION_ACCESS_VIOLATION|Problematic frame|SIGSEGV' } | Select-Object -Last 16)
        foreach ($line in $lines) {
            $safe = $line -replace '(?i)(password|authorization|bearer)[^\r\n]*', '[redacted]'
            $safe = $safe -replace '[A-Za-z0-9_-]{32,}', '[redacted]'
            $safe = $safe -replace '[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]', ''
            if ($safe.Length -gt 500) { $safe = $safe.Substring(0, 500) }
            Report ('Exit diagnostic: ' + $safe)
        }
    }
    if (Test-Path -LiteralPath (Join-Path $temp 'active-editor-result.txt.progress')) {
        Get-Content -LiteralPath (Join-Path $temp 'active-editor-result.txt.progress') -Encoding utf8 | ForEach-Object { Report $_ }
    }
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
