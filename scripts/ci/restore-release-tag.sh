#!/usr/bin/env bash
# Copyright 2026 lemonadern
# SPDX-License-Identifier: Apache-2.0
# actions/checkout may replace a tag ref with the event's peeled commit. Restore
# only that local ref from origin; never create, push or rewrite a remote tag.
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo 'usage: restore-release-tag.sh <plugin|cli|vscode>/vX.Y.Z' >&2
    exit 1
fi
release_tag=$1
release_pattern='^(plugin|cli|vscode)/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
if [[ ! $release_tag =~ $release_pattern ]]; then
    echo 'invalid component release tag' >&2
    exit 1
fi
release_ref="refs/tags/$release_tag"
git check-ref-format "$release_ref"

# --force updates only the local destination ref in this fetch refspec. A tag
# changed remotely to a different commit still fails the HEAD comparison below.
git fetch --force --no-tags origin "$release_ref:$release_ref"
if [[ $(git cat-file -t "$release_ref") != tag ]]; then
    echo 'remote release tag must be annotated' >&2
    exit 1
fi
release_commit=$(git rev-parse --verify "$release_ref^{commit}")
checkout_commit=$(git rev-parse --verify 'HEAD^{commit}')
if [[ $release_commit != "$checkout_commit" ]]; then
    echo 'release tag does not point to the checked-out commit' >&2
    exit 1
fi
