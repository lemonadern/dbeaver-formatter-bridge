//go:build ignore

// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Run release tooling from the repository root without installing another CLI.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root, err := os.Getwd()
	if err == nil {
		_, err = os.Stat(filepath.Join(root, "VERSION"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "run this tool from the repository root")
		os.Exit(1)
	}
	cmd := exec.Command("go", append([]string{"run", "."}, os.Args[1:]...)...)
	cmd.Dir = filepath.Join(root, "tools", "release")
	cmd.Env = append(os.Environ(), "BRIDGE_REPOSITORY_ROOT="+root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
