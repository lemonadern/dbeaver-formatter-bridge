// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Package client sends SQL to the bridge command in a local DBeaver instance.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/instance"
)

const MaxSQLBytes = 1 << 20
const maxResponseBytes = 64 << 20

type Request struct {
	ActiveEditor bool    `json:"activeEditor,omitempty"`
	Version      int     `json:"version"`
	SQL          string  `json:"sql"`
	Connection   string  `json:"connection,omitempty"`
	Indent       *string `json:"indent,omitempty"`
}

func Format(ctx context.Context, target instance.Instance, input Request, timeout time.Duration) (string, error) {
	if !utf8.ValidString(input.SQL) || len(input.SQL) > MaxSQLBytes {
		return "", fmt.Errorf("SQL must be UTF-8 and at most 1 MiB")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("cannot encode formatting request")
	}
	body, _ := json.Marshal(struct {
		Args []string `json:"args"`
	}{[]string{"bridge-format", "--request", string(payload)}})
	// Numeric loopback address and no proxy prevent credentials leaving this machine.
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	hc := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(target.Port)+"/handleCommandLine", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("cannot construct bridge request")
	}
	req.Header.Set("Authorization", "Bearer "+target.Password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return "", fmt.Errorf("DBeaver bridge request timed out")
		}
		return "", fmt.Errorf("cannot connect to DBeaver; start DBeaver with this workspace")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DBeaver bridge returned HTTP %d; check the running instance and bridge installation", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("cannot read DBeaver response")
	}
	if len(data) > maxResponseBytes {
		return "", fmt.Errorf("DBeaver response exceeds 64 MiB")
	}
	var result struct {
		ExitCode *int     `json:"exitCode"`
		Output   []string `json:"output"`
	}
	if json.Unmarshal(data, &result) != nil || result.ExitCode == nil {
		return "", fmt.Errorf("invalid DBeaver command response")
	}
	if *result.ExitCode != 0 {
		if len(result.Output) == 1 {
			var failure struct {
				Version int    `json:"version"`
				Error   string `json:"error"`
			}
			if json.Unmarshal([]byte(result.Output[0]), &failure) == nil && (failure.Version == input.Version || failure.Version == 1 && failure.Error == "unsupported_version") {
				if input.ActiveEditor && failure.Error == "unsupported_version" {
					return "", fmt.Errorf("DBeaver bridge does not support --active-editor; update the Formatter Bridge plugin and restart DBeaver")
				}
				messages := map[string]string{
					"active_editor_unavailable":   "DBeaver has no selected SQL editor; select a SQL editor tab in DBeaver",
					"active_editor_no_connection": "DBeaver selected SQL editor has no assigned connection",
					"active_window_ambiguous":     "DBeaver window is ambiguous; activate the intended DBeaver window and retry",
					"ui_unavailable":              "DBeaver workbench is unavailable",
					"ui_timeout":                  "DBeaver UI did not respond in time; close any modal dialog and retry",
					"invalid_request":             "DBeaver bridge rejected the formatting request",
					"unsupported_version":         "DBeaver bridge protocol version is unsupported",
					"connection_not_found":        "DBeaver connection ID was not found",
					"connection_ambiguous":        "DBeaver connection ID is ambiguous across projects",
					"connection_disconnected":     "DBeaver connection is disconnected; connect it in DBeaver first",
					"formatter_unavailable":       "DBeaver configured formatter is unavailable",
					"formatting_failed":           "DBeaver formatter failed",
				}
				if message, ok := messages[failure.Error]; ok {
					return "", fmt.Errorf("%s", message)
				}
			}
		}
		return "", fmt.Errorf("DBeaver bridge command failed (exit code %d); check bridge installation and compatibility", *result.ExitCode)
	}
	if len(result.Output) != 1 {
		return "", fmt.Errorf("unexpected bridge output; check bridge installation and compatibility")
	}
	var formatted struct {
		Version int     `json:"version"`
		SQL     *string `json:"sql"`
	}
	if json.Unmarshal([]byte(result.Output[0]), &formatted) != nil || formatted.Version != input.Version || formatted.SQL == nil {
		return "", fmt.Errorf("invalid bridge response or unsupported protocol version")
	}
	return *formatted.SQL, nil
}
