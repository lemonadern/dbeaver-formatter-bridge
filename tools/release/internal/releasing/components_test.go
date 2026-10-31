// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func componentFake(t *testing.T, component string) (*fakeGH, *engine) {
	t.Helper()
	f := fake(t)
	e := f.engine()
	e.component = component
	e.tag = component + "/v" + testVersion
	only := map[string][]byte{}
	for name := range componentFiles(component, testVersion) {
		only[name] = []byte("distribution: " + name)
	}
	f.data = only
	base := f.execute
	e.client = newClient("owner/repo", func(args []string) ([]byte, error) {
		raw, err := base(args)
		if err != nil {
			return nil, err
		}
		for _, a := range args {
			if a == "repos/owner/repo/actions/runs/23" {
				var value map[string]any
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, err
				}
				value["head_branch"] = e.tag
				return json.Marshal(value)
			}
		}
		return raw, nil
	})
	dir := filepath.Join(e.root, "releases", component)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("# %s %s\n\n## Summary\n\nUser-facing feature.\n\n## Changes\n\nSpecific fix.\n\n## Compatibility\n\nOther components 0.1.0 or later.\n", component, testVersion)
	if err := os.WriteFile(filepath.Join(dir, testVersion+".md"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return f, e
}
func TestComponentPublicationUsesOnlySelectedArtifactAndAuthoredNotes(t *testing.T) {
	for _, component := range []string{"plugin", "cli", "vscode"} {
		t.Run(component, func(t *testing.T) {
			f, e := componentFake(t, component)
			if component != "cli" {
				for _, j := range f.jobs {
					if strings.HasSuffix(j["name"].(string), "Go install / Tagged module") {
						j["conclusion"] = "skipped"
					}
				}
			}
			if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
				t.Fatal(err)
			}
			r, err := parseRecord(f.rel.Body, testVersion, testSHA)
			if err != nil {
				t.Fatal(err)
			}
			if r.Schema != 2 || r.Component != component || len(r.Artifacts) != 1 || len(r.Files) != len(componentFiles(component, testVersion)) {
				t.Fatal("mixed component provenance", r)
			}
			if !strings.Contains(f.rel.Body, "User-facing feature.") || strings.Contains(f.rel.Body, "Format SQL through the formatter") {
				t.Fatal("authored release notes were not used")
			}
			for other, ref := range f.refs {
				if other != component {
					for _, request := range f.requests {
						if strings.Contains(request, fmt.Sprintf("actions/artifacts/%d", ref.ID)) {
							t.Fatal("downloaded unrelated artifact", request)
						}
					}
				}
			}
			// Re-executing after source artifacts expire must be read-only.
			f.sources = map[int64][]byte{}
			f.metas = map[int64]artifactMeta{}
			f.writes = nil
			if err := e.publish("", testRun, 2, nil, false); err != nil {
				t.Fatal(err)
			}
			noWrites(t, f)
		})
	}
}
func TestComponentRecordRejectsWrongOrExtraArtifacts(t *testing.T) {
	f, e := componentFake(t, "vscode")
	if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	r, err := parseRecord(f.rel.Body, testVersion, testSHA)
	if err != nil {
		t.Fatal(err)
	}
	r.Artifacts["cli"] = f.refs["cli"]
	if err := r.validate(testVersion, testSHA); err == nil {
		t.Fatal("accepted unrelated artifact")
	}
	delete(r.Artifacts, "cli")
	r.Component = "plugin"
	if err := r.validate(testVersion, testSHA); err == nil {
		t.Fatal("accepted wrong component")
	}
}
func TestComponentPublicationNeedsAuthoredNotesBeforeWrites(t *testing.T) {
	f, e := componentFake(t, "vscode")
	if err := os.Remove(filepath.Join(e.root, "releases", "vscode", testVersion+".md")); err != nil {
		t.Fatal(err)
	}
	if err := e.publish("", testRun, 1, f.ids(), false); err == nil {
		t.Fatal("accepted missing authored notes")
	}
	noWrites(t, f)
}
func TestComponentCollectionDoesNotRequireUnrelatedArtifacts(t *testing.T) {
	root := licenseFixture(t)
	input := t.TempDir()
	dir := filepath.Join(input, "vscode")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "dbeaver-formatter-bridge-" + testVersion + ".vsix"
	data := makeZip(t, vsixEntries(t, root))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VSIX-SHA256SUMS"), []byte(digest(data)+"  "+name+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	hashes, err := collectComponent(root, "vscode", testVersion, testSHA, "owner/repo", input, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[name] != digest(data) {
		t.Fatal(hashes)
	}
	actual, err := os.ReadFile(filepath.Join(out, name))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("collection changed VSIX")
	}
}
func TestPagesIndexVerifiesOnlyPublishedPluginReleases(t *testing.T) {
	f, e := componentFake(t, "plugin")
	if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	f.writes = nil
	out := filepath.Join(t.TempDir(), "site")
	if err := pagesIndex(e.client, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "compositeContent.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "releases/download/plugin%2Fv0.2.0/") && !strings.Contains(string(data), "releases/download/plugin/v0.2.0/") {
		t.Fatal(string(data))
	}
	noWrites(t, f)
	for _, a := range f.rel.Assets {
		f.remote[a.ID] = []byte("corrupt")
		break
	}
	if err := pagesIndex(e.client, filepath.Join(t.TempDir(), "site")); err == nil {
		t.Fatal("accepted corrupt public plugin release")
	}
	noWrites(t, f)
}
func TestPagesIndexRefusesDraftPrereleaseLegacyAndWrongProvenance(t *testing.T) {
	for _, kind := range []string{"draft", "prerelease", "legacy", "wrong-component"} {
		t.Run(kind, func(t *testing.T) {
			f, e := componentFake(t, "plugin")
			if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
				t.Fatal(err)
			}
			f.writes = nil
			switch kind {
			case "draft":
				f.rel.Draft = true
			case "prerelease":
				f.rel.Prerelease = true
			case "legacy":
				f.rel.Body = "legacy"
			case "wrong-component":
				f.rel.Body = strings.Replace(f.rel.Body, `"component": "plugin"`, `"component": "vscode"`, 1)
			}
			if err := pagesIndex(e.client, filepath.Join(t.TempDir(), "site")); err == nil {
				t.Fatal("accepted", kind)
			}
			noWrites(t, f)
		})
	}
}

