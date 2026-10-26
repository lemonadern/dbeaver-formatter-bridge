// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) == "scripts" {
		root = filepath.Dir(root)
	}
	return root
}

func TestSixDistributionArchives(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "build", "dbeaver-format-darwin-arm64")); os.IsNotExist(err) {
		t.Skip("run go run scripts/build-cli.go first for six-platform packaging integration")
	}
	out := filepath.Join(t.TempDir(), "dist")
	if err := packageCLI(root, "build", out); err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 6 {
		t.Fatalf("checksums=%d", len(lines))
	}
	hashes := map[string]string{}
	for _, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 {
			t.Fatalf("invalid checksum: %q", line)
		}
		hashes[parts[1]] = parts[0]
	}
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	var first map[string][]byte = map[string][]byte{}
	for _, target := range cliTargets {
		stem := "dbeaver-format-" + target.os + "-" + target.arch
		version, err := os.ReadFile(filepath.Join(root, "VERSION"))
		if err != nil {
			t.Fatal(err)
		}
		archive := "dbeaver-format-" + strings.TrimSpace(string(version)) + "-" + target.os + "-" + target.arch + ".zip"
		data, err := os.ReadFile(filepath.Join(out, archive))
		if err != nil {
			t.Fatal(err)
		}
		first[archive] = data
		sum := sha256.Sum256(data)
		if hashes[archive] != hex.EncodeToString(sum[:]) {
			t.Fatalf("checksum mismatch %s", archive)
		}
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		contents := map[string][]byte{}
		binary := "dbeaver-format"
		if target.os == "windows" {
			binary += ".exe"
		}
		for _, file := range reader.File {
			if filepath.IsAbs(file.Name) || strings.Contains(file.Name, "..") || strings.Contains(file.Name, "\\") {
				t.Fatalf("unsafe entry %s", file.Name)
			}
			if _, ok := contents[file.Name]; ok {
				t.Fatalf("duplicate entry %s", file.Name)
			}
			stream, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			contents[file.Name], err = io.ReadAll(stream)
			stream.Close()
			if err != nil {
				t.Fatal(err)
			}
			if file.Modified.Year() != 1980 {
				t.Fatalf("non-fixed timestamp %s", file.Name)
			}
			wanted := os.FileMode(0644)
			if file.Name == binary {
				wanted = 0755
			}
			if file.Mode().Perm() != wanted {
				t.Fatalf("%s permissions %o", file.Name, file.Mode().Perm())
			}
		}
		original, err := os.ReadFile(filepath.Join(root, "build", stem+strings.TrimPrefix(binary, "dbeaver-format")))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, contents[binary]) {
			t.Fatalf("binary changed %s", archive)
		}
		if !bytes.Equal(contents["LICENSE"], license) || !bytes.Contains(contents["COPYRIGHT"], []byte("lemonadern")) {
			t.Fatal("missing project licenses")
		}
		for _, name := range []string{"go-LICENSE.txt", "go-PATENTS.txt", "go-x-crypto-LICENSE.txt", "go-x-net-LICENSE.txt", "go-x-sys-LICENSE.txt", "go-x-text-LICENSE.txt", "go-source-NOTICES.txt", "go-fiat-crypto-README.txt", "manifest.json", "README.md"} {
			if len(contents["licenses/"+name]) == 0 {
				t.Fatalf("missing %s", name)
			}
		}
		if !bytes.Contains(contents["BUILD-INFO.txt"], []byte(licensedGoVersion)) || !bytes.Contains(contents["BUILD-INFO.txt"], []byte("CGO_ENABLED=0")) {
			t.Fatal("missing build provenance")
		}
	}
	if err := packageCLI(root, "build", out); err != nil {
		t.Fatal(err)
	}
	for name, before := range first {
		after, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("non-deterministic archive %s", name)
		}
	}
}

func TestLicenseTamperingRejected(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	source := filepath.Join(root, "licenses")
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "licenses"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "licenses", entry.Name()), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := licenseEntries(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "licenses", "go-LICENSE.txt"), []byte("removed notice"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := licenseEntries(dir); err == nil {
		t.Fatal("accepted changed upstream license")
	}
}

func TestRejectInvalidBinaryAndPlatform(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-binary")
	os.WriteFile(path, []byte("not executable"), 0644)
	if err := validateBinary(path, "darwin", "arm64"); err == nil {
		t.Fatal("accepted non-Go binary")
	}
	root := repoRoot(t)
	actual := filepath.Join(root, "build", "dbeaver-format-darwin-arm64")
	if _, err := os.Stat(actual); err == nil {
		if err := validateBinary(actual, "windows", "amd64"); err == nil {
			t.Fatal("accepted wrong target")
		}
	}
}
