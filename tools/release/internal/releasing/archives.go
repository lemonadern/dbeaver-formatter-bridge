// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func digest(data []byte) string     { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func normalized(data []byte) []byte { return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")) }
func readRegular(name string, limit int64) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("not a bounded regular file: %s", filepath.Base(name))
	}
	return os.ReadFile(name)
}
func zipContents(data []byte, maxTotal uint64) (map[string][]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	entries := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(f.Name, "\\") || path.IsAbs(f.Name) || path.Clean(f.Name) != f.Name || f.Name == ".." || strings.HasPrefix(f.Name, "../") || f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe archive entry: %s", f.Name)
		}
		if _, ok := entries[f.Name]; ok {
			return nil, fmt.Errorf("duplicate archive entry: %s", f.Name)
		}
		if f.UncompressedSize64 > maxTotal-total {
			return nil, fmt.Errorf("archive exceeds uncompressed limit")
		}
		total += f.UncompressedSize64
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(r, int64(maxTotal)+1))
		closeErr := r.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if uint64(len(content)) != f.UncompressedSize64 {
			return nil, fmt.Errorf("archive size mismatch")
		}
		entries[f.Name] = content
	}
	return entries, nil
}
func verifyLicense(root string, entries map[string][]byte, name string) error {
	data, ok := entries[name]
	if !ok || len(data) == 0 {
		return fmt.Errorf("missing license: %s", name)
	}
	base := path.Base(name)
	if base == "LICENSE.txt" {
		base = "LICENSE"
	}
	expected, err := os.ReadFile(filepath.Join(root, base))
	if err != nil {
		return err
	}
	if !bytes.Equal(normalized(expected), normalized(data)) {
		return fmt.Errorf("license mismatch: %s", name)
	}
	return nil
}
func expectedFiles(version string) map[string]bool {
	files := map[string]bool{"formatter-bridge-update-site-" + version + ".zip": true, "dbeaver-formatter-bridge-" + version + ".vsix": true, "SHA256SUMS": true}
	for _, system := range []string{"windows", "darwin", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			files["dbeaver-format-"+version+"-"+system+"-"+arch+".zip"] = true
		}
	}
	return files
}
func parseSums(data []byte, expected map[string]bool) (map[string]string, error) {
	sums := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || !hashRE.MatchString(parts[0]) || !expected[parts[1]] {
			return nil, fmt.Errorf("invalid checksum entry")
		}
		if _, ok := sums[parts[1]]; ok {
			return nil, fmt.Errorf("duplicate checksum entry")
		}
		sums[parts[1]] = parts[0]
	}
	if len(sums) != len(expected) {
		return nil, fmt.Errorf("checksum inventory mismatch")
	}
	return sums, nil
}
func verifyCLI(root, version, sha, system, arch string, data []byte) error {
	entries, err := zipContents(data, 96<<20)
	if err != nil {
		return err
	}
	binary := "dbeaver-format"
	if system == "windows" {
		binary += ".exe"
	}
	allowed := map[string]bool{binary: true, "LICENSE": true, "COPYRIGHT": true, "BUILD-INFO.txt": true}
	licenseDir := filepath.Join(root, "licenses")
	local, err := os.ReadDir(licenseDir)
	if err != nil {
		return err
	}
	for _, f := range local {
		if f.Type().IsRegular() {
			allowed["licenses/"+f.Name()] = true
		}
	}
	if len(entries) != len(allowed) {
		return fmt.Errorf("CLI archive inventory mismatch")
	}
	for name := range entries {
		if !allowed[name] {
			return fmt.Errorf("unexpected CLI file: %s", name)
		}
	}
	for _, name := range []string{"LICENSE", "COPYRIGHT"} {
		if err := verifyLicense(root, entries, name); err != nil {
			return err
		}
	}
	var licenses struct {
		GoVersion string `json:"goVersion"`
		Files     []struct {
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	manifest, err := os.ReadFile(filepath.Join(licenseDir, "manifest.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(manifest, &licenses); err != nil {
		return err
	}
	if !bytes.Equal(normalized(manifest), normalized(entries["licenses/manifest.json"])) {
		return fmt.Errorf("CLI license manifest mismatch")
	}
	for _, file := range licenses.Files {
		if digest(entries["licenses/"+file.File]) != file.SHA256 {
			return fmt.Errorf("CLI third-party license mismatch: %s", file.File)
		}
	}
	info, err := buildinfo.Read(bytes.NewReader(entries[binary]))
	if err != nil {
		return fmt.Errorf("CLI binary build info: %w", err)
	}
	if info.GoVersion != licenses.GoVersion || info.Path != "github.com/lemonadern/dbeaver-formatter-bridge/cli/cmd/dbeaver-format" || info.Main.Path != "github.com/lemonadern/dbeaver-formatter-bridge/cli" || len(info.Deps) != 0 {
		return fmt.Errorf("unexpected CLI module/toolchain")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != system || settings["GOARCH"] != arch || settings["CGO_ENABLED"] != "0" || settings["vcs.revision"] != sha || settings["vcs.modified"] != "false" || settings["-trimpath"] != "true" {
		return fmt.Errorf("CLI provenance or target mismatch: %s/%s", system, arch)
	}
	if string(entries["BUILD-INFO.txt"]) != info.String() {
		return fmt.Errorf("CLI build-info file mismatch")
	}
	return nil
}
func verifyPlugin(root, version string, data []byte) error {
	entries, err := zipContents(data, 16<<20)
	if err != nil {
		return err
	}
	expected := map[string]string{"plugins/com.github.lemonadern.dbeaver.formatter.bridge_" + version + ".jar": "plugin", "features/com.github.lemonadern.dbeaver.formatter.bridge.feature_" + version + ".jar": "feature"}
	metadata := map[string]bool{"artifacts.jar": true, "artifacts.xml.xz": true, "content.jar": true, "content.xml.xz": true, "p2.index": true}
	if len(entries) != len(expected)+len(metadata) {
		return fmt.Errorf("p2 repository inventory mismatch")
	}
	for name, data := range entries {
		if _, ok := expected[name]; !ok && !metadata[name] {
			return fmt.Errorf("unexpected p2 repository file: %s", name)
		}
		if len(data) == 0 {
			return fmt.Errorf("empty p2 repository file: %s", name)
		}
	}

	for name := range entries {
		if strings.HasPrefix(name, "plugins/") || strings.HasPrefix(name, "features/") {
			if _, ok := expected[name]; !ok {
				return fmt.Errorf("unexpected plugin payload: %s", name)
			}
		}
	}
	for name, kind := range expected {
		body, ok := entries[name]
		if !ok {
			return fmt.Errorf("missing versioned plugin/feature")
		}
		jar, err := zipContents(body, 16<<20)
		if err != nil {
			return err
		}
		for _, license := range []string{"LICENSE", "COPYRIGHT"} {
			if err := verifyLicense(root, jar, license); err != nil {
				return err
			}
		}
		if kind == "plugin" {
			text := "\n" + string(normalized(jar["META-INF/MANIFEST.MF"]))
			if !strings.Contains(text, "\nBundle-Version: "+version+"\n") {
				return fmt.Errorf("plugin internal version mismatch")
			}
		} else {
			var feature struct {
				Version string `xml:"version,attr"`
			}
			if err := xml.Unmarshal(jar["feature.xml"], &feature); err != nil {
				return err
			}
			if feature.Version != version {
				return fmt.Errorf("feature internal version mismatch")
			}
		}
	}
	return nil
}
func verifyVSIX(root, version string, data []byte) error {
	entries, err := zipContents(data, 4<<20)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"[Content_Types].xml": true, "extension.vsixmanifest": true, "extension/package.json": true, "extension/readme.md": true, "extension/LICENSE.txt": true, "extension/COPYRIGHT": true, "extension/out/src/extension.js": true, "extension/out/src/configuration.js": true, "extension/out/src/formatter.js": true}
	if len(entries) != len(allowed) {
		return fmt.Errorf("VSIX inventory mismatch")
	}
	for name, content := range entries {
		if !allowed[name] || len(content) == 0 {
			return fmt.Errorf("unexpected or empty VSIX entry: %s", name)
		}
	}
	for _, name := range []string{"extension/LICENSE.txt", "extension/COPYRIGHT"} {
		if err := verifyLicense(root, entries, name); err != nil {
			return err
		}
	}
	var manifest struct {
		Name, Publisher, Version, Main, License string
		Dependencies                            map[string]any
		ExtensionKind                           []string
		Capabilities                            struct{ UntrustedWorkspaces struct{ Supported bool } }
	}
	if err := json.Unmarshal(entries["extension/package.json"], &manifest); err != nil {
		return err
	}
	var container struct {
		Metadata struct {
			Identity struct {
				Version   string `xml:"Version,attr"`
				Publisher string `xml:"Publisher,attr"`
				ID        string `xml:"Id,attr"`
			} `xml:"Identity"`
		} `xml:"Metadata"`
	}
	if err := xml.Unmarshal(entries["extension.vsixmanifest"], &container); err != nil {
		return err
	}
	if container.Metadata.Identity.Version != version || container.Metadata.Identity.Publisher != "lemonadern" || container.Metadata.Identity.ID != "dbeaver-formatter-bridge" {
		return fmt.Errorf("VSIX container version/identity mismatch")
	}
	if manifest.Name != "dbeaver-formatter-bridge" || manifest.Publisher != "lemonadern" || manifest.Version != version || manifest.Main != "./out/src/extension.js" || manifest.License != "Apache-2.0" || len(manifest.Dependencies) != 0 || len(manifest.ExtensionKind) != 1 || manifest.ExtensionKind[0] != "ui" || manifest.Capabilities.UntrustedWorkspaces.Supported {
		return fmt.Errorf("VSIX metadata mismatch")
	}
	return nil
}
func collect(root, version, sha, input, out string) (map[string]string, error) {
	files := map[string][]byte{}
	cliExpected := map[string]bool{}
	for _, system := range []string{"windows", "darwin", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			cliExpected["dbeaver-format-"+version+"-"+system+"-"+arch+".zip"] = true
		}
	}
	for component, allowed := range map[string]map[string]bool{
		"cli": func() map[string]bool {
			m := map[string]bool{"SHA256SUMS": true}
			for name := range cliExpected {
				m[name] = true
			}
			return m
		}(),
		"plugin": {"formatter-bridge-update-site-" + version + ".zip": true},
		"vscode": {"dbeaver-formatter-bridge-" + version + ".vsix": true, "VSIX-SHA256SUMS": true},
	} {
		found, err := os.ReadDir(filepath.Join(input, component))
		if err != nil {
			return nil, err
		}
		if len(found) != len(allowed) {
			return nil, fmt.Errorf("source artifact inventory mismatch: %s", component)
		}
		for _, f := range found {
			if !allowed[f.Name()] {
				return nil, fmt.Errorf("unexpected source artifact file: %s", f.Name())
			}
		}
	}
	cliSumsData, err := readRegular(filepath.Join(input, "cli", "SHA256SUMS"), 16<<10)
	if err != nil {
		return nil, err
	}
	cliSums, err := parseSums(cliSumsData, cliExpected)
	if err != nil {
		return nil, err
	}
	for _, system := range []string{"windows", "darwin", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "dbeaver-format-" + version + "-" + system + "-" + arch + ".zip"
			data, err := readRegular(filepath.Join(input, "cli", name), 64<<20)
			if err != nil {
				return nil, err
			}
			if digest(data) != cliSums[name] {
				return nil, fmt.Errorf("CLI source checksum mismatch: %s", name)
			}
			if err := verifyCLI(root, version, sha, system, arch, data); err != nil {
				return nil, err
			}
			files[name] = data
		}
	}
	plugin := "formatter-bridge-update-site-" + version + ".zip"
	data, err := readRegular(filepath.Join(input, "plugin", plugin), 16<<20)
	if err != nil {
		return nil, err
	}
	if err := verifyPlugin(root, version, data); err != nil {
		return nil, err
	}
	files[plugin] = data
	vsix := "dbeaver-formatter-bridge-" + version + ".vsix"
	data, err = readRegular(filepath.Join(input, "vscode", vsix), 4<<20)
	if err != nil {
		return nil, err
	}
	vsixSumsData, err := readRegular(filepath.Join(input, "vscode", "VSIX-SHA256SUMS"), 16<<10)
	if err != nil {
		return nil, err
	}
	vsixSums, err := parseSums(vsixSumsData, map[string]bool{vsix: true})
	if err != nil {
		return nil, err
	}
	if digest(data) != vsixSums[vsix] {
		return nil, fmt.Errorf("VSIX source checksum mismatch")
	}
	if err := verifyVSIX(root, version, data); err != nil {
		return nil, err
	}
	files[vsix] = data
	names := make([]string, 0, len(files))
	for name := range files {
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
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(out, name), content, 0644); err != nil {
			return nil, err
		}
		hashes[name] = digest(content)
	}
	return hashes, nil
}
