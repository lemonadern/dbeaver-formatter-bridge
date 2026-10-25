// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

//go:build ignore

// Exercise a running DBeaver bridge without modifying its settings or databases.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func main() {
	cli := flag.String("cli", "", "absolute CLI executable path")
	workspace := flag.String("workspace", "", "running DBeaver workspace")
	flag.Parse()
	if *cli == "" || *workspace == "" {
		fmt.Fprintln(os.Stderr, "--cli and --workspace are required")
		os.Exit(2)
	}
	if err := check(*cli, *workspace); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: real bridge strings, CRLF, empty input, failures and concurrent indentation")
}

func check(cli, workspace string) error {
	run := func(sql string, args ...string) (string, string, error) {
		cmd := exec.Command(cli, append([]string{"--workspace", workspace}, args...)...)
		cmd.Stdin = strings.NewReader(sql)
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		return out.String(), stderr.String(), err
	}
	for _, newline := range []string{"\n", "\r\n"} {
		sql := "select '日本語😀' as label,'C:\\temp\\new.sql' as path,'\"quoted\"' as value" + newline + "from example where id=1;"
		out, stderr, err := run(sql)
		if err != nil || stderr != "" {
			return fmt.Errorf("string formatting failed (newline=%q, process=%v, diagnostic=%q)", newline, err, strings.TrimSpace(stderr))
		}
		for _, literal := range []string{"'日本語😀'", "'C:\\temp\\new.sql'", "'\"quoted\"'"} {
			if !strings.Contains(out, literal) {
				return fmt.Errorf("literal was not preserved: %q", literal)
			}
		}
	}
	if out, stderr, err := run(""); err != nil || out != "" || stderr != "" {
		return fmt.Errorf("empty input failed")
	}
	for _, tt := range []struct {
		args    []string
		message string
	}{
		{[]string{"--connection", "bridge-test-nonexistent-connection"}, "connection ID was not found"},
		{[]string{"--indent", ""}, "--indent"},
		{[]string{"--timeout", "1ns"}, "timed out"},
	} {
		out, stderr, err := run("select 1", tt.args...)
		if err == nil || out != "" || !strings.Contains(stderr, tt.message) {
			return fmt.Errorf("expected safe failure for %v", tt.args)
		}
	}
	// Build baselines for explicit indent values, then interleave requests.
	// This detects request state leaking without changing the user's formatter preferences.
	inputs := []string{" ", "  ", "\t"}
	expected := make([]string, len(inputs))
	for i, indent := range inputs {
		out, stderr, err := run("select a,b from example where id=1", "--indent", indent)
		if err != nil || stderr != "" {
			return fmt.Errorf("indent baseline failed")
		}
		expected[i] = out
	}
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for j := 0; j < 12; j++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, stderr, err := run("select a,b from example where id=1", "--indent", inputs[i])
			if err != nil || stderr != "" || out != expected[i] {
				failures <- fmt.Errorf("concurrent formatting differed")
			}
		}(j % len(inputs))
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		return err
	}
	return nil
}
