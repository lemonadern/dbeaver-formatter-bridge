// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/p2site"
	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/versioning"
)

func validComponent(component string) bool {
	return component == "cli" || component == "plugin" || component == "vscode"
}
func componentPrefixes(component string) map[string]string {
	if component == "" {
		return artifactPrefixes
	}
	if !validComponent(component) {
		return map[string]string{}
	}
	return map[string]string{component: artifactPrefixes[component]}
}
func tagComponent(tag string) (string, error) {
	component, version, ok := strings.Cut(tag, "/v")
	if !ok || !validComponent(component) {
		return "", fmt.Errorf("expected plugin/vX.Y.Z, cli/vX.Y.Z or vscode/vX.Y.Z tag")
	}
	if err := versioning.ValidateComponent(component, version); err != nil {
		return "", err
	}
	return component, nil
}
func resolveComponent(root, component, tag string) (string, string, error) {
	selected, err := tagComponent(tag)
	if err != nil || selected != component {
		return "", "", fmt.Errorf("invalid component tag")
	}
	version := strings.TrimPrefix(tag, component+"/v")
	if compareVersions(version, "0.1.0") <= 0 {
		return "", "", fmt.Errorf("component release must be newer than the legacy 0.1.0 release")
	}
	actual, err := versioning.CheckComponent(root, component)
	if err != nil {
		return "", "", err
	}
	if actual != version {
		return "", "", fmt.Errorf("tag and component version do not match")
	}
	if _, err = versioning.ReleaseNotes(root, component, version); err != nil {
		return "", "", err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil || !shaRE.MatchString(head) {
		return "", "", fmt.Errorf("invalid Git commit")
	}
	tagged, err := git(root, "rev-parse", "refs/tags/"+tag+"^{}")
	if err != nil || tagged != head {
		return "", "", fmt.Errorf("component tag must point to HEAD")
	}
	kind, err := git(root, "cat-file", "-t", "refs/tags/"+tag)
	if err != nil || kind != "tag" {
		return "", "", fmt.Errorf("component release tag must be annotated")
	}
	for _, args := range [][]string{{"diff", "--quiet"}, {"diff", "--cached", "--quiet"}} {
		if _, err = git(root, args...); err != nil {
			return "", "", fmt.Errorf("release source has tracked changes")
		}
	}
	return version, head, nil
}
func releaseTitle(component, version string) string {
	switch component {
	case "plugin":
		return "DBeaver plugin v" + version
	case "cli":
		return "CLI v" + version
	case "vscode":
		return "VS Code extension v" + version
	}
	return "v" + version
}
func componentNotes(root, repo string, r record) (string, error) {
	text, err := versioning.ReleaseNotes(root, r.Component, r.Version)
	if err != nil {
		return "", err
	}
	body, err := notes(repo, r)
	if err != nil {
		return "", err
	}
	_, machine, _ := strings.Cut(body, markerStart)
	if strings.Contains(text, markerStart) || strings.Contains(text, markerEnd) {
		return "", fmt.Errorf("release notes contain reserved provenance marker")
	}
	return strings.TrimSpace(text) + fmt.Sprintf("\n\n## Verification\n\nCommit: `%s`\n\n[Build and test](https://github.com/%s/actions/runs/%d).\n\n", r.Commit, repo, r.RunID) + markerStart + machine, nil
}
func componentFiles(component, version string) map[string]bool {
	files := map[string]bool{"SHA256SUMS": true}
	switch component {
	case "cli":
		for _, system := range []string{"windows", "darwin", "linux"} {
			for _, arch := range []string{"amd64", "arm64"} {
				files["dbeaver-format-"+version+"-"+system+"-"+arch+".zip"] = true
			}
		}
	case "vscode":
		files["dbeaver-formatter-bridge-"+version+".vsix"] = true
	case "plugin":
		files["formatter-bridge-update-site-"+version+".zip"] = true
		for _, name := range []string{"p2.index", "content.jar", "content.xml.xz", "artifacts.jar", "artifacts.xml.xz", "com.github.lemonadern.dbeaver.formatter.bridge_" + version + ".jar", "com.github.lemonadern.dbeaver.formatter.bridge.feature_" + version + ".jar"} {
			files[name] = true
		}
	}
	return files
}
func collectComponent(root, component, version, sha, repo, input, out string) (map[string]string, error) {
	if !validComponent(component) {
		return nil, fmt.Errorf("invalid component")
	}
	expected := componentFiles(component, version)
	allowed := map[string]bool{}
	for name := range expected {
		if name != "SHA256SUMS" {
			allowed[name] = true
		}
	}
	switch component {
	case "cli":
		allowed["SHA256SUMS"] = true
	case "vscode":
		allowed["VSIX-SHA256SUMS"] = true
	case "plugin":
		allowed = map[string]bool{"formatter-bridge-update-site-" + version + ".zip": true}
	}
	dir := filepath.Join(input, component)
	found, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(found) != len(allowed) {
		return nil, fmt.Errorf("source artifact inventory mismatch: %s", component)
	}
	files := map[string][]byte{}
	for _, entry := range found {
		if !allowed[entry.Name()] {
			return nil, fmt.Errorf("unexpected source file: %s", entry.Name())
		}
		data, err := readRegular(filepath.Join(dir, entry.Name()), 128<<20)
		if err != nil {
			return nil, err
		}
		files[entry.Name()] = data
	}
	switch component {
	case "cli":
		expectedCLI := componentFiles(component, version)
		delete(expectedCLI, "SHA256SUMS")
		sums, err := parseSums(files["SHA256SUMS"], expectedCLI)
		if err != nil {
			return nil, err
		}
		delete(files, "SHA256SUMS")
		for _, system := range []string{"windows", "darwin", "linux"} {
			for _, arch := range []string{"amd64", "arm64"} {
				name := "dbeaver-format-" + version + "-" + system + "-" + arch + ".zip"
				data := files[name]
				if digest(data) != sums[name] {
					return nil, fmt.Errorf("CLI source checksum mismatch")
				}
				if err := verifyCLI(root, version, sha, system, arch, data); err != nil {
					return nil, err
				}
			}
		}
	case "vscode":
		name := "dbeaver-formatter-bridge-" + version + ".vsix"
		sums, err := parseSums(files["VSIX-SHA256SUMS"], map[string]bool{name: true})
		if err != nil {
			return nil, err
		}
		delete(files, "VSIX-SHA256SUMS")
		if digest(files[name]) != sums[name] {
			return nil, fmt.Errorf("VSIX source checksum mismatch")
		}
		if err := verifyVSIX(root, version, files[name]); err != nil {
			return nil, err
		}
	case "plugin":
		data := files["formatter-bridge-update-site-"+version+".zip"]
		if err := verifyPlugin(root, version, data); err != nil {
			return nil, err
		}
		base, err := p2site.ReleaseBase(repo, version)
		if err != nil {
			return nil, err
		}
		flat, err := p2site.Flatten(data, version, base)
		if err != nil {
			return nil, err
		}
		for name, body := range flat {
			if !expected[name] || name == "SHA256SUMS" || files[name] != nil {
				return nil, fmt.Errorf("unexpected flattened p2 file")
			}
			files[name] = body
		}
	}
	return writeCollection(files, expected, out)
}
func writeCollection(files map[string][]byte, expected map[string]bool, out string) (map[string]string, error) {
	if len(files) != len(expected)-1 {
		return nil, fmt.Errorf("distribution inventory mismatch")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if !expected[name] || name == "SHA256SUMS" {
			return nil, fmt.Errorf("unexpected distribution file")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sums, "%s  %s\n", digest(files[name]), name)
	}
	files["SHA256SUMS"] = []byte(sums.String())
	if err := os.MkdirAll(out, 0755); err != nil {
		return nil, err
	}
	existing, err := os.ReadDir(out)
	if err != nil {
		return nil, err
	}
	if len(existing) != 0 {
		return nil, fmt.Errorf("collection output must be empty")
	}
	hashes := map[string]string{}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, name), body, 0644); err != nil {
			return nil, err
		}
		hashes[name] = digest(body)
	}
	return hashes, nil
}

