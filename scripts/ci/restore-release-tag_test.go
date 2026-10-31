// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func releaseTagGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func releaseTagFixture(t *testing.T) (remote, checkout, tag, commit string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	checkout = filepath.Join(root, "checkout")
	tag = "plugin/v0.2.1"
	releaseTagGit(t, root, "init", "--bare", remote)
	releaseTagGit(t, root, "init", "-b", "main", seed)
	releaseTagGit(t, seed, "config", "user.name", "Release test")
	releaseTagGit(t, seed, "config", "user.email", "release-test@example.invalid")
	if err := os.WriteFile(filepath.Join(seed, "source.txt"), []byte("release source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	releaseTagGit(t, seed, "add", "source.txt")
	releaseTagGit(t, seed, "commit", "-m", "release source")
	commit = releaseTagGit(t, seed, "rev-parse", "HEAD")
	releaseTagGit(t, seed, "tag", "-a", tag, "-m", "component release")
	releaseTagGit(t, seed, "remote", "add", "origin", remote)
	releaseTagGit(t, seed, "push", "origin", "main", "refs/tags/"+tag)
	releaseTagGit(t, root, "clone", "--branch", "main", remote, checkout)
	// Reproduce checkout's +<event commit>:refs/tags/<tag> fetch destination.
	releaseTagGit(t, checkout, "update-ref", "refs/tags/"+tag, commit)
	if got := releaseTagGit(t, checkout, "cat-file", "-t", "refs/tags/"+tag); got != "commit" {
		t.Fatal("fixture did not create lightweight local ref")
	}
	return
}
func restoreReleaseTag(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate helper script")
	}
	script := filepath.Join(filepath.Dir(file), "restore-release-tag.sh")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
func TestRestoreReleaseTagRepairsCheckoutWithoutChangingOrigin(t *testing.T) {
	for _, component := range []string{"plugin", "cli", "vscode"} {
		t.Run(component, func(t *testing.T) {
			remote, checkout, oldTag, commit := releaseTagFixture(t)
			tag := component + "/v0.2.1"
			if component != "plugin" {
				original := releaseTagGit(t, remote, "rev-parse", "refs/tags/"+oldTag)
				releaseTagGit(t, remote, "update-ref", "refs/tags/"+tag, original)
				releaseTagGit(t, checkout, "update-ref", "refs/tags/"+tag, commit)
			}
			before := releaseTagGit(t, remote, "show-ref")
			if out, err := restoreReleaseTag(t, checkout, tag); err != nil {
				t.Fatalf("restore: %v\n%s", err, out)
			}
			if got := releaseTagGit(t, checkout, "cat-file", "-t", "refs/tags/"+tag); got != "tag" {
				t.Fatal("annotation not restored")
			}
			if got := releaseTagGit(t, checkout, "rev-parse", "refs/tags/"+tag+"^{commit}"); got != commit {
				t.Fatal("restored wrong commit")
			}
			if after := releaseTagGit(t, remote, "show-ref"); after != before {
				t.Fatal("modified remote refs")
			}
			// Re-execution is safe when checkout already contains the annotated tag.
			if out, err := restoreReleaseTag(t, checkout, tag); err != nil {
				t.Fatalf("repeat restore: %v\n%s", err, out)
			}
		})
	}
}
func TestRestoreReleaseTagRejectsWrongHeadAndRemoteLightweightTag(t *testing.T) {
	for _, failure := range []string{"wrong-head", "remote-lightweight"} {
		t.Run(failure, func(t *testing.T) {
			remote, checkout, tag, commit := releaseTagFixture(t)
			if failure == "wrong-head" {
				releaseTagGit(t, checkout, "-c", "user.name=Release test", "-c", "user.email=release-test@example.invalid", "commit", "--allow-empty", "-m", "different checkout")
			} else {
				releaseTagGit(t, remote, "update-ref", "refs/tags/"+tag, commit)
			}
			before := releaseTagGit(t, remote, "show-ref")
			if out, err := restoreReleaseTag(t, checkout, tag); err == nil {
				t.Fatalf("accepted %s: %s", failure, out)
			}
			if after := releaseTagGit(t, remote, "show-ref"); after != before {
				t.Fatal("modified remote refs after failure")
			}
		})
	}
}
func TestRestoreReleaseTagRejectsInvalidInputBeforeGit(t *testing.T) {
	// No Git repository: validation must reject these before any fetch attempt.
	for _, tag := range []string{"", "v0.2.1", "unknown/v0.2.1", "plugin/v00.2.1", "plugin/v0.2", "plugin/v0.2.1-rc.1", "plugin/v0.2.1/extra", "plugin/v0.2.1:refs/heads/main", "--all", "plugin/v0.2.1\ncli/v0.2.1"} {
		out, err := restoreReleaseTag(t, t.TempDir(), tag)
		if err == nil || !strings.Contains(out, "invalid component release tag") {
			t.Fatalf("invalid tag %q reached git: %v %s", tag, err, out)
		}
	}
	for _, args := range [][]string{nil, {"plugin/v0.2.1", "extra"}} {
		if out, err := restoreReleaseTag(t, t.TempDir(), args...); err == nil || !strings.Contains(out, "usage:") {
			t.Fatalf("accepted arity %v: %v %s", args, err, out)
		}
	}
}
