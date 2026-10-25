// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import { chmod, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import * as os from 'node:os';
import * as path from 'node:path';
import { test } from 'node:test';
import { formatSQL, FormattingCancelled, FormattingError, MAX_SQL_BYTES, resolveCLI } from '../../src/formatter';

function nodeRequest(source: string, sql = '', signal?: AbortSignal, timeoutMs = 1000) {
  return formatSQL({ executable: process.execPath, args: ['-e', source], sql, timeoutMs, signal });
}

test('PATH discovery handles Unicode and spaces and skips directories and missing entries', async t => {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'bridge PATH 日本語 '));
  t.after(() => rm(temporary, { recursive: true, force: true }));
  const binaryName = process.platform === 'win32' ? 'dbeaver-format.exe' : 'dbeaver-format';
  const ignored = path.join(temporary, 'directory candidate');
  const executableDir = path.join(temporary, 'CLI 日本語');
  await mkdir(path.join(ignored, binaryName), { recursive: true });
  await mkdir(executableDir);
  const executable = path.join(executableDir, binaryName);
  await writeFile(executable, 'test executable');
  await chmod(executable, 0o755);
  const PATH = ['', '.', 'relative', path.join(temporary, 'missing'), ignored, executableDir].join(path.delimiter);
  assert.equal(await resolveCLI({ PATH }), executable);
});

test('Windows uses case-insensitive PATH and only a native .exe', async () => {
  const checked: string[] = [];
  const resolved = await resolveCLI({ Path: ';.;relative;"C:\\CLI 日本語";C:\\next' }, 'win32', async candidate => {
    checked.push(candidate);
    return candidate.startsWith('C:\\next');
  });
  assert.deepEqual(checked, ['C:\\CLI 日本語\\dbeaver-format.exe', 'C:\\next\\dbeaver-format.exe']);
  assert.equal(resolved, 'C:\\next\\dbeaver-format.exe');
});

test('empty and relative PATH cannot discover a current-directory tool', async () => {
  let checks = 0;
  await assert.rejects(resolveCLI({ PATH: ':.:relative' }, 'linux', async () => { checks++; return true; }), /Extension Host PATH/);
  assert.equal(checks, 0);
  await assert.rejects(resolveCLI({}, 'linux'), /not found/);
  await assert.rejects(resolveCLI({}, 'freebsd'), /supports desktop/);
});

test('permission errors are distinguished from absent tools', { skip: process.platform === 'win32' }, async t => {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'bridge-permissions-'));
  t.after(() => rm(temporary, { recursive: true, force: true }));
  const executable = path.join(temporary, 'dbeaver-format');
  await writeFile(executable, 'not executable', { mode: 0o644 });
  await assert.rejects(resolveCLI({ PATH: temporary }), /not executable/);
});

test('stdin/stdout preserve Unicode, whitespace, BOM and exact line endings', async () => {
  const sql = '\ufeff  select \'日本語😀\', \'C:\\temp\\new.sql\';\r\n\n ';
  assert.equal(await nodeRequest('process.stdin.pipe(process.stdout)', sql), sql);
  assert.equal(await nodeRequest('process.stdin.resume()'), '');
});

test('SQL stays in stdin and shell metacharacters remain literal arguments', async () => {
  const args = ['--workspace', 'space 日本語 " $HOME `ignored` & ;', '--active-editor'];
  const output = await formatSQL({
    executable: process.execPath,
    args: ['-e', 'process.stdin.resume(); process.stdout.write(JSON.stringify(process.argv.slice(1)))', '--', ...args],
    sql: 'not an argument', timeoutMs: 1000,
  });
  assert.deepEqual(JSON.parse(output), args);
});

test('input limit counts UTF-8 bytes before starting a process', async () => {
  await assert.rejects(formatSQL({ executable: '/does-not-exist', args: [], sql: '😀'.repeat(MAX_SQL_BYTES / 4 + 1), timeoutMs: 1000 }), /at most 1 MiB/);
  assert.equal((await nodeRequest('process.stdin.pipe(process.stdout)', 'x'.repeat(MAX_SQL_BYTES))).length, MAX_SQL_BYTES);
});

test('CLI diagnostics survive early exit without an unhandled EPIPE', async () => {
  await assert.rejects(nodeRequest('process.stderr.write("dbeaver-format: cannot read workspace instance"); process.exit(1)', 'x'.repeat(MAX_SQL_BYTES)), /cannot read workspace instance/);
  await assert.rejects(nodeRequest('process.exit(2)'), /check DBeaver/);
});

test('spawn failure is actionable and does not expose source SQL', async () => {
  await assert.rejects(formatSQL({ executable: path.resolve('nonexistent-cli'), args: [], sql: 'select secret', timeoutMs: 1000 }), error => {
    assert.ok(error instanceof FormattingError);
    assert.match(error.message, /Cannot start/);
    assert.doesNotMatch(error.message, /secret/);
    return true;
  });
});

test('malformed UTF-8 output is rejected', async () => {
  await assert.rejects(nodeRequest('process.stdin.resume(); process.stdout.write(Buffer.from([0xff]))'), /invalid UTF-8/);
});

test('oversized diagnostics and successful stdout are rejected', async () => {
  await assert.rejects(nodeRequest('process.stdin.resume(); process.stderr.write(Buffer.alloc(65537))'), /64 KiB/);
  await assert.rejects(nodeRequest('process.stdin.resume(); process.stdout.write(Buffer.alloc(67108865))', '', undefined, 10000), /64 MiB/);
});

test('pre-cancelled requests do not start a process', async () => {
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(formatSQL({ executable: '/never-run', args: [], sql: '', timeoutMs: 1000, signal: controller.signal }), FormattingCancelled);
});

test('cancellation terminates a running child and resolves promptly', async () => {
  const controller = new AbortController();
  const result = nodeRequest('process.stdin.resume(); setInterval(() => {}, 1000)', '', controller.signal);
  setTimeout(() => controller.abort(), 50);
  await assert.rejects(result, FormattingCancelled);
});

test('process deadline covers hangs outside the CLI HTTP timeout', { timeout: 7000 }, async () => {
  await assert.rejects(nodeRequest('process.stdin.resume(); setInterval(() => {}, 1000)', '', undefined, 1), /process deadline/);
});
