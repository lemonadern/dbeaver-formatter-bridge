//go:build ignore

// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Verify that the distributable site includes only our licensed feature/bundle.
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	site := flag.String("site", "plugin/repository/target/formatter-bridge-update-site-0.1.0-SNAPSHOT.zip", "p2 site ZIP")
	flag.Parse()
	if err := verify(*site); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: p2 site includes only bridge feature/bundle, both with LICENSE and COPYRIGHT")
}

func verify(path string) error {
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
			if !strings.HasPrefix(f.Name, expected+"_") || !strings.HasSuffix(f.Name, ".jar") {
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
			for _, entry := range jar.File {
				if _, ok := found[entry.Name]; ok {
					found[entry.Name] = entry.UncompressedSize64 > 0
				}
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
