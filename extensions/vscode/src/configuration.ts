// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

import * as path from 'node:path';

export type ConnectionMode = 'activeEditor' | 'connection' | 'global';

export interface FormatterConfiguration {
  workspace: string;
  connectionMode: ConnectionMode;
  connectionId: string;
  timeoutMs: number;
}

export class ConfigurationError extends Error {}

export function parseConfiguration(values: {
  workspace?: unknown;
  connectionMode?: unknown;
  connectionId?: unknown;
  timeoutMs?: unknown;
}): FormatterConfiguration {
  const { workspace, connectionMode = 'activeEditor', connectionId = '', timeoutMs = 10000 } = values;
  if (typeof workspace !== 'string' || !path.isAbsolute(workspace) || workspace.includes('\0')) {
    throw new ConfigurationError('Set dbeaverFormatter.workspace to the absolute path of the running DBeaver workspace.');
  }
  if (connectionMode !== 'activeEditor' && connectionMode !== 'connection' && connectionMode !== 'global') {
    throw new ConfigurationError('Choose activeEditor, connection or global for dbeaverFormatter.connectionMode.');
  }
  if (typeof connectionId !== 'string' || connectionId.includes('\0') || (connectionMode === 'connection' && connectionId.trim() === '')) {
    throw new ConfigurationError('Set dbeaverFormatter.connectionId to a nonempty DBeaver connection ID.');
  }
  if (typeof timeoutMs !== 'number' || !Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 120000) {
    throw new ConfigurationError('Set dbeaverFormatter.timeoutMs to an integer from 1 to 120000.');
  }
  return { workspace, connectionMode, connectionId, timeoutMs };
}

export function cliArguments(configuration: FormatterConfiguration): string[] {
  const args = ['--workspace', configuration.workspace, '--timeout', `${configuration.timeoutMs}ms`];
  if (configuration.connectionMode === 'activeEditor') {
    args.push('--active-editor');
  } else if (configuration.connectionMode === 'connection') {
    args.push('--connection', configuration.connectionId);
  }
  return args;
}
