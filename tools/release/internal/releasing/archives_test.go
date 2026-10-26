// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func licenseFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"LICENSE", "COPYRIGHT"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("project "+name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func vsixEntries(t *testing.T, root string) map[string][]byte {
	t.Helper()
	entries := map[string][]byte{"[Content_Types].xml": []byte("content types"), "extension/readme.md": []byte("instructions"), "extension/out/src/extension.js": []byte("extension"), "extension/out/src/configuration.js": []byte("configuration"), "extension/out/src/formatter.js": []byte("formatter")}
	entries["extension.vsixmanifest"] = []byte(`<PackageManifest><Metadata><Identity Id="dbeaver-formatter-bridge" Version="0.2.0" Publisher="lemonadern"/></Metadata></PackageManifest>`)
	pkg := map[string]any{"name": "dbeaver-formatter-bridge", "publisher": "lemonadern", "version": testVersion, "main": "./out/src/extension.js", "license": "Apache-2.0", "extensionKind": []string{"ui"}, "capabilities": map[string]any{"untrustedWorkspaces": map[string]bool{"supported": false}}}
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	entries["extension/package.json"] = data
	for _, name := range []string{"LICENSE", "COPYRIGHT"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		dest := "extension/" + name
		if name == "LICENSE" {
			dest += ".txt"
		}
		entries[dest] = data
	}
	return entries
}
func TestVSIXValidatesBothContainerAndPackageIdentity(t *testing.T) {
	root := licenseFixture(t)
	entries := vsixEntries(t, root)
	if err := verifyVSIX(root, testVersion, makeZip(t, entries)); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`Version="0.2.0"`, `Id="dbeaver-formatter-bridge"`, `Publisher="lemonadern"`} {
		t.Run(field, func(t *testing.T) {
			entries := vsixEntries(t, root)
			entries["extension.vsixmanifest"] = bytes.Replace(entries["extension.vsixmanifest"], []byte(field), []byte(strings.Split(field, "=")[0]+`="wrong"`), 1)
			if err := verifyVSIX(root, testVersion, makeZip(t, entries)); err == nil {
				t.Fatal("accepted incorrect VSIX container")
			}
		})
	}
	entries = vsixEntries(t, root)
	entries["extension/package.json"] = bytes.Replace(entries["extension/package.json"], []byte(`"version":"0.2.0"`), []byte(`"version":"9.9.9"`), 1)
	if err := verifyVSIX(root, testVersion, makeZip(t, entries)); err == nil {
		t.Fatal("accepted incorrect package version")
	}
	entries = vsixEntries(t, root)
	entries["extension/CLI.exe"] = []byte("must not ship")
	if err := verifyVSIX(root, testVersion, makeZip(t, entries)); err == nil {
		t.Fatal("accepted unexpected payload")
	}
}
func pluginEntries(t *testing.T, root string) map[string][]byte {
	t.Helper()
	entries := map[string][]byte{}
	for _, name := range []string{"artifacts.jar", "artifacts.xml.xz", "content.jar", "content.xml.xz", "p2.index"} {
		entries[name] = []byte("p2 metadata")
	}
	for _, kind := range []string{"plugin", "feature"} {
		jar := map[string][]byte{}
		for _, name := range []string{"LICENSE", "COPYRIGHT"} {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			jar[name] = data
		}
		name := "plugins/com.github.lemonadern.dbeaver.formatter.bridge_0.2.0.jar"
		if kind == "plugin" {
			jar["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0\nBundle-Version: 0.2.0\n")
		} else {
			name = "features/com.github.lemonadern.dbeaver.formatter.bridge.feature_0.2.0.jar"
			jar["feature.xml"] = []byte(`<feature version="0.2.0"/>`)
		}
		entries[name] = makeZip(t, jar)
	}
	return entries
}
func TestPluginInventoryAndInternalVersions(t *testing.T) {
	root := licenseFixture(t)
	entries := pluginEntries(t, root)
	if err := verifyPlugin(root, testVersion, makeZip(t, entries)); err != nil {
		t.Fatal(err)
	}
	entries = pluginEntries(t, root)
	entries["dbeaver.jar"] = []byte("must not redistribute")
	if err := verifyPlugin(root, testVersion, makeZip(t, entries)); err == nil {
		t.Fatal("accepted unexpected root payload")
	}
	entries = pluginEntries(t, root)
	delete(entries, "p2.index")
	if err := verifyPlugin(root, testVersion, makeZip(t, entries)); err == nil {
		t.Fatal("accepted missing p2 metadata")
	}
	entries = pluginEntries(t, root)
	entries["plugins/com.github.lemonadern.dbeaver.formatter.bridge_0.2.0.jar"] = makeZip(t, map[string][]byte{"LICENSE": []byte("project LICENSE\n"), "COPYRIGHT": []byte("project COPYRIGHT\n"), "META-INF/MANIFEST.MF": []byte("Bundle-Version: 0.2.0.20261026\n")})
	if err := verifyPlugin(root, testVersion, makeZip(t, entries)); err == nil {
		t.Fatal("accepted qualifier build as release")
	}
}
func TestLicenseComparisonAllowsOnlyCheckoutLineEndings(t *testing.T) {
	root := licenseFixture(t)
	entries := map[string][]byte{"LICENSE": []byte("project LICENSE\r\n")}
	if err := verifyLicense(root, entries, "LICENSE"); err != nil {
		t.Fatal(err)
	}
	entries["LICENSE"] = []byte("changed LICENSE\r\n")
	if err := verifyLicense(root, entries, "LICENSE"); err == nil {
		t.Fatal("accepted changed license")
	}
}
