// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

//go:build ignore

// Build a test-only OSGi fixture against existing DBeaver bundles; no downloads.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	home := flag.String("dbeaver-home", "", "DBeaver installation directory")
	javaHome := flag.String("java-home", os.Getenv("JAVA_HOME"), "JDK directory")
	output := flag.String("output", "build/active-editor-fixture.jar", "fixture JAR output")
	flag.Parse()
	if err := build(*home, *javaHome, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(home, javaHome, output string) error {
	if strings.HasSuffix(home, ".app") {
		home = filepath.Join(home, "Contents", "Eclipse")
	}
	jars, err := filepath.Glob(filepath.Join(home, "plugins", "*.jar"))
	if err != nil || len(jars) == 0 {
		return fmt.Errorf("DBeaver plugin JARs not found")
	}
	var sources []string
	err = filepath.WalkDir("tests/active-editor/src", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".java") {
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	classes, err := os.MkdirTemp("", "bridge-fixture-classes-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(classes)
	tool := func(name string, args ...string) error {
		if javaHome != "" {
			name = filepath.Join(javaHome, "bin", name)
		}
		cmd := exec.Command(name, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	// javac argument files avoid Windows' native command line length limit.
	quote := func(value string) string { return `"` + strings.ReplaceAll(filepath.ToSlash(value), `"`, `\"`) + `"` }
	arguments := []string{"--release", "21", "-encoding", "UTF-8", "-cp", quote(strings.Join(jars, string(os.PathListSeparator))), "-d", quote(classes)}
	for _, source := range sources {
		arguments = append(arguments, quote(source))
	}
	argfile := filepath.Join(classes, "javac.args")
	if err := os.WriteFile(argfile, []byte(strings.Join(arguments, "\n")), 0600); err != nil {
		return err
	}
	if err := tool("javac", "@"+argfile); err != nil {
		return err
	}
	if err := os.Remove(argfile); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return tool("jar", "--create", "--file", output, "--manifest", "tests/active-editor/META-INF/MANIFEST.MF", "-C", classes, ".", "-C", "tests/active-editor", "plugin.xml")
}
