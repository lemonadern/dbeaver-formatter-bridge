// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package versioning

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var Components = []string{"plugin", "cli", "vscode"}

func validateStable(version string) error {
	if !stable.MatchString(version) {
		return fmt.Errorf("version must be X.Y.Z without prefixes, leading zeroes or prerelease suffixes")
	}
	for _, part := range strings.Split(version, ".") {
		if _, err := strconv.ParseUint(part, 10, 31); err != nil {
			return fmt.Errorf("version components must fit nonnegative 32-bit integers")
		}
	}
	return nil
}
func componentVersionFile(component string) string {
	switch component {
	case "plugin":
		return "plugin/VERSION"
	case "cli":
		return "cli/VERSION"
	case "vscode":
		return "extensions/vscode/VERSION"
	}
	return ""
}

// ValidateComponent keeps the Go module-path restriction specific to the CLI.
func ValidateComponent(component, version string) error {
	if componentVersionFile(component) == "" {
		return fmt.Errorf("unknown component %q: use plugin, cli or vscode", component)
	}
	if err := validateStable(version); err != nil {
		return err
	}
	if component == "cli" {
		return Validate(version)
	}
	return nil
}
func ReadComponent(root, component string) (string, error) {
	name := componentVersionFile(component)
	if name == "" {
		return "", fmt.Errorf("unknown component %q", component)
	}
	data, err := regularRead(root, name)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if err := ValidateComponent(component, value); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return value, nil
}

// CheckComponent checks version definitions; ReleaseNotes is the publication gate.
func CheckComponent(root, component string) (string, error) {
	version, err := ReadComponent(root, component)
	if err != nil {
		return "", err
	}
	changes, err := planComponent(root, version, component)
	if err != nil {
		return "", err
	}
	for _, c := range changes {
		if !bytes.Equal(c.before, c.after) {
			return "", fmt.Errorf("version definition out of sync: %s; run release prepare %s %s", c.name, component, version)
		}
	}
	return version, nil
}
func regularRead(root, name string) ([]byte, error) {
	current := root
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink not allowed: %s", name)
		}
	}
	info, err := os.Lstat(current)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("regular file required: %s", name)
	}
	return os.ReadFile(current)
}

