#!/usr/bin/env bash
# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cli=$(cygpath -u "$DBEAVER_BRIDGE_CLI")
output=$(mktemp)
trap 'rm -f "$output"' EXIT
printf '%s' "select '日本語😀' as greeting,'C:\\temp\\new.sql' as path from example where id=1;" |
  "$cli" --workspace "$DBEAVER_BRIDGE_WORKSPACE" > "$output"
grep -Fq "'日本語😀'" "$output"
grep -Fq "'C:\\temp\\new.sql'" "$output"
grep -Fqi 'select' "$output"
printf '%s\n' 'PASS: Git Bash UTF-8 stdin and literal preservation'
