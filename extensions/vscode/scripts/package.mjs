// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
import { mkdir } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createVSIX } from '@vscode/vsce';
const require = createRequire(import.meta.url);
const { name, version } = require('../package.json');
const cwd = fileURLToPath(new URL('..', import.meta.url));
const output = resolve(cwd, '../../dist', `${name}-${version}.vsix`);
await mkdir(resolve(cwd, '../../dist'), { recursive: true });
await createVSIX({ cwd, packagePath: output, dependencies: false });
console.log(`Created ${output}`);
