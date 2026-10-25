//go:build ignore

// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Verify that the distributable site includes only our licensed feature/bundle.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	site := flag.String("site", "plugin/repository/target/formatter-bridge-update-site-0.1.0.zip", "p2 site ZIP")
	version := flag.String("version", "0.1.0", "expected release bundle and feature version")
	flag.Parse()
	if err := verify(*site, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: p2 site includes only bridge feature/bundle, both with LICENSE and COPYRIGHT")
}

func verify(path, version string) error {
	outer, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer outer.Close()
	counts := map[string]int{"features/": 0, "plugins/": 0}
	for _, f := range outer.File {
		for dir := range counts {
			if !strings.HasPrefix(f.Name, dir) || f.FileInfo().IsDir() {
				continue
			}
			expected := dir + "com.github.lemonadern.dbeaver.formatter.bridge"
			if dir == "features/" {
				expected += ".feature"
			}
			if f.Name != expected+"_"+version+".jar" {
				return fmt.Errorf("unexpected site payload: %s", f.Name)
			}
			counts[dir]++
			reader, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				return err
			}
			jar, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				return err
			}
			found := map[string]bool{"LICENSE": false, "COPYRIGHT": false}
			versionFound := false
			for _, entry := range jar.File {
				if (dir == "plugins/" && entry.Name == "META-INF/MANIFEST.MF") || (dir == "features/" && entry.Name == "feature.xml") {
					r, err := entry.Open()
					if err != nil {
						return err
					}
					content, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						return err
					}
					if dir == "plugins/" {
						text := strings.ReplaceAll(string(content), "\r\n", "\n")
						versionFound = strings.Contains("\n"+text, "\nBundle-Version: "+version+"\n")
					} else {
						var feature struct {
							Version string `xml:"version,attr"`
						}
						if err := xml.Unmarshal(content, &feature); err != nil {
							return err
						}
						versionFound = feature.Version == version
					}
				}
				if _, ok := found[entry.Name]; ok {
					found[entry.Name] = entry.UncompressedSize64 > 0
				}
			}
			if !versionFound {
				return fmt.Errorf("%s does not contain release version %s", f.Name, version)
			}
			for name, ok := range found {
				if !ok {
					return fmt.Errorf("%s missing nonempty %s", f.Name, name)
				}
			}
		}
	}
	for dir, n := range counts {
		if n != 1 {
			return fmt.Errorf("expected exactly one %s artifact, got %d", dir, n)
		}
	}
	return nil
}
