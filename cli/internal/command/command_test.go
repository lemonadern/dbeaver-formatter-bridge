// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputAndFailurePreservation(t *testing.T) {
	for _, success := range []bool{true, false} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !success {
				w.WriteHeader(500)
				return
			}
			p, _ := json.Marshal(map[string]any{"version": 1, "sql": "SELECT 日本語;\r\n"})
			json.NewEncoder(w).Encode(map[string]any{"exitCode": 0, "output": []string{string(p)}})
		}))
		_, port, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, ".metadata"), 0700)
		os.WriteFile(filepath.Join(dir, ".metadata", "dbeaver-instance.properties"), []byte("port="+port+"\npassword=secret"), 0600)
		var out, diag bytes.Buffer
		code := Run([]string{"--workspace", dir}, strings.NewReader("select 日本語;\r\n"), &out, &diag)
		s.Close()
		if success {
			if code != 0 || out.String() != "SELECT 日本語;\r\n" || diag.Len() != 0 {
				t.Fatalf("%d %q %q", code, out.String(), diag.String())
			}
		} else {
			if code == 0 || out.Len() != 0 || diag.Len() == 0 {
				t.Fatalf("%d %q %q", code, out.String(), diag.String())
			}
		}
	}
}
func TestUsage(t *testing.T) {
	for _, a := range [][]string{{}, {"--workspace", "x", "extra"}, {"--workspace", "x", "--timeout", "0s"}, {"--workspace", "x", "--indent", ""}, {"--workspace", "x", "--indent", "x"}, {"--workspace", "x", "--indent", strings.Repeat(" ", 33)}} {
		var out, diag bytes.Buffer
		if c := Run(a, strings.NewReader("sql"), &out, &diag); c == 0 || out.Len() != 0 {
			t.Fatalf("accepted %#v", a)
		}
	}
}
