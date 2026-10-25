// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/instance"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func target(s *httptest.Server) instance.Instance {
	_, p, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
	n, _ := strconv.Atoi(p)
	return instance.Instance{Port: n, Password: "secret"}
}
func TestRoundTrip(t *testing.T) {
	sql := "select '日本語 🚀 C:\\temp\\new.sql', '\"';\r\n"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/handleCommandLine" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("invalid request metadata")
		}
		var body struct {
			Args []string `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Args) != 3 || body.Args[0] != "bridge-format" || body.Args[1] != "--request" {
			t.Error("invalid args")
			return
		}
		var req Request
		if err := json.Unmarshal([]byte(body.Args[2]), &req); err != nil {
			t.Error(err)
			return
		}
		if req.SQL != sql || req.Connection != "id" || req.Version != 1 {
			t.Error("SQL or metadata changed")
		}
		p, _ := json.Marshal(map[string]any{"version": 1, "sql": sql})
		json.NewEncoder(w).Encode(map[string]any{"exitCode": 0, "output": []string{string(p)}})
	}))
	defer s.Close()
	got, err := Format(context.Background(), target(s), Request{Version: 1, SQL: sql, Connection: "id"}, time.Second)
	if err != nil || got != sql {
		t.Fatalf("got %q error %v", got, err)
	}
}
func TestFailureDoesNotLeakResponse(t *testing.T) {
	for _, body := range []string{
		`{"exitCode":1,"output":["secret SQL"]}`, `{"output":[]}`, `{"exitCode":0,"output":[]}`, `{"exitCode":0,"output":["{\"version\":1}"]}`, `{"exitCode":0,"output":["{\"version\":2,\"sql\":\"secret SQL\"}"]}`, `secret SQL`,
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		got, err := Format(context.Background(), target(s), Request{Version: 1, SQL: "secret SQL"}, time.Second)
		s.Close()
		if err == nil || got != "" {
			t.Fatalf("accepted invalid response %q %v", got, err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("leaked secret")
		}
	}
}
func TestRedirectAndTimeout(t *testing.T) {
	reached := false
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer sink.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, sink.URL, 307) }))
	defer s.Close()
	if _, err := Format(context.Background(), target(s), Request{}, time.Second); err == nil {
		t.Fatal("accepted redirect")
	}
	if reached {
		t.Fatal("followed redirect")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(50 * time.Millisecond) }))
	defer slow.Close()
	if _, err := Format(context.Background(), target(slow), Request{}, 20*time.Millisecond); err == nil {
		t.Fatal("no timeout")
	}
}

func TestKnownErrorEnvelope(t *testing.T) {
	for _, code := range []string{"invalid_request", "unsupported_version", "connection_not_found", "connection_ambiguous", "connection_disconnected", "formatter_unavailable", "formatting_failed"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := json.Marshal(map[string]any{"version": 1, "error": code})
			json.NewEncoder(w).Encode(map[string]any{"exitCode": 1, "output": []string{string(p)}})
		}))
		got, err := Format(context.Background(), target(s), Request{Version: 1}, time.Second)
		s.Close()
		if got != "" || err == nil || strings.Contains(err.Error(), "compatibility") {
			t.Fatalf("code %s: %q %v", code, got, err)
		}
	}
}

func TestActiveEditorProtocolCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name      string
		version   int
		errorCode string
		wantError string
	}{
		{"current-plugin", 2, "", ""},
		{"old-plugin", 1, "unsupported_version", "update the Formatter Bridge plugin"},
		{"wrong-success-version", 1, "", "unsupported protocol version"},
		{"no-editor", 2, "active_editor_unavailable", "select a SQL editor tab"},
		{"unassigned", 2, "active_editor_no_connection", "no assigned connection"},
		{"ambiguous-window", 2, "active_window_ambiguous", "window is ambiguous"},
		{"ui-unavailable", 2, "ui_unavailable", "workbench is unavailable"},
		{"ui-timeout", 2, "ui_timeout", "UI did not respond"},
		{"disconnected", 2, "connection_disconnected", "connect it in DBeaver first"},
		{"wrong-error-version", 1, "active_editor_unavailable", "check bridge installation and compatibility"},
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
				var req map[string]any
				if err := json.Unmarshal([]byte(body.Args[2]), &req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if req["version"] != float64(2) || req["activeEditor"] != true || req["connection"] != nil {
					t.Errorf("wrong active-editor request: %v", req)
				}
				envelope := map[string]any{"version": tt.version}
				exitCode := 0
				if tt.errorCode != "" {
					envelope["error"] = tt.errorCode
					exitCode = 1
				} else {
					envelope["sql"] = "SELECT 1"
				}
				p, _ := json.Marshal(envelope)
				json.NewEncoder(w).Encode(map[string]any{"exitCode": exitCode, "output": []string{string(p)}})
			}))
			defer s.Close()
			got, err := Format(context.Background(), target(s), Request{Version: 2, ActiveEditor: true, SQL: "select 1"}, time.Second)
			if tt.wantError == "" {
				if err != nil || got != "SELECT 1" {
					t.Fatalf("%q %v", got, err)
				}
			} else if err == nil || got != "" || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("%q %v; want %q", got, err, tt.wantError)
			}
		})
	}
}
