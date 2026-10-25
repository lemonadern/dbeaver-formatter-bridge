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

func TestConnectionSelection(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		version    int
		connection string
		active     bool
	}{
		{"global", nil, 1, "", false},
		{"explicit", []string{"--connection", "database-A"}, 1, "database-A", false},
		{"active", []string{"--active-editor"}, 2, "", true},
		{"active-disabled", []string{"--active-editor=false"}, 1, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Args []string `json:"args"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Args) != 3 {
					t.Error("invalid command envelope")
					w.WriteHeader(400)
					return
				}
				var input struct {
					Version      int    `json:"version"`
					Connection   string `json:"connection"`
					ActiveEditor bool   `json:"activeEditor"`
				}
				if err := json.Unmarshal([]byte(body.Args[2]), &input); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if input.Version != tt.version || input.Connection != tt.connection || input.ActiveEditor != tt.active {
					t.Errorf("wrong selection: %+v", input)
				}
				p, _ := json.Marshal(map[string]any{"version": tt.version, "sql": "SELECT 1"})
				json.NewEncoder(w).Encode(map[string]any{"exitCode": 0, "output": []string{string(p)}})
			}))
			defer s.Close()
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".metadata"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".metadata", "dbeaver-instance.properties"), []byte("port="+port+"\npassword=secret"), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			args := append([]string{"--workspace", dir}, tt.args...)
			if code := Run(args, strings.NewReader("select 1"), &out, &diag); code != 0 || out.String() != "SELECT 1" || diag.Len() != 0 {
				t.Fatalf("%d %q %q", code, &out, &diag)
			}
		})
	}
}

func TestActiveEditorRejectsExplicitConnectionBeforeReadingInput(t *testing.T) {
	for _, connection := range []string{"database-A", ""} {
		var out, diag bytes.Buffer
		code := Run([]string{"--workspace", "nonexistent", "--active-editor", "--connection", connection}, unreadableInput{}, &out, &diag)
		if code != 2 || out.Len() != 0 || !strings.Contains(diag.String(), "cannot be used together") {
			t.Fatalf("%d %q %q", code, &out, &diag)
		}
	}
}

type unreadableInput struct{}

func (unreadableInput) Read([]byte) (int, error) { panic("invalid options must not read stdin") }
