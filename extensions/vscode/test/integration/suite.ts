// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import * as http from 'node:http';
import { spawn } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import * as vscode from 'vscode';

type BridgeRequest = { version: number; sql: string; activeEditor?: boolean; connection?: string; indent?: string };
type Handler = (request: BridgeRequest, response: http.ServerResponse) => void;
const extensionID = 'lemonadern.dbeaver-formatter-bridge';

async function configure(mode = 'activeEditor', connectionId = '', timeout = 10000): Promise<void> {
  const config = vscode.workspace.getConfiguration('dbeaverFormatter');
  await config.update('workspace', process.env.BRIDGE_TEST_WORKSPACE, vscode.ConfigurationTarget.Global);
  await config.update('connectionMode', mode, vscode.ConfigurationTarget.Global);
  await config.update('connectionId', connectionId, vscode.ConfigurationTarget.Global);
  await config.update('timeoutMs', timeout, vscode.ConfigurationTarget.Global);
}
async function document(sql: string): Promise<vscode.TextDocument> {
  const doc = await vscode.workspace.openTextDocument({ language: 'sql', content: sql });
  await vscode.window.showTextDocument(doc);
  return doc;
}
async function edits(doc: vscode.TextDocument): Promise<vscode.TextEdit[]> {
  return await vscode.commands.executeCommand<vscode.TextEdit[]>('vscode.executeFormatDocumentProvider', doc.uri, { tabSize: 8, insertSpaces: false }) || [];
}
async function apply(doc: vscode.TextDocument, value: vscode.TextEdit[]): Promise<void> {
  const edit = new vscode.WorkspaceEdit();
  edit.set(doc.uri, value);
  assert.ok(await vscode.workspace.applyEdit(edit));
}
function response(res: http.ServerResponse, request: BridgeRequest, sql: string): void {
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({ exitCode: 0, output: [JSON.stringify({ version: request.version, sql })] }));
}
async function directCLI(sql: string, mode: string): Promise<string> {
  const args = ['--workspace', process.env.BRIDGE_TEST_WORKSPACE!];
  if (mode === 'activeEditor') { args.push('--active-editor'); }
  const child = spawn(process.env.BRIDGE_TEST_CLI!, args, { shell: false, windowsHide: true });
  let output = '';
  child.stdout.setEncoding('utf8').on('data', chunk => { output += chunk; });
  child.stderr.resume();
  return await new Promise<string>((resolve, reject) => {
    child.once('error', reject);
    child.stdin.once('error', reject);
    child.once('close', code => code === 0 ? resolve(output) : reject(new Error('Direct CLI comparison failed')));
    child.stdin.end(sql, 'utf8');
  });
}
function deferred<T = void>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  return { promise: new Promise<T>(r => { resolve = r; }), resolve: value => resolve(value) };
}

