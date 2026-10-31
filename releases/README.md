# Release notes

Keep user-facing changes in `plugin/unreleased.md`, `cli/unreleased.md`, or `vscode/unreleased.md` in the same PR as the change.
Shared changes may require entries for more than one component.
Include behavior changes, installation steps, compatibility requirements and known limitations that affect users.

Run `go run ./scripts/release.go prepare <component> <version>` from the repository root to copy that component's unreleased notes to `<component>/<version>.md` and update only its version definitions.
Completed unreleased notes are consumed and reset to an empty template; incomplete notes are copied but publication will reject their placeholders.
Repeated preparation preserves an existing version's edited notes and leaves unreleased notes unchanged.
Review the versioned draft in the version preparation PR before merging.

Every versioned draft must begin with `# <component> <version>` and contain nonempty `Summary`, `Changes`, and `Compatibility` sections.
Remove all `TODO`, `TBD`, `FIXME`, `PLACEHOLDER`, and HTML comment placeholders before tagging.
The tag command and release preflight reject missing, empty or unfinished drafts.
The release publisher uses the checked draft as the human-readable GitHub Release body.
Released versions' notes are historical records: correct problems in a new release rather than republishing the same version.

The existing shared `v0.1.0` release remains unchanged; its component versions establish the initial version floor.
