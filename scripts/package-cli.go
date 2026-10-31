//go:build ignore

// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Package the six default Go CLI builds with project and upstream licenses.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const licensedGoVersion = "go1.27.2"

var cliTargets = []struct{ os, arch string }{{"windows", "amd64"}, {"windows", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}}

type archiveEntry struct {
	name string
	data []byte
	mode fs.FileMode
}

func main() {
	input := flag.String("in", "build", "directory containing six CLI binaries")
	out := flag.String("out", "dist", "archive output directory")
	flag.Parse()
	root, err := os.Getwd()
	if err == nil {
		err = packageCLI(root, *input, *out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func packageCLI(root, input, out string) error {
	if !filepath.IsAbs(input) {
		input = filepath.Join(root, input)
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	versionData, err := os.ReadFile(filepath.Join(root, "cli", "VERSION"))
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(versionData))
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) {
		return fmt.Errorf("invalid VERSION")
	}
	var shared []archiveEntry
	for _, name := range []string{"LICENSE", "COPYRIGHT"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("empty %s", name)
		}
		shared = append(shared, archiveEntry{name, data, 0644})
	}
	licenses, err := licenseEntries(root)
	if err != nil {
		return err
	}
	shared = append(shared, licenses...)
	type artifact struct {
		name    string
		entries []archiveEntry
	}
	var artifacts []artifact
	for _, target := range cliTargets {
		suffix := ""
		if target.os == "windows" {
			suffix = ".exe"
		}
		name := "dbeaver-format-" + target.os + "-" + target.arch
		path := filepath.Join(input, name+suffix)
		if err := validateBinary(path, target.os, target.arch); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries := append([]archiveEntry(nil), shared...)
		entries = append(entries, archiveEntry{"dbeaver-format" + suffix, data, 0755})
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, archiveEntry{"BUILD-INFO.txt", []byte(info.String()), 0644})
		artifacts = append(artifacts, artifact{"dbeaver-format-" + version + "-" + target.os + "-" + target.arch + ".zip", entries})
	}
	// Validate every input before creating output; leave unrelated distributions intact.
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(out, ".package-cli-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var sums []string
	for _, artifact := range artifacts {
		path := filepath.Join(stage, artifact.name)
		if err := writeArchive(path, artifact.entries); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		sums = append(sums, hex.EncodeToString(digest[:])+"  "+artifact.name)
	}
	sort.Strings(sums)
	if err := os.WriteFile(filepath.Join(stage, "SHA256SUMS"), []byte(strings.Join(sums, "\n")+"\n"), 0644); err != nil {
		return err
	}
	names := []string{"SHA256SUMS"}
	for _, artifact := range artifacts {
		names = append(names, artifact.name)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(stage, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0644); err != nil {
			return err
		}
	}
	fmt.Printf("Packaged %d CLI archives in %s\n", len(artifacts), out)
	return nil
}

func licenseEntries(root string) ([]archiveEntry, error) {
	dir := filepath.Join(root, "licenses")
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		GoVersion string `json:"goVersion"`
		Files     []struct{ File, SHA256 string }
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.GoVersion != licensedGoVersion {
		return nil, fmt.Errorf("license inventory Go version mismatch")
	}
	expected := map[string]string{}
	for _, file := range manifest.Files {
		if filepath.Base(file.File) != file.File || file.File == "." || file.File == "" {
			return nil, fmt.Errorf("unsafe license filename")
		}
		if _, exists := expected[file.File]; exists {
			return nil, fmt.Errorf("duplicate license filename")
		}
		expected[file.File] = file.SHA256
	}
	if len(expected) == 0 {
		return nil, fmt.Errorf("empty license inventory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []archiveEntry
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("license must be a regular file: %s", entry.Name())
		}
		contents, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if digest, wanted := expected[entry.Name()]; wanted {
			actual := sha256.Sum256(contents)
			if hex.EncodeToString(actual[:]) != digest {
				return nil, fmt.Errorf("license hash mismatch: %s", entry.Name())
			}
			delete(expected, entry.Name())
		} else if entry.Name() != "README.md" && entry.Name() != "manifest.json" {
			return nil, fmt.Errorf("unlisted license file: %s", entry.Name())
		}
		result = append(result, archiveEntry{"licenses/" + entry.Name(), contents, 0644})
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("missing inventoried license files")
	}
	return result, nil
}

func validateBinary(path, goos, goarch string) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("binary must be a regular file: %s", path)
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read binary provenance %s: %w", path, err)
	}
	if info.GoVersion != licensedGoVersion {
		return fmt.Errorf("%s uses %s; license inventory covers %s", path, info.GoVersion, licensedGoVersion)
	}
	if info.Path != "github.com/lemonadern/dbeaver-formatter-bridge/cli/cmd/dbeaver-format" || info.Main.Path != "github.com/lemonadern/dbeaver-formatter-bridge/cli" || len(info.Deps) != 0 {
		return fmt.Errorf("unexpected module dependencies in %s", path)
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != goos || settings["GOARCH"] != goarch || settings["CGO_ENABLED"] != "0" || settings["GOEXPERIMENT"] != "" || settings["-tags"] != "" || (settings["GOFIPS140"] != "" && settings["GOFIPS140"] != "off") || settings["-race"] == "true" || settings["-msan"] == "true" || settings["-asan"] == "true" {
		return fmt.Errorf("unsupported build settings in %s; rebuild with the default build-cli.go configuration", path)
	}
	return nil
}

func writeArchive(path string, entries []archiveEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		header.SetMode(entry.mode)
		dest, err := writer.CreateHeader(header)
		if err != nil {
			writer.Close()
			file.Close()
			return err
		}
		if _, err := dest.Write(entry.data); err != nil {
			writer.Close()
			file.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
