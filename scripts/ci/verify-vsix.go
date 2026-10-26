// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
//go:build ignore

// Verify the exact VSIX distribution, excluding development tools and binaries.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	version := strings.TrimSpace(string(data))
	file := flag.String("vsix", "dist/dbeaver-formatter-bridge-"+version+".vsix", "VSIX archive")
	writeChecksum := flag.Bool("write-checksum", false, "write checksum after verification instead of comparing it")
	flag.Parse()
	if err := verify(*file, *writeChecksum); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: VSIX contents, metadata and licenses")
}

func verify(name string, writeChecksum bool) error {
	archive, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer archive.Close()
	expected := map[string]bool{
		"[Content_Types].xml": false, "extension.vsixmanifest": false,
		"extension/package.json": false, "extension/readme.md": false,
		"extension/LICENSE.txt": false, "extension/COPYRIGHT": false,
		"extension/out/src/extension.js": false, "extension/out/src/configuration.js": false,
		"extension/out/src/formatter.js": false,
	}
	for _, entry := range archive.File {
		seen, allowed := expected[entry.Name]
		if !allowed || seen || entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > 1<<20 {
			return fmt.Errorf("unexpected, duplicate or oversized VSIX entry: %s", entry.Name)
		}
		expected[entry.Name] = true
		r, err := entry.Open()
		if err != nil {
			return err
		}
		contents, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
		closeErr := r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		switch entry.Name {
		case "extension.vsixmanifest":
			var metadata struct {
				XMLName  xml.Name `xml:"PackageManifest"`
				Identity struct {
					ID        string `xml:"Id,attr"`
					Version   string `xml:"Version,attr"`
					Publisher string `xml:"Publisher,attr"`
				} `xml:"Metadata>Identity"`
			}
			if err := xml.Unmarshal(contents, &metadata); err != nil {
				return err
			}
			if metadata.Identity.ID != "dbeaver-formatter-bridge" || metadata.Identity.Publisher != "lemonadern" || metadata.Identity.Version != expectedVersion() {
				return fmt.Errorf("VSIX identity/version mismatch")
			}
		case "extension/LICENSE.txt", "extension/COPYRIGHT":
			source, err := os.ReadFile(map[string]string{"extension/LICENSE.txt": "LICENSE", "extension/COPYRIGHT": "COPYRIGHT"}[entry.Name])
			if err != nil {
				return err
			}
			// Git may check text out as CRLF on Windows; preserve byte-exact
			// license content except for that platform checkout conversion.
			if !bytes.Equal(bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))) {
				return fmt.Errorf("VSIX license mismatch: %s", entry.Name)
			}
		case "extension/package.json":
			var manifest struct {
				Name, Publisher, Version, Main, License string
				Dependencies                            map[string]any
				ExtensionKind                           []string
				Capabilities                            struct{ UntrustedWorkspaces struct{ Supported bool } }
			}
			if err := json.Unmarshal(contents, &manifest); err != nil {
				return err
			}
			if manifest.Name != "dbeaver-formatter-bridge" || manifest.Publisher != "lemonadern" || manifest.Version != expectedVersion() || manifest.Main != "./out/src/extension.js" || manifest.License != "Apache-2.0" || len(manifest.Dependencies) != 0 || len(manifest.ExtensionKind) != 1 || manifest.ExtensionKind[0] != "ui" || manifest.Capabilities.UntrustedWorkspaces.Supported {
				return fmt.Errorf("invalid VSIX extension metadata")
			}
		}
	}
	for name, seen := range expected {
		if !seen {
			return fmt.Errorf("missing VSIX entry: %s", name)
		}
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), filepath.Base(name))
	checksumPath := filepath.Join(filepath.Dir(name), "VSIX-SHA256SUMS")
	if writeChecksum {
		return os.WriteFile(checksumPath, []byte(sums), 0644)
	}
	expectedSum, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	if string(expectedSum) != sums {
		return fmt.Errorf("VSIX checksum mismatch")
	}
	return nil
}

func expectedVersion() string {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
