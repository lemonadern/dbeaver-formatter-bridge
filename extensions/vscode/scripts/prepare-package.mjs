// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
import { copyFile } from 'node:fs/promises';
for (const name of ['LICENSE', 'COPYRIGHT']) {
  await copyFile(new URL(`../../../${name}`, import.meta.url), new URL(`../${name}`, import.meta.url));
}
