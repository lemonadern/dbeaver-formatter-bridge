//go:build ignore

// Build the plugin with an escaped, platform-correct local target URI.
package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := buildPlugin(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildPlugin() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "plugin", "pom.xml")); err != nil {
		return fmt.Errorf("run this tool from the repository root")
	}
	target := filepath.Join(root, "plugin", "target-platform", "target", "dbeaver-target")
	if _, err := os.Stat(filepath.Join(target, "content.jar")); err != nil {
		return fmt.Errorf("prepare the installed DBeaver target first with scripts/prepare-target.go: %w", err)
	}
	uri := fileURI(target)
	qualifier := time.Now().UTC().Format("20060102150405")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Maven is a .cmd file on Windows. Expand the encoded URI exactly once,
		// inside quotes, so cmd.exe never parses its percent escapes as variables.
		cmd = exec.Command("cmd.exe", "/d", "/s", "/c", `mvn.cmd -B -f plugin/pom.xml "-Ddbeaver.target.uri=%DBEAVER_BRIDGE_TARGET_URI%" "-DforceContextQualifier=%DBEAVER_BRIDGE_QUALIFIER%" clean verify`)
		cmd.Env = append(os.Environ(), "DBEAVER_BRIDGE_TARGET_URI="+uri, "DBEAVER_BRIDGE_QUALIFIER="+qualifier)
	} else {
		cmd = exec.Command("mvn", "-B", "-f", "plugin/pom.xml", "-Ddbeaver.target.uri="+uri, "-DforceContextQualifier="+qualifier, "clean", "verify")
	}
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Maven build: %w", err)
	}
	return nil
}

// Accept both Windows and Unix absolute paths so the URI rules can be tested
// on every build host. No platform path is used as a URI authority by accident.
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
