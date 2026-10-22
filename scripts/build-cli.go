//go:build ignore

// Build standalone CLI binaries for the supported desktop platforms.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	out := flag.String("out", "build", "output directory")
	flag.Parse()
	if err := build(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(out string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "cli", "go.mod")); err != nil {
		return fmt.Errorf("run this tool from the repository root")
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	for _, target := range []struct{ os, arch string }{{"windows", "amd64"}, {"windows", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		name := "dbeaver-format-" + target.os + "-" + target.arch
		if target.os == "windows" {
			name += ".exe"
		}
		cmd := exec.Command("go", "build", "-trimpath", "-o", filepath.Join(out, name), "./cmd/dbeaver-format")
		cmd.Dir = filepath.Join(root, "cli")
		cmd.Env = append(os.Environ(), "GOOS="+target.os, "GOARCH="+target.arch, "CGO_ENABLED=0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Println("Building", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", name, err)
		}
	}
	return nil
}
