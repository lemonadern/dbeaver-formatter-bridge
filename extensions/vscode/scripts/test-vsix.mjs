// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
// Tests the installed VSIX, never the production extension's development tree.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { downloadAndUnzipVSCode } from '@vscode/test-electron';

const extensionRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const root = path.resolve(extensionRoot, '../..');
// macOS IPC sockets have a 103-byte path limit; keep profile roots short.
const temp = await mkdtemp(path.join(process.platform === 'darwin' ? '/tmp' : os.tmpdir(), 'bridge-vsix-'));
const executable = process.env.VSCODE_EXECUTABLE_PATH || await downloadAndUnzipVSCode(process.env.VSCODE_TEST_VERSION || '1.100.0');
const vsix = path.resolve(process.env.BRIDGE_TEST_VSIX || path.join(root, 'dist/dbeaver-formatter-bridge-0.1.0.vsix'));
const realWorkspace = process.env.DBEAVER_BRIDGE_WORKSPACE;
const realCLI = process.env.DBEAVER_BRIDGE_CLI;
assert.equal(Boolean(realWorkspace), Boolean(realCLI), 'Both real-DBeaver environment variables must be supplied together');

async function command(program, args, options = {}) {
  const { report = false, ...spawnOptions } = options;
  const child = spawn(program, args, { stdio: ['ignore', 'pipe', 'pipe'], detached: process.platform !== 'win32', windowsHide: true, ...spawnOptions });
  // Compiler/installer output contains no SQL or instance data. Keep it bounded.
  let output = '';
  for (const stream of [child.stdout, child.stderr]) {
    stream.on('data', chunk => { output = (output + chunk).slice(-16000); });
  }
  const timeout = setTimeout(() => {
    if (!child.pid) { return; }
    if (process.platform === 'win32') {
      const killer = spawn('taskkill.exe', ['/pid', String(child.pid), '/t', '/f'], { windowsHide: true, stdio: 'ignore' });
      killer.once('error', () => child.kill());
    } else {
      try { process.kill(-child.pid, 'SIGKILL'); } catch { child.kill('SIGKILL'); }
    }
  }, 180000);
  try {
    await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', code => code === 0 ? resolve() : reject(new Error(`Test setup command failed (${code}): ${output}`)));
    });
    if (report) {
      for (const line of output.split(/\r?\n/)) { if (line.startsWith('PASS:')) { console.log(line); } }
    }
  } finally { clearTimeout(timeout); }
}

try {
  const bin = path.join(temp, 'CLI 配置');
  const extensions = path.join(temp, '拡張 配置');
  const workspace = path.join(temp, 'SQL 文書');
  await Promise.all([mkdir(bin), mkdir(extensions), mkdir(workspace)]);
  const cli = path.join(bin, process.platform === 'win32' ? 'dbeaver-format.exe' : 'dbeaver-format');
  if (realCLI) {
    await copyFile(realCLI, cli);
  } else {
    await command(process.env.BRIDGE_GO || 'go', ['build', '-o', cli, './cmd/dbeaver-format'], { cwd: path.join(root, 'cli') });
  }
  const env = { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH || ''}` };
  // Electron's CLI JS entry point avoids cmd.exe/batch quoting on Windows.
  const cliJS = path.join(path.dirname(executable), process.platform === 'darwin' ? '../Resources/app/out/cli.js' : 'resources/app/out/cli.js');
  const profile = path.join(temp, 'profile');
  await command(executable, [cliJS, '--install-extension', vsix, '--force', '--extensions-dir', extensions, '--user-data-dir', profile], {
    env: { ...env, ELECTRON_RUN_AS_NODE: '1' }
  });
  const metadata = JSON.parse(await readFile(path.join(extensionRoot, 'package.json'), 'utf8'));
  const settings = {
    'security.workspace.trust.startupPrompt': 'never',
    'workbench.startupEditor': 'none',
    'extensions.autoCheckUpdates': false,
    'extensions.autoUpdate': false,
    'update.mode': 'none'
  };
  async function run(restricted) {
    const userData = restricted ? path.join(temp, 'restricted-profile') : profile;
    await mkdir(path.join(userData, 'User'), { recursive: true });
    await writeFile(path.join(userData, 'User/settings.json'), JSON.stringify(settings));
    // runTests() always disables Workspace Trust; launch directly to test the
    // actual restricted mode as well as an ordinary trusted workspace.
    await command(executable, [workspace, '--user-data-dir', userData, '--extensions-dir', extensions,
      '--extensionDevelopmentPath=' + path.join(extensionRoot, 'test/integration/driver'),
      '--extensionTestsPath=' + path.join(extensionRoot, 'out/test/integration/suite.js'),
      '--no-sandbox', '--disable-gpu-sandbox', '--skip-welcome', '--skip-release-notes',
      '--disable-updates', '--disable-telemetry', '--no-cached-data',
      ...(restricted ? [] : ['--disable-workspace-trust'])], {
      report: true,
      env: {
        ...env,
        BRIDGE_TEST_EXTENSIONS: extensions,
        BRIDGE_TEST_CLI: cli,
        BRIDGE_TEST_VERSION: metadata.version,
        BRIDGE_TEST_WORKSPACE: realWorkspace || path.join(temp, 'DBeaver ワークスペース'),
        BRIDGE_TEST_REAL: realWorkspace ? '1' : '',
        BRIDGE_TEST_RESTRICTED: restricted ? '1' : ''
      }
    });
  }
  await run(false);
  if (!realWorkspace && process.env.BRIDGE_TEST_SKIP_RESTRICTED !== '1') { await run(true); }
  console.log(`PASS: installed VSIX ${metadata.version}; ${realWorkspace ? 'real DBeaver active/global' : 'actual CLI and controlled RPC, including restricted workspace'}`);
} finally {
  await rm(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
}
