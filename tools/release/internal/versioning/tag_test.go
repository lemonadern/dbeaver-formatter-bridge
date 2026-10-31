// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package versioning

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitOK(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := git(root, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func gitFixture(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := fixture(t)
	remote := filepath.Join(t.TempDir(), "origin 日本語.git")
	gitOK(t, root, "init", "--initial-branch=main")
	gitOK(t, root, "config", "user.name", "Release Test")
	gitOK(t, root, "config", "user.email", "release@example.invalid")
	gitOK(t, root, "config", "commit.gpgsign", "false")
	gitOK(t, root, "config", "tag.gpgsign", "false")
	gitOK(t, root, "add", ".")
	gitOK(t, root, "commit", "-m", "version definitions")
	gitOK(t, root, "init", "--bare", remote)
	gitOK(t, root, "remote", "add", "origin", remote)
	gitOK(t, root, "push", "-u", "origin", "main")
	return root, remote
}

func TestTagPushesTwoAnnotatedTagsToOneCommit(t *testing.T) {
	root, remote := gitFixture(t)
	if err := Tag(root, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	head := gitOK(t, root, "rev-parse", "HEAD")
	for _, name := range []string{"v0.1.0", "cli/v0.1.0"} {
		if got := gitOK(t, remote, "cat-file", "-t", "refs/tags/"+name); got != "tag" {
			t.Fatalf("not annotated: %s", name)
		}
		if got := gitOK(t, remote, "rev-parse", "refs/tags/"+name+"^{}"); got != head {
			t.Fatalf("tag %s points to %s", name, got)
		}
	}
	if err := Tag(root, "0.1.0"); err == nil {
		t.Fatal("existing published tag accepted")
	}
}

func TestTagRejectsDirtyUntrackedAndNonMain(t *testing.T) {
	root, _ := gitFixture(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("not committed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Tag(root, "0.1.0"); err == nil || !strings.Contains(err.Error(), "clean working tree") {
		t.Fatalf("error=%v", err)
	}
	if err := os.Remove(filepath.Join(root, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	gitOK(t, root, "switch", "-c", "release-draft")
	if err := Tag(root, "0.1.0"); err == nil || !strings.Contains(err.Error(), "from main") {
		t.Fatalf("error=%v", err)
	}
}

func TestTagRejectsPartialRemoteTagAndOlderVersion(t *testing.T) {
	for _, name := range []string{"cli/v0.1.0", "v0.2.0", "v2.0.0"} {
		t.Run(name, func(t *testing.T) {
			root, _ := gitFixture(t)
			gitOK(t, root, "tag", name)
			gitOK(t, root, "push", "origin", "refs/tags/"+name)
			gitOK(t, root, "tag", "-d", name)
			if err := Tag(root, "0.1.0"); err == nil {
				t.Fatal("conflicting remote release accepted")
			}
			if _, err := git(root, "rev-parse", "refs/tags/v0.1.0"); err == nil {
				t.Fatal("new product tag created on failure")
			}
		})
	}
}

func TestTagFetchesCurrentRemoteMain(t *testing.T) {
	root, remote := gitFixture(t)
	other := t.TempDir()
	gitOK(t, other, "clone", "--branch", "main", remote, ".")
	gitOK(t, other, "config", "user.name", "Other")
	gitOK(t, other, "config", "user.email", "other@example.invalid")
	gitOK(t, other, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(other, "new.txt"), []byte("new remote head"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOK(t, other, "add", ".")
	gitOK(t, other, "commit", "-m", "advance main")
	gitOK(t, other, "push", "origin", "main")
	if err := Tag(root, "0.1.0"); err == nil || !strings.Contains(err.Error(), "freshly fetched") {
		t.Fatalf("error=%v", err)
	}
}

func TestAtomicPushRejectionRollsBackLocalTags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX receive hook")
	}
	root, remote := gitFixture(t)
	if err := os.WriteFile(filepath.Join(remote, "hooks/pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Tag(root, "0.1.0"); err == nil || !strings.Contains(err.Error(), "atomic tag push failed") {
		t.Fatalf("error=%v", err)
	}
	for _, location := range []string{root, remote} {
		for _, name := range []string{"v0.1.0", "cli/v0.1.0"} {
			if _, err := git(location, "rev-parse", "refs/tags/"+name); err == nil {
				t.Fatalf("tag leaked after failed push: %s %s", location, name)
			}
		}
	}
}
