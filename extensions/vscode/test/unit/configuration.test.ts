// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import * as path from 'node:path';
import { test } from 'node:test';
import { cliArguments, ConfigurationError, parseConfiguration } from '../../src/configuration';

const workspace = path.resolve('DBeaver workspace 日本語');

test('defaults use the selected DBeaver tab without overriding indentation', () => {
  const configuration = parseConfiguration({ workspace });
  assert.deepEqual(cliArguments(configuration), ['--workspace', workspace, '--timeout', '10000ms', '--active-editor']);
});

test('only the selected mode contributes a connection selector', () => {
  assert.deepEqual(cliArguments(parseConfiguration({ workspace, connectionMode: 'global', connectionId: 'ignored' })), ['--workspace', workspace, '--timeout', '10000ms']);
  assert.deepEqual(cliArguments(parseConfiguration({ workspace, connectionMode: 'connection', connectionId: '--quoted 日本語 " id', timeoutMs: 123 })), ['--workspace', workspace, '--timeout', '123ms', '--connection', '--quoted 日本語 " id']);
});

test('invalid settings fail before a CLI can be spawned', () => {
  for (const values of [
    {}, { workspace: '' }, { workspace: 'relative/path' }, { workspace: '/nul\0path' },
    { workspace, connectionMode: 'automatic' },
    { workspace, connectionMode: 'connection' }, { workspace, connectionMode: 'connection', connectionId: '   ' },
    { workspace, connectionId: 'a\0b' }, { workspace, connectionId: 1 },
    ...[0, -1, 120001, 0.5, NaN, Infinity, '10000'].map(timeoutMs => ({ workspace, timeoutMs })),
  ]) {
    assert.throws(() => parseConfiguration(values), ConfigurationError);
  }
});

test('timeout endpoints and irrelevant empty ID are accepted', () => {
  assert.equal(parseConfiguration({ workspace, timeoutMs: 1 }).timeoutMs, 1);
  assert.equal(parseConfiguration({ workspace, timeoutMs: 120000 }).timeoutMs, 120000);
  assert.equal(parseConfiguration({ workspace, connectionMode: 'global' }).connectionId, '');
});