// PrepareComponent changes only the selected component and snapshots its notes.
func PrepareComponent(root, component, version string) error {
	if err := ValidateComponent(component, version); err != nil {
		return err
	}
	changes, err := planComponent(root, version, component)
	if err != nil {
		return err
	}
	notesName := filepath.Join("releases", component, version+".md")
	pendingName := filepath.Join("releases", component, "unreleased.md")
	if _, err := os.Lstat(filepath.Join(root, notesName)); os.IsNotExist(err) {
		pending, err := regularRead(root, pendingName)
		if err != nil {
			return fmt.Errorf("read unreleased notes: %w", err)
		}
		if !bytes.HasPrefix(bytes.ReplaceAll(pending, []byte("\r\n"), []byte("\n")), []byte("# Unreleased\n")) {
			return fmt.Errorf("unreleased notes must start with # Unreleased")
		}
		note := bytes.Replace(pending, []byte("# Unreleased"), []byte("# "+component+" "+version), 1)
		changes = append(changes, change{name: notesName, after: note, mode: 0644})
		if validateNotes(strings.ReplaceAll(string(note), "\r\n", "\n"), component, version) == nil {
			changes = append(changes, change{name: pendingName, before: pending, after: []byte(notesTemplate()), mode: 0644})
		}
	} else if err != nil {
		return err
	} else if _, err := regularRead(root, notesName); err != nil {
		return err
	}
	return applyComponentChanges(root, changes)
}
func applyComponentChanges(root string, changes []change) error {
	staged := []change{}
	defer func() {
		for _, c := range staged {
			_ = os.Remove(c.temporary)
		}
	}()
	for _, c := range changes {
		if bytes.Equal(c.before, c.after) {
			continue
		}
		name := filepath.Join(root, c.name)
		if c.before != nil {
			current, err := regularRead(root, c.name)
			if err != nil || !bytes.Equal(current, c.before) {
				return fmt.Errorf("file changed while preparing: %s", c.name)
			}
		} else if _, err := os.Lstat(name); !os.IsNotExist(err) {
			return fmt.Errorf("new file already exists: %s", c.name)
		}
		f, err := os.CreateTemp(filepath.Dir(name), ".release-component-*")
		if err != nil {
			return err
		}
		c.temporary = f.Name()
		staged = append(staged, c)
		if _, err = f.Write(c.after); err == nil {
			err = f.Chmod(c.mode)
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	applied := []change{}
	rollback := func(cause error) error {
		for i := len(applied) - 1; i >= 0; i-- {
			c := applied[i]
			name := filepath.Join(root, c.name)
			current, err := regularRead(root, c.name)
			if err != nil || !bytes.Equal(current, c.after) {
				cause = fmt.Errorf("%w; concurrent edit prevents rollback: %s", cause, c.name)
				continue
			}
			if c.before == nil {
				err = os.Remove(name)
			} else {
				err = os.WriteFile(name, c.before, c.mode)
			}
			if err != nil {
				cause = fmt.Errorf("%w; rollback failed for %s: %v", cause, c.name, err)
			}
		}
		return cause
	}
	for _, c := range staged {
		name := filepath.Join(root, c.name)
		if c.before != nil {
			current, err := regularRead(root, c.name)
			if err != nil || !bytes.Equal(current, c.before) {
				return rollback(fmt.Errorf("file changed while preparing: %s", c.name))
			}
		} else if _, err := os.Lstat(name); !os.IsNotExist(err) {
			return rollback(fmt.Errorf("new file already exists: %s", c.name))
		}
		if err := os.Rename(c.temporary, name); err != nil {
			return rollback(err)
		}
		applied = append(applied, c)
	}
	return nil
}
func notesTemplate() string {
	return "# Unreleased\n\n## Summary\n\nTODO\n\n## Changes\n\nTODO\n\n## Compatibility\n\nTODO\n"
}

// ReleaseNotes loads an exact component/version and rejects unfinished drafts.
func ReleaseNotes(root, component, version string) (string, error) {
	if err := ValidateComponent(component, version); err != nil {
		return "", err
	}
	data, err := regularRead(root, filepath.Join("releases", component, version+".md"))
	if err != nil {
		return "", fmt.Errorf("release notes required for %s %s: %w", component, version, err)
	}
	notes := strings.ReplaceAll(string(data), "\r\n", "\n")
	if err := validateNotes(notes, component, version); err != nil {
		return "", err
	}
	return notes, nil
}
func Notes(root, component, version string) (string, error) {
	return ReleaseNotes(root, component, version)
}
func CheckNotes(root, component, version string) error {
	_, err := ReleaseNotes(root, component, version)
	return err
}
func validateNotes(notes, component, version string) error {
	if !strings.HasPrefix(notes, "# "+component+" "+version+"\n") {
		return fmt.Errorf("release notes title must identify %s %s", component, version)
	}
	upper := strings.ToUpper(notes)
	for _, marker := range []string{"TODO", "TBD", "FIXME", "PLACEHOLDER", "<!--", "[記入", "未記入"} {
		if strings.Contains(upper, marker) {
			return fmt.Errorf("release notes contain an unfinished placeholder: %s", marker)
		}
	}
	for _, heading := range []string{"Summary", "Changes", "Compatibility"} {
		marker := "\n## " + heading + "\n"
		start := strings.Index(notes, marker)
		if start < 0 {
			return fmt.Errorf("release notes require ## %s", heading)
		}
		section := notes[start+len(marker):]
		if next := strings.Index(section, "\n## "); next >= 0 {
			section = section[:next]
		}
		if strings.Trim(strings.TrimSpace(section), "-* ") == "" {
			return fmt.Errorf("release notes section %s is empty", heading)
		}
	}
	return nil
}
