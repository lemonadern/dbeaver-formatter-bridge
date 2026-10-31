// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/version"
)

type versionUnreadableInput struct{}

func (versionUnreadableInput) Read([]byte) (int, error) { panic("--version must not read SQL") }

func TestVersionWithoutWorkspaceOrSQL(t *testing.T) {
	var out, diagnostics bytes.Buffer
	if status := Run([]string{"--version"}, versionUnreadableInput{}, &out, &diagnostics); status != 0 {
		t.Fatalf("status=%d: %s", status, &diagnostics)
	}
	if !strings.HasPrefix(out.String(), "dbeaver-format "+version.Version) || diagnostics.Len() != 0 {
		t.Fatalf("output=%q diagnostics=%q", out.String(), diagnostics.String())
	}
}
func TestVersionDoesNotHideOtherArguments(t *testing.T) {
	for _, args := range [][]string{{"--version", "file.sql"}, {"--workspace", "/missing", "--version"}} {
		var out, diagnostics bytes.Buffer
		if status := Run(args, versionUnreadableInput{}, &out, &diagnostics); status != 2 || out.Len() != 0 {
			t.Fatalf("args=%q status=%d output=%q", args, status, out.String())
		}
	}
}
