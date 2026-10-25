// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

import { spawn } from 'node:child_process';
import { constants } from 'node:fs';
import { access, stat } from 'node:fs/promises';
import * as path from 'node:path';
import { TextDecoder } from 'node:util';

export const MAX_SQL_BYTES = 1 << 20;
export const MAX_STDOUT_BYTES = 64 << 20;
export const MAX_STDERR_BYTES = 64 << 10;

export class FormattingError extends Error {}
export class FormattingCancelled extends Error {}

export async function resolveCLI(
  environment: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
  checkExecutable?: (candidate: string) => Promise<boolean>,
): Promise<string> {
  if (!['win32', 'darwin', 'linux'].includes(platform)) {
    throw new FormattingError('DBeaver Formatter Bridge supports desktop Windows, macOS and Linux.');
  }
  const windows = platform === 'win32';
  const paths = windows ? path.win32 : path.posix;
  const name = windows ? 'dbeaver-format.exe' : 'dbeaver-format';
  const pathKey = windows ? Object.keys(environment).find(key => key.toLowerCase() === 'path') : 'PATH';
  const search = (pathKey ? environment[pathKey] : '') ?? '';
  let denied = false;
  for (let directory of search.split(windows ? ';' : ':')) {
    if (windows && directory.startsWith('"') && directory.endsWith('"')) {
      directory = directory.slice(1, -1);
    }
    if (!directory || !paths.isAbsolute(directory)) {
      continue;
    }
    const candidate = paths.join(directory, name);
    try {
      if (checkExecutable) {
        if (await checkExecutable(candidate)) { return candidate; }
      } else if ((await stat(candidate)).isFile()) {
        await access(candidate, windows ? constants.F_OK : constants.X_OK);
        return candidate;
      }
    } catch (error) {
      if (['EACCES', 'EPERM'].includes((error as NodeJS.ErrnoException).code ?? '')) {
        denied = true;
      }
    }
  }
  throw new FormattingError(denied
    ? 'dbeaver-format on PATH is not executable; check its file permissions.'
    : 'dbeaver-format was not found on the VS Code Extension Host PATH. Install the CLI, add its directory to PATH, then restart VS Code.');
}

export interface FormatRequest {
  executable: string;
  args: string[];
  sql: string;
  timeoutMs: number;
  signal?: AbortSignal;
}

/** No shell, files or logs contain the SQL. Successful stdout is preserved verbatim. */
export function formatSQL(request: FormatRequest): Promise<string> {
  if (request.signal?.aborted) { return Promise.reject(new FormattingCancelled()); }
  const input = Buffer.from(request.sql, 'utf8');
  if (input.byteLength > MAX_SQL_BYTES) {
    return Promise.reject(new FormattingError('SQL must be UTF-8 and at most 1 MiB.'));
  }
  return new Promise((resolve, reject) => {
    const child = spawn(request.executable, request.args, {
      shell: false,
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
      // Avoid looking up or running a project-local executable via cwd.
      cwd: path.dirname(request.executable),
    });
    let settled = false;
    let stdoutBytes = 0;
    let stderrBytes = 0;
    let inputError = false;
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    const cleanup = () => {
      clearTimeout(timer);
      request.signal?.removeEventListener('abort', cancel);
    };
    const fail = (error: Error) => {
      if (settled) { return; }
      settled = true;
      cleanup();
      // The CLI is a single Go process, so no shell/process tree needs killing.
      child.kill('SIGKILL');
      child.stdin.destroy();
      child.stdout.destroy();
      child.stderr.destroy();
      reject(error);
    };
    const cancel = () => fail(new FormattingCancelled());
    const timer = setTimeout(() => fail(new FormattingError('DBeaver formatting exceeded its process deadline.')), request.timeoutMs + 2000);
    request.signal?.addEventListener('abort', cancel, { once: true });
    // Cancellation may have happened between the initial check and subscribing.
    if (request.signal?.aborted) { cancel(); }
    child.on('error', () => fail(new FormattingError('Cannot start dbeaver-format; check the CLI permissions and OS/CPU compatibility.')));
    child.stdout.on('data', (chunk: Buffer) => {
      if (settled) { return; }
      stdoutBytes += chunk.byteLength;
      if (stdoutBytes > MAX_STDOUT_BYTES) {
        fail(new FormattingError('dbeaver-format stdout exceeds 64 MiB.'));
      } else { stdout.push(chunk); }
    });
    child.stderr.on('data', (chunk: Buffer) => {
      if (settled) { return; }
      stderrBytes += chunk.byteLength;
      if (stderrBytes > MAX_STDERR_BYTES) {
        fail(new FormattingError('dbeaver-format diagnostics exceed 64 KiB.'));
      } else { stderr.push(chunk); }
    });
    // CLI configuration failures can close stdin before reading any SQL (EPIPE).
    child.stdin.on('error', () => { inputError = true; });
    child.stdout.on('error', () => fail(new FormattingError('Cannot read dbeaver-format output.')));
    child.stderr.on('error', () => fail(new FormattingError('Cannot read dbeaver-format diagnostics.')));
    child.on('close', code => {
      if (settled) { return; }
      if (code !== 0) {
        // The bridge CLI emits fixed diagnostics, never SQL or credentials.
        // Bound the visible text and strip control characters from a PATH tool.
        const diagnostic = Buffer.concat(stderr).toString('utf8')
          .replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, '').trim().slice(0, 1000);
        fail(new FormattingError(diagnostic || 'dbeaver-format failed; check DBeaver and the bridge plugin.'));
        return;
      }
      if (inputError) {
        fail(new FormattingError('Cannot write SQL to dbeaver-format stdin.'));
        return;
      }
      let formatted: string;
      try {
        formatted = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(Buffer.concat(stdout));
      } catch {
        fail(new FormattingError('dbeaver-format returned invalid UTF-8.'));
        return;
      }
      settled = true;
      cleanup();
      resolve(formatted);
    });
    child.stdin.end(input);
  });
}
