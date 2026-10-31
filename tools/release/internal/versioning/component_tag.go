// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package versioning

import (
	"fmt"
	"strings"
)

// TagComponent pushes one annotated component tag after all preparation checks.
func TagComponent(root, component, version string) error {
	if err := ValidateComponent(component, version); err != nil {
		return err
	}
	current, err := CheckComponent(root, component)
	if err != nil {
		return err
	}
	if current != version {
		return fmt.Errorf("requested version %s differs from VERSION %s", version, current)
	}
	if !greater(version, "0.1.0") {
		return fmt.Errorf("version must be greater than the existing public 0.1.0 release")
	}
	if _, err := ReleaseNotes(root, component, version); err != nil {
		return err
	}
	if err := clean(root); err != nil {
		return err
	}
	branch, err := git(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return err
	}
	if branch != "main" {
		return fmt.Errorf("tag must run from main after the version PR is merged")
	}
	if _, err := git(root, "fetch", "--no-tags", "origin", "refs/heads/main:refs/remotes/origin/main"); err != nil {
		return err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remoteHead, err := git(root, "rev-parse", "refs/remotes/origin/main")
	if err != nil {
		return err
	}
	if head != remoteHead {
		return fmt.Errorf("local main must match the freshly fetched origin/main")
	}
	refs, err := git(root, "ls-remote", "--tags", "origin")
	if err != nil {
		return err
	}
	names := []string{component + "/v" + version}
	for _, line := range strings.Split(refs, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSuffix(fields[1], "^{}"), "refs/tags/")
		if name == names[0] {
			return fmt.Errorf("remote tag already exists: %s", name)
		}
		previous := strings.TrimPrefix(name, component+"/v")
		if strings.HasPrefix(name, component+"/v") && stable.MatchString(previous) && !greater(version, previous) {
			return fmt.Errorf("version %s must be greater than remote release %s", version, previous)
		}
	}
	for _, name := range names {
		if _, err := git(root, "show-ref", "--verify", "--quiet", "refs/tags/"+name); err == nil {
			return fmt.Errorf("local tag already exists: %s", name)
		}
	}
	if err := clean(root); err != nil {
		return err
	}
	created := map[string]string{}
	rollback := func() {
		for name, oid := range created {
			current, err := git(root, "rev-parse", "refs/tags/"+name)
			if err == nil && current == oid {
				_, _ = git(root, "tag", "-d", name)
			}
		}
	}
	for _, name := range names {
		if _, err := git(root, "tag", "-a", name, head, "-m", "DBeaver Formatter Bridge "+name); err != nil {
			rollback()
			return err
		}
		oid, err := git(root, "rev-parse", "refs/tags/"+name)
		if err != nil {
			rollback()
			return err
		}
		created[name] = oid
	}
	if _, err := git(root, "-c", "push.followTags=false", "push", "--atomic", "origin", "refs/tags/"+names[0]+":refs/tags/"+names[0]); err != nil {
		rollback()
		return fmt.Errorf("atomic tag push failed; inspect remote refs before retrying: %w", err)
	}
	return nil
}
