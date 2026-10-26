// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package versioning

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var definitionFiles = []string{
	"VERSION", "plugin/pom.xml", "plugin/bundles/formatter-bridge/pom.xml", "plugin/features/formatter-bridge/pom.xml", "plugin/repository/pom.xml",
	"plugin/bundles/formatter-bridge/META-INF/MANIFEST.MF", "plugin/features/formatter-bridge/feature.xml",
	"extensions/vscode/package.json", "extensions/vscode/package-lock.json", "cli/internal/version/value.go",
}

func fixture(t *testing.T) string {
	t.Helper()
	root := rawFixture(t)
	if err := Prepare(root, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	return root
}

// rawFixture copies the checkout exactly, before any preparation can repair it.
func rawFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range definitionFiles {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func snapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for _, name := range definitionFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		result[name] = data
	}
	return result
}
func unchanged(t *testing.T, root string, before map[string][]byte) {
	t.Helper()
	after := snapshot(t, root)
	for name, data := range before {
		if !bytes.Equal(data, after[name]) {
			t.Errorf("unexpected change: %s", name)
		}
	}
}

func TestPrepareAndCheckAllDefinitions(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)
	if err := Prepare(root, "1.3.4"); err != nil {
		t.Fatal(err)
	}
	if version, err := Check(root); err != nil || version != "1.3.4" {
		t.Fatalf("version=%q error=%v", version, err)
	}
	after := snapshot(t, root)
	for _, name := range definitionFiles {
		if bytes.Equal(before[name], after[name]) {
			t.Errorf("version not updated: %s", name)
		}
	}
	if !bytes.Contains(after["plugin/pom.xml"], []byte("<version>${tycho.version}</version>")) {
		t.Fatal("dependency version was modified")
	}
	// Only the two product version fields change in the lockfile.
	expected := bytes.Replace(before["extensions/vscode/package-lock.json"], []byte(`"version": "0.1.0"`), []byte(`"version": "1.3.4"`), 2)
	if !bytes.Equal(expected, after["extensions/vscode/package-lock.json"]) {
		t.Fatal("lockfile dependencies or formatting changed")
	}
	stable := snapshot(t, root)
	if err := Prepare(root, "1.3.4"); err != nil {
		t.Fatal(err)
	}
	unchanged(t, root, stable)
}

func TestInvalidVersionsDoNotChangeFiles(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)
	for _, value := range []string{"", "v1.2.3", "1.2", "01.2.3", "1.2.3-beta.1", "1.2.3+build", "1.2.3\n", "2147483648.0.0", "-1.2.3", "2.0.0", "99.1.2"} {
		if err := Prepare(root, value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	unchanged(t, root, before)
	for _, value := range []string{"0.0.0", "1.2.3", "1.2147483647.0"} {
		if err := Validate(value); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
}

func TestMalformedLastInputDoesNotPartiallyPrepare(t *testing.T) {
	root := fixture(t)
	file := filepath.Join(root, "extensions/vscode/package-lock.json")
	if err := os.WriteFile(file, []byte(`{"version":"0.1.0","packages":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	if err := Prepare(root, "0.2.0"); err == nil {
		t.Fatal("accepted missing root package version")
	}
	unchanged(t, root, before)
}

func TestCheckDetectsDriftWithoutRepair(t *testing.T) {
	root := fixture(t)
	file := filepath.Join(root, "plugin/bundles/formatter-bridge/META-INF/MANIFEST.MF")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.ReplaceAll(data, []byte("Bundle-Version: 0.1.0"), []byte("Bundle-Version: 0.2.0")), 0644); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	if _, err := Check(root); err == nil || !strings.Contains(err.Error(), "out of sync") {
		t.Fatalf("error=%v", err)
	}
	unchanged(t, root, before)
	if err := Prepare(root, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(root); err != nil {
		t.Fatal(err)
	}
}

func TestJSONVersionsPreserveMinifiedDocumentsAndDependencies(t *testing.T) {
	data := []byte(`{"version":"0.1.0","packages":{"":{"version":"0.1.0"},"node_modules/a":{"version":"0.1.0"}},"other":{"version":"0.1.0"}}`)
	after, err := replaceJSON(data, "1.2.3", true)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte(`{"version":"1.2.3","packages":{"":{"version":"1.2.3"},"node_modules/a":{"version":"0.1.0"}},"other":{"version":"0.1.0"}}`)
	if !bytes.Equal(after, expected) {
		t.Fatalf("got %s", after)
	}
}

func TestStagingFailureLeavesAllDefinitionsUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permissions")
	}
	root := fixture(t)
	directory := filepath.Join(root, "plugin/bundles/formatter-bridge")
	if err := os.Chmod(directory, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0755) })
	before := snapshot(t, root)
	if err := Prepare(root, "0.2.0"); err == nil {
		t.Fatal("expected staging permission failure")
	}
	unchanged(t, root, before)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".release-version-") {
			t.Fatal("staged file leaked")
		}
	}
}

func TestWindowsCRLFCheckAndPrepare(t *testing.T) {
	root := rawFixture(t)
	for _, name := range definitionFiles {
		file := filepath.Join(root, name)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		// The checkout may already use CRLF on Windows. Normalize first so
		// converting it again never invents invalid CR-CR-LF line endings.
		lf := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		if err := os.WriteFile(file, bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n")), 0644); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot(t, root)
	if _, err := Check(root); err != nil {
		t.Fatal(err)
	}
	unchanged(t, root, before)
	if err := Prepare(root, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	if version, err := Check(root); err != nil || version != "0.2.0" {
		t.Fatalf("version=%q error=%v", version, err)
	}
	for _, name := range definitionFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("\r\n")) {
			t.Errorf("lost CRLF: %s", name)
		}
	}
}
