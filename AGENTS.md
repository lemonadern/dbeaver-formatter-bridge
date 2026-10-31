# Working on DBeaver Formatter Bridge

## Release notes are part of the change

For every user-facing change, update `releases/<component>/unreleased.md` in the same PR.
The components are `plugin`, `cli`, and `vscode`; shared behavior or protocol changes may affect several components.
Describe observable behavior, installation changes, compatibility requirements and known limitations.
Pure test, internal refactoring, CI and documentation changes do not require a user-facing entry unless they change distribution or installation behavior.
Review release-note coverage explicitly before reporting the work complete; automated checks can reject missing drafts but cannot judge whether their content covers every change.

## Preparing a release

Use `go run ./scripts/release.go prepare <component> <version>` from the root, then review the versioned draft at `releases/<component>/<version>.md` with the version definition changes.
Do not bump unaffected components or infer a version from commit messages.
Keep `Summary`, `Changes`, and `Compatibility` sections complete; remove placeholders before the version PR is merged and tagged.
Run version checks and the component's required tests before requesting release approval.
Do not create a release tag, push, publish, merge or change Pages settings unless the user has authorized that operation.

## Distribution invariants

Preserve the existing shared `v0.1.0` release and tags.
Use `plugin/vX.Y.Z`, `cli/vX.Y.Z`, and `vscode/vX.Y.Z` for new component releases.
The CLI tag also identifies the nested Go module; major 2 needs a Go module-path migration.
Plugin online p2 metadata and offline ZIP metadata have different artifact locations; retain local paths in offline ZIPs.
Publish only tested artifacts and verify their bytes after download; never replace published assets or force-update published tags.