func TestPagesProofBindsAllAPIValidatedAssetsAndRemainsOutsideSite(t *testing.T) {
	f, e := componentFake(t, "plugin")
	if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	site := filepath.Join(dir, "site")
	proofPath := filepath.Join(dir, "proof.json")
	if err := pagesIndexWithProof(e.client, site, proofPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal(err)
	}
	var proof map[string]map[string]string
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	if len(proof) != 1 {
		t.Fatal(proof)
	}
	for base, hashes := range proof {
		if !strings.Contains(base, "plugin%2Fv"+testVersion) {
			t.Fatal(base)
		}
		if len(hashes) != len(f.data) {
			t.Fatal("incomplete proof")
		}
		for name, data := range f.data {
			if hashes[name] != digest(data) {
				t.Fatal("proof not bound to verified API bytes", name)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(site, "proof.json")); !os.IsNotExist(err) {
		t.Fatal("proof exposed in published site")
	}
	if err := pagesIndexWithProof(e.client, filepath.Join(dir, "second"), proofPath); err == nil {
		t.Fatal("overwrote existing proof")
	}
	inside := filepath.Join(dir, "inside")
	if err := pagesIndexWithProof(e.client, inside, filepath.Join(inside, "proof.json")); err == nil {
		t.Fatal("accepted proof inside site")
	}
}
