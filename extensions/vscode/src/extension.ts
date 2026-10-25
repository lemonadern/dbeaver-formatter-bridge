// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

import * as vscode from 'vscode';
import { cliArguments, ConfigurationError, parseConfiguration } from './configuration';
import { formatSQL, FormattingCancelled, resolveCLI } from './formatter';

export function activate(context: vscode.ExtensionContext): void {
  const pending = new Map<string, AbortController>();
  let disposed = false;
  let lastError = '';
  let lastErrorAt = 0;
  const cancelDocument = (document: vscode.TextDocument) => {
    pending.get(document.uri.toString())?.abort();
  };
  context.subscriptions.push(
    { dispose: () => { disposed = true; for (const request of pending.values()) { request.abort(); } pending.clear(); } },
    vscode.workspace.onDidCloseTextDocument(cancelDocument),
    vscode.workspace.onDidChangeTextDocument(event => cancelDocument(event.document)),
    vscode.languages.registerDocumentFormattingEditProvider(
      [{ language: 'sql', scheme: 'file' }, { language: 'sql', scheme: 'untitled' }],
      {
        async provideDocumentFormattingEdits(document, _options, token) {
          if (disposed || !vscode.workspace.isTrusted || token.isCancellationRequested) { return []; }
          const key = document.uri.toString();
          pending.get(key)?.abort();
          const controller = new AbortController();
          pending.set(key, controller);
          const cancellation = token.onCancellationRequested(() => controller.abort());
          const version = document.version;
          try {
            const settings = vscode.workspace.getConfiguration('dbeaverFormatter');
            const configuration = parseConfiguration({
              workspace: settings.get('workspace'),
              connectionMode: settings.get('connectionMode'),
              connectionId: settings.get('connectionId'),
              timeoutMs: settings.get('timeoutMs'),
            });
            const executable = await resolveCLI();
            if (controller.signal.aborted) { return []; }
            const output = await formatSQL({ executable, args: cliArguments(configuration), sql: document.getText(), timeoutMs: configuration.timeoutMs, signal: controller.signal });
            if (controller.signal.aborted || document.isClosed || document.version !== version || disposed || !vscode.workspace.isTrusted) { return []; }
            lastError = '';
            const range = new vscode.Range(document.positionAt(0), document.positionAt(document.getText().length));
            return [vscode.TextEdit.replace(range, output)];
          } catch (error) {
            if (error instanceof FormattingCancelled || controller.signal.aborted || disposed || document.isClosed || document.version !== version) { return []; }
            const message = error instanceof Error ? error.message : 'DBeaver formatting failed.';
            const now = Date.now();
            // Format-on-save must not repeatedly interrupt the same failing setup.
            if (message !== lastError || now - lastErrorAt >= 30000) {
              lastError = message;
              lastErrorAt = now;
              const choices = error instanceof ConfigurationError ? ['Open Settings'] : [];
              void vscode.window.showErrorMessage(`DBeaver Formatter Bridge: ${message}`, ...choices)
                .then(choice => { if (choice === 'Open Settings') { void vscode.commands.executeCommand('workbench.action.openSettings', '@ext:lemonadern.dbeaver-formatter-bridge'); } });
            }
            return [];
          } finally {
            cancellation.dispose();
            if (pending.get(key) === controller) { pending.delete(key); }
          }
        },
      },
    ),
  );
}