// pagesIndex reconstructs the index from all verified published plugin releases.
// Source Actions artifacts may expire: only immutable tags and public assets are used.
func pagesIndex(c *client, out string) error {
	children := []string{}
	seen := map[string]bool{}
	complete := false
	for page := 1; page <= 100; page++ {
		var list []release
		if err := c.api(fmt.Sprintf("releases?per_page=100&page=%d", page), &list); err != nil {
			return err
		}
		for _, rel := range list {
			if rel.Draft || rel.Prerelease || !strings.HasPrefix(rel.Tag, "plugin/") {
				continue
			}
			component, err := tagComponent(rel.Tag)
			if err != nil || component != "plugin" {
				return fmt.Errorf("invalid published plugin release tag")
			}
			version := strings.TrimPrefix(rel.Tag, "plugin/v")
			if seen[version] {
				return fmt.Errorf("duplicate plugin release")
			}
			seen[version] = true
			// The commit comes from the strict machine record, then the remote annotated tag is checked.
			var head struct {
				Commit string `json:"commit"`
			}
			_, rest, ok := strings.Cut(rel.Body, markerStart)
			if !ok {
				return fmt.Errorf("published plugin release has no provenance")
			}
			raw, _, ok := strings.Cut(rest, markerEnd)
			if !ok {
				return fmt.Errorf("invalid plugin provenance")
			}
			if err := json.Unmarshal([]byte(raw), &head); err != nil {
				return err
			}
			r, err := parseRecord(rel.Body, version, head.Commit)
			if err != nil {
				return err
			}
			if r.Component != "plugin" {
				return fmt.Errorf("plugin provenance component mismatch")
			}
			e := engine{component: "plugin", version: version, sha: r.Commit, tag: rel.Tag, client: c}
			if err = e.verifyTags(); err != nil {
				return err
			}
			if err = e.verifyRemote(&rel, r, true); err != nil {
				return err
			}
			base, err := p2site.ReleaseBase(c.repo, version)
			if err != nil {
				return err
			}
			children = append(children, base)
		}
		if len(list) < 100 {
			complete = true
			break
		}
	}
	if !complete {
		return fmt.Errorf("release listing exceeded pagination limit")
	}
	if len(children) == 0 {
		return fmt.Errorf("no verified published plugin releases; refusing to publish an empty index")
	}
	sort.Strings(children)
	files, err := p2site.Composite(children)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		return err
	}
	existing, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return fmt.Errorf("Pages output must be empty")
	}
	for name, body := range files {
		if filepath.Base(name) != name {
			return fmt.Errorf("unexpected composite filename")
		}
		if err = os.WriteFile(filepath.Join(out, name), body, 0644); err != nil {
			return err
		}
	}
	return nil
}

func compareVersions(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.ParseUint(left[i], 10, 31)
		y, _ := strconv.ParseUint(right[i], 10, 31)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
