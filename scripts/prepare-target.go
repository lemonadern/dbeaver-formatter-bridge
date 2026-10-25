// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

//go:build ignore

// Prepare a local p2 target without modifying the installed DBeaver.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	home := flag.String("dbeaver-home", "", "DBeaver installation directory (or macOS .app)")
	java := flag.String("java", "java", "Java executable able to run installed DBeaver bundles")
	flag.Parse()
	if err := prepare(*home, *java); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare(home, java string) error {
	if home == "" {
		return fmt.Errorf("-dbeaver-home is required")
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(home), ".app") {
		home = filepath.Join(home, "Contents", "Eclipse")
	}
	infoPath := filepath.Join(home, "configuration", "org.eclipse.equinox.simpleconfigurator", "bundles.info")
	input, err := os.Open(infoPath)
	if err != nil {
		return fmt.Errorf("read installed bundles: %w", err)
	}
	defer input.Close()
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "plugin", "pom.xml")); err != nil {
		return fmt.Errorf("run this tool from the repository root")
	}
	output := filepath.Join(root, "plugin", "target-platform", "target", "dbeaver-target")
	// Refuse stale metadata and leave a previously prepared target intact on failure.
	staging := output + ".preparing"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	config := filepath.Join(root, "plugin", "target-platform", "target", "publisher-configuration")
	if err := os.RemoveAll(config); err != nil {
		return err
	}
	if err := os.MkdirAll(config, 0755); err != nil {
		return err
	}
	var lines []string
	var framework, configurator string
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			return fmt.Errorf("unsupported bundles.info record: %q", line)
		}
		location := parts[2]
		if !strings.HasPrefix(location, "file:") {
			location = fileURI(filepath.Join(home, filepath.FromSlash(location)))
		}
		parts[2] = location
		lines = append(lines, strings.Join(parts, ","))
		switch parts[0] {
		case "org.eclipse.osgi":
			framework = location
		case "org.eclipse.equinox.simpleconfigurator":
			configurator = location
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if framework == "" || configurator == "" {
		return fmt.Errorf("installed DBeaver has no Equinox framework/simpleconfigurator")
	}
	if err := os.WriteFile(filepath.Join(config, "bundles.info"), []byte("#encoding=UTF-8\n#version=1\n"+strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return err
	}
	props := "osgi.bundles.defaultStartLevel=4\nosgi.configuration.cascaded=false\n" +
		"org.eclipse.equinox.simpleconfigurator.exclusiveInstallation=false\n" +
		"org.eclipse.equinox.simpleconfigurator.configUrl=" + fileURI(filepath.Join(config, "bundles.info")) + "\n" +
		"osgi.bundles=reference:" + configurator + "@1:start\nosgi.framework=" + framework + "\n"
	if err := os.WriteFile(filepath.Join(config, "config.ini"), []byte(props), 0644); err != nil {
		return err
	}
	launchers, err := filepath.Glob(filepath.Join(home, "plugins", "org.eclipse.equinox.launcher_*.jar"))
	if err != nil {
		return err
	}
	if len(launchers) != 1 {
		return fmt.Errorf("expected one Equinox launcher, found %d", len(launchers))
	}
	args := []string{"-jar", launchers[0], "-configuration", config, "-data", filepath.Join(root, "plugin", "target-platform", "target", "publisher-workspace"), "-nosplash", "-consoleLog", "-application", "org.eclipse.equinox.p2.publisher.FeaturesAndBundlesPublisher", "-source", home, "-metadataRepository", fileURI(staging), "-artifactRepository", fileURI(staging), "-publishArtifacts", "-compress"}
	cmd := exec.Command(java, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Equinox publisher: %w", err)
	}
	if _, err := os.Stat(filepath.Join(staging, "content.jar")); err != nil {
		return fmt.Errorf("publisher did not create content.jar: %w", err)
	}
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	if err := os.Rename(staging, output); err != nil {
		return err
	}
	fmt.Println("Prepared", output)
	return nil
}

func fileURI(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	var host string
	if strings.HasPrefix(path, "//") {
		host, path, _ = strings.Cut(strings.TrimPrefix(path, "//"), "/")
		path = "/" + path
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Host: host, Path: path}).String()
}