export async function run(): Promise<void> {
  const target = vscode.extensions.getExtension(extensionID);
  if (process.env.BRIDGE_TEST_RESTRICTED === '1') {
    assert.equal(vscode.workspace.isTrusted, false, 'Restricted test must actually be untrusted');
    const doc = await document('select 1');
    assert.deepEqual(await edits(doc), []);
    // Unsupported extensions may be omitted from this window's API entirely.
    assert.ok(!target || !target.isActive, 'Untrusted workspace must not activate the formatter');
    console.log('PASS: untrusted workspace cannot activate the installed formatter');
    return;
  }
  assert.ok(target, 'Target VSIX must be installed');
  assert.equal(target.packageJSON.version, process.env.BRIDGE_TEST_VERSION);
  const installedRoot = path.resolve(process.env.BRIDGE_TEST_EXTENSIONS!);
  const relative = path.relative(installedRoot, target.extensionPath);
  assert.ok(relative && !relative.startsWith('..') && !path.isAbsolute(relative), 'Must run the installed VSIX, not the development tree');
  assert.ok(vscode.workspace.isTrusted);
  await target.activate();
  if (process.env.BRIDGE_TEST_REAL === '1') {
    for (const mode of ['activeEditor', 'global']) {
      await configure(mode);
      const sql = "select '日本語' as greeting from example where id=1;";
      const expected = await directCLI(sql, mode);
      const doc = await document(sql);
      const formatted = await edits(doc);
      assert.ok(formatted.length > 0, 'Real DBeaver must return an edit');
      await apply(doc, formatted);
      const eol = doc.eol === vscode.EndOfLine.CRLF ? '\r\n' : '\n';
      assert.equal(doc.getText(), expected.replace(/\r?\n/g, eol), 'Installed formatter must match the direct CLI result');
    }
    console.log('PASS: installed formatter -> PATH CLI -> real DBeaver active/global');
    return;
  }
  let handler: Handler = (request, res) => response(res, request, 'SELECT 1');
  let serverFailure: unknown;
  let requestCount = 0;
  const server = http.createServer((req, res) => {
    requestCount++;
    let body = '';
    req.setEncoding('utf8');
    req.on('data', chunk => { body += chunk; });
    req.on('end', () => {
      try {
        assert.equal(req.url, '/handleCommandLine');
        assert.equal(req.headers.authorization, 'Bearer test-only-token');
        const envelope = JSON.parse(body) as { args: string[] };
        assert.deepEqual(envelope.args.slice(0, 2), ['bridge-format', '--request']);
        handler(JSON.parse(envelope.args[2]) as BridgeRequest, res);
      } catch (err) { serverFailure = err; res.writeHead(500).end(); }
    });
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  assert.ok(address && typeof address !== 'string');
  const metadata = path.join(process.env.BRIDGE_TEST_WORKSPACE!, '.metadata');
  await mkdir(metadata, { recursive: true });
  await writeFile(path.join(metadata, 'dbeaver-instance.properties'), `port=${address.port}\npassword=test-only-token\n`);
  try {
    for (const [mode, connection] of [['activeEditor', ''], ['connection', 'connection 日本語'], ['global', '']]) {
      await configure(mode, connection);
      const sql = "select '日本語😀', 'C:\\temp\\file.sql';\r\n";
      const output = "SELECT\n  '日本語😀', 'C:\\temp\\file.sql';\n";
      handler = (request, res) => {
        assert.equal(request.sql, sql);
        assert.equal(request.indent, undefined, 'Keep DBeaver indentation settings');
        assert.equal(request.version, mode === 'activeEditor' ? 2 : 1);
        assert.equal(request.activeEditor, mode === 'activeEditor' ? true : undefined);
        assert.equal(request.connection, connection || undefined);
        response(res, request, output);
      };
      const doc = await document(sql);
      const result = await edits(doc);
      assert.ok(result.length > 0);
      // VS Code minimizes provider edits and normalizes their EOL before
      // returning this command's result; compare the applied document instead.
      await apply(doc, result);
      assert.equal(doc.getText(), output.replace(/\n/g, '\r\n'), 'VS Code preserves the document CRLF setting on edit application');
      assert.ifError(serverFailure);
    }
    await configure('global');
    const savedPath = path.join(vscode.workspace.workspaceFolders![0].uri.fsPath, '保存済み SQL.sql');
    await writeFile(savedPath, 'select 9');
    const saved = await vscode.workspace.openTextDocument(vscode.Uri.file(savedPath));
    await vscode.window.showTextDocument(saved);
    handler = (request, res) => { assert.equal(request.sql, 'select 9'); response(res, request, 'SELECT 9'); };
    await apply(saved, await edits(saved));
    assert.equal(saved.getText(), 'SELECT 9');
    assert.equal(await readFile(savedPath, 'utf8'), 'select 9', 'Formatting edits the buffer without writing the file');
    await configure();
    handler = (request, res) => response(res, request, '');
    const emptyDoc = await document('select 1');
    const empty = await edits(emptyDoc);
    assert.equal(empty.length, 1);
    assert.equal(empty[0].newText, '');
    await apply(emptyDoc, empty);
    assert.equal(emptyDoc.getText(), '');
    for (const code of ['active_editor_unavailable', 'connection_disconnected', 'unsupported_version']) {
      handler = (request, res) => res.end(JSON.stringify({ exitCode: 1, output: [JSON.stringify({ version: code === 'unsupported_version' ? 1 : request.version, error: code })] }));
      const doc = await document('select 1');
      assert.deepEqual(await edits(doc), []);
      assert.equal(doc.getText(), 'select 1');
    }
    // A promise gate proves a request is in flight before changing the buffer.
    const received = deferred();
    const release = deferred();
    handler = (request, res) => { received.resolve(); void release.promise.then(() => response(res, request, 'OBSOLETE')); };
    const changed = await document('select 1');
    const pending = edits(changed);
    await received.promise;
    const change = new vscode.WorkspaceEdit();
    change.insert(changed.uri, new vscode.Position(0, 8), ' -- edited');
    assert.ok(await vscode.workspace.applyEdit(change));
    release.resolve();
    assert.deepEqual(await pending, []);
    assert.equal(changed.getText(), 'select 1 -- edited');
    // A newer request for the same document supersedes an in-flight result.
    const firstReceived = deferred();
    const firstRelease = deferred();
    let calls = 0;
    handler = (request, res) => {
      if (++calls === 1) {
        firstReceived.resolve();
        void firstRelease.promise.then(() => response(res, request, 'OBSOLETE'));
      } else { response(res, request, 'SELECT 2'); }
    };
    const superseded = await document('select 2');
    const first = edits(superseded);
    await firstReceived.promise;
    const second = await edits(superseded);
    firstRelease.resolve();
    assert.deepEqual(await first, []);
    await apply(superseded, second);
    assert.equal(superseded.getText(), 'SELECT 2');
    // Closing an untitled buffer also discards its pending result.
    const closeReceived = deferred();
    const closeRelease = deferred();
    handler = (request, res) => { closeReceived.resolve(); void closeRelease.promise.then(() => response(res, request, 'OBSOLETE')); };
    const closing = await document('select 3');
    const beforeClose = edits(closing);
    await closeReceived.promise;
    await vscode.commands.executeCommand('workbench.action.revertAndCloseActiveEditor');
    assert.ok(closing.isClosed);
    closeRelease.resolve();
    assert.deepEqual(await beforeClose, []);
    await configure('activeEditor', '', 100);
    handler = () => { /* CLI's HTTP timeout must terminate the request. */ };
    const timedOut = await document('select 1');
    assert.deepEqual(await edits(timedOut), []);
    assert.equal(timedOut.getText(), 'select 1');
    const before = requestCount;
    await configure('connection', '');
    const invalid = await document('select 1');
    assert.deepEqual(await edits(invalid), []);
    assert.equal(requestCount, before, 'Invalid settings must fail before launching a request');
    assert.ifError(serverFailure);
    console.log('PASS: installed VSIX, actual CLI, three selectors, UTF-8, CRLF application, empty result, failures, stale edit, supersession, close and timeout');
  } finally {
    server.closeAllConnections();
    await new Promise<void>(resolve => server.close(() => resolve()));
  }
}
