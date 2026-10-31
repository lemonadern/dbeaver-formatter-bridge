// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package versioning

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func componentFixture(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	for _, c := range Components {
		name := filepath.Join(root, componentVersionFile(c))
		if err := os.WriteFile(name, []byte("0.1.0\n"), 0644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "releases", c)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "unreleased.md"), []byte(notesTemplate()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func completedNotes(component, version string) string {
	return "# " + component + " " + version + "\n\n## Summary\n\nA user-facing change.\n\n## Changes\n\n- Explain the changed behavior.\n\n## Compatibility\n\nRequires the supported CLI and DBeaver versions.\n"
}
func writeCompleteNotes(t *testing.T, root, component, version string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "releases", component, version+".md"), []byte(completedNotes(component, version)), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareComponentIsolatesVersionsAndPreservesEditedDraft(t *testing.T) {
	for _, component := range Components {
		t.Run(component, func(t *testing.T) {
			root := componentFixture(t)
			before := snapshot(t, root)
			if err := PrepareComponent(root, component, "0.2.0"); err != nil {
				t.Fatal(err)
			}
			for _, other := range Components {
				want := "0.1.0"
				if other == component {
					want = "0.2.0"
				}
				got, err := CheckComponent(root, other)
				if err != nil || got != want {
					t.Fatalf("%s = %s %v", other, got, err)
				}
			}
			// Legacy product VERSION must not be used or updated by component commands.
			after, err := os.ReadFile(filepath.Join(root, "VERSION"))
			if err != nil || !bytes.Equal(after, before["VERSION"]) {
				t.Fatal("legacy VERSION changed")
			}
			if _, err := ReleaseNotes(root, component, "0.2.0"); err == nil {
				t.Fatal("placeholder draft accepted")
			}
			writeCompleteNotes(t, root, component, "0.2.0")
			if err := PrepareComponent(root, component, "0.2.0"); err != nil {
				t.Fatal(err)
			}
			got, err := ReleaseNotes(root, component, "0.2.0")
			if err != nil || got != completedNotes(component, "0.2.0") {
				t.Fatalf("edited draft lost: %v", err)
			}
		})
	}
}
func TestCompleteUnreleasedNotesAreSnapshottedAndConsumed(t *testing.T) {
	root := componentFixture(t)
	pending := strings.Replace(completedNotes("plugin", "0.2.0"), "# plugin 0.2.0", "# Unreleased", 1)
	if err := os.WriteFile(filepath.Join(root, "releases/plugin/unreleased.md"), []byte(pending), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PrepareComponent(root, "plugin", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReleaseNotes(root, "plugin", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "releases/plugin/unreleased.md"))
	if err != nil || string(data) != notesTemplate() {
		t.Fatal("pending notes not reset")
	}
}
func TestComponentValidationDoesNotShareGoMajorRestriction(t *testing.T) {
	if err := ValidateComponent("cli", "2.0.0"); err == nil {
		t.Fatal("CLI major 2 accepted")
	}
	for _, c := range []string{"plugin", "vscode"} {
		root := componentFixture(t)
		if err := PrepareComponent(root, c, "2.0.0"); err != nil {
			t.Fatal(err)
		}
		if _, err := CheckComponent(root, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []string{"", "../cli", "product"} {
		if err := ValidateComponent(c, "0.2.0"); err == nil {
			t.Fatal("unknown component accepted")
		}
	}
}
func TestReleaseNotesRejectMissingEmptyAndPlaceholderSections(t *testing.T) {
	root := componentFixture(t)
	if _, err := ReleaseNotes(root, "cli", "0.2.0"); err == nil {
		t.Fatal("missing notes accepted")
	}
	for _, text := range []string{notesTemplate(), "# cli 0.2.0\n", strings.Replace(completedNotes("cli", "0.2.0"), "A user-facing change.", "", 1), strings.Replace(completedNotes("cli", "0.2.0"), "A user-facing change.", "TODO", 1), strings.Replace(completedNotes("cli", "0.2.0"), "# cli 0.2.0", "# plugin 0.2.0", 1)} {
		if err := os.WriteFile(filepath.Join(root, "releases/cli/0.2.0.md"), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReleaseNotes(root, "cli", "0.2.0"); err == nil {
			t.Fatalf("invalid notes accepted: %q", text)
		}
	}
}
func componentGitFixture(t *testing.T, component, version string) (string, string) {
	t.Helper()
	root := componentFixture(t)
	if err := PrepareComponent(root, component, version); err != nil {
		t.Fatal(err)
	}
	writeCompleteNotes(t, root, component, version)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitOK(t, root, "init", "--initial-branch=main")
	gitOK(t, root, "config", "user.name", "Release Test")
	gitOK(t, root, "config", "user.email", "release@example.invalid")
	gitOK(t, root, "config", "commit.gpgsign", "false")
	gitOK(t, root, "config", "tag.gpgsign", "false")
	gitOK(t, root, "add", ".")
	gitOK(t, root, "commit", "-m", "component preparation")
	gitOK(t, root, "init", "--bare", remote)
	gitOK(t, root, "remote", "add", "origin", remote)
	gitOK(t, root, "push", "-u", "origin", "main")
	return root, remote
}
func TestTagComponentPushesOnlyItsAnnotatedTag(t *testing.T) {
	root, remote := componentGitFixture(t, "plugin", "0.2.0")
	// A newer unrelated release must not affect this component's version ordering.
	gitOK(t, root, "tag", "cli/v1.0.0")
	gitOK(t, root, "push", "origin", "refs/tags/cli/v1.0.0")
	if err := TagComponent(root, "plugin", "0.2.0"); err != nil {
		t.Fatal(err)
	}
	if got := gitOK(t, remote, "cat-file", "-t", "refs/tags/plugin/v0.2.0"); got != "tag" {
		t.Fatal(got)
	}
	if got := gitOK(t, remote, "rev-parse", "refs/tags/plugin/v0.2.0^{}"); got != gitOK(t, root, "rev-parse", "HEAD") {
		t.Fatal("wrong target")
	}
	for _, tag := range []string{"v0.2.0", "cli/v0.2.0", "vscode/v0.2.0"} {
		if _, err := git(remote, "rev-parse", "refs/tags/"+tag); err == nil {
			t.Fatal("unrelated tag created")
		}
	}
	if err := TagComponent(root, "plugin", "0.2.0"); err == nil {
		t.Fatal("existing tag accepted")
	}
}
func TestTagComponentRejectsLegacyFloorAndUnfinishedNotes(t *testing.T) {
	root, _ := componentGitFixture(t, "plugin", "0.1.0")
	if err := TagComponent(root, "plugin", "0.1.0"); err == nil {
		t.Fatal("same legacy version accepted")
	}
	root, _ = componentGitFixture(t, "cli", "0.2.0")
	if err := os.WriteFile(filepath.Join(root, "releases/cli/0.2.0.md"), []byte("# cli 0.2.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOK(t, root, "add", ".")
	gitOK(t, root, "commit", "-m", "unfinished notes")
	gitOK(t, root, "push", "origin", "main")
	if err := TagComponent(root, "cli", "0.2.0"); err == nil || !strings.Contains(err.Error(), "release notes") {
		t.Fatalf("error=%v", err)
	}
}
