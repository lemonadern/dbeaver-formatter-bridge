//go:build ignore

// Run with go run scripts/test-efm.go --efm PATH --cli PATH --workspace PATH.
// This exercises the installed efm over stdio LSP using an unsaved SQL buffer.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	efm := flag.String("efm", "efm-langserver", "efm executable")
	cli := flag.String("cli", "", "absolute CLI executable path")
	workspace := flag.String("workspace", "", "running DBeaver workspace")
	flag.Parse()
	if *cli == "" || *workspace == "" {
		return fmt.Errorf("--cli and --workspace required")
	}
	sql := "select '日本語 🚀 C:\\temp\\new.sql' as greeting,a,b from example where a=1;\n"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	direct := exec.CommandContext(ctx, *cli, "--workspace", *workspace)
	direct.Stdin = strings.NewReader(sql)
	expected, err := direct.Output()
	if err != nil {
		return fmt.Errorf("direct CLI failed: %w", err)
	}
	if string(expected) == sql {
		return fmt.Errorf("fixture was not formatted")
	}
	if err := exercise(ctx, *efm, *cli, *workspace, sql, string(expected), false); err != nil {
		return err
	}
	if err := exercise(ctx, *efm, *cli, filepath.Join(os.TempDir(), "dbeaver-bridge-missing-workspace"), sql, "", true); err != nil {
		return err
	}
	fmt.Println("PASS: efm formatted unsaved SQL buffer to CLI-equivalent output; CLI failure returned no edits")
	return nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func exercise(ctx context.Context, efm, cli, workspace, sql, expected string, failure bool) error {
	dir, err := os.MkdirTemp("", "dbeaver-efm-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	config := map[string]any{"version": 2, "languages": map[string]any{"sql": []any{map[string]any{"format-command": shellQuote(cli) + " --workspace " + shellQuote(workspace), "format-stdin": true}}}}
	data, _ := json.Marshal(config)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, efm, "-c", path)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { in.Close(); cmd.Process.Kill(); cmd.Wait() }()
	reader := bufio.NewReader(out)
	send := func(v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		_, e = fmt.Fprintf(in, "Content-Length: %d\r\n\r\n%s", len(b), b)
		return e
	}
	receive := func(id int) (json.RawMessage, json.RawMessage, error) {
		for {
			length := 0
			for {
				line, e := reader.ReadString('\n')
				if e != nil {
					return nil, nil, e
				}
				line = strings.TrimSpace(line)
				if line == "" {
					break
				}
				if strings.HasPrefix(line, "Content-Length:") {
					length, e = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")))
					if e != nil {
						return nil, nil, e
					}
				}
			}
			if length < 1 || length > 4<<20 {
				return nil, nil, fmt.Errorf("invalid LSP frame length")
			}
			b := make([]byte, length)
			if _, e := io.ReadFull(reader, b); e != nil {
				return nil, nil, e
			}
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if e := json.Unmarshal(b, &msg); e != nil {
				return nil, nil, e
			}
			if msg.ID == id {
				return msg.Result, msg.Error, nil
			}
		}
	}
	if err := send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"processId": os.Getpid(), "rootUri": nil, "capabilities": map[string]any{}, "initializationOptions": map[string]any{"documentFormatting": true}}}); err != nil {
		return err
	}
	if _, rpcErr, err := receive(1); err != nil || rpcErr != nil {
		return fmt.Errorf("efm initialization failed: %v %s", err, rpcErr)
	}
	if err := send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}}); err != nil {
		return err
	}
	uri := "file:///unsaved-bridge-test.sql"
	if err := send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "sql", "version": 1, "text": sql}}}); err != nil {
		return err
	}
	if err := send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "textDocument/formatting", "params": map[string]any{"textDocument": map[string]any{"uri": uri}, "options": map[string]any{"tabSize": 4, "insertSpaces": true}}}); err != nil {
		return err
	}
	result, rpcErr, err := receive(2)
	if err != nil {
		return err
	}
	var edits []struct {
		NewText string `json:"newText"`
		Range   struct {
			Start struct{ Line, Character int }
			End   struct{ Line, Character int }
		} `json:"range"`
	}
	if rpcErr != nil && string(rpcErr) != "null" {
		if failure {
			return nil
		}
		return fmt.Errorf("efm formatting RPC error: %s", rpcErr)
	}
	if err := json.Unmarshal(result, &edits); err != nil {
		return err
	}
	if failure {
		if len(edits) > 0 {
			return fmt.Errorf("CLI failure unexpectedly produced edits")
		}
		return nil
	}
	// LSP edits refer to the original buffer. Apply in reverse position order;
	// efm may express a replacement as separate deletion and insertion edits.
	type change struct {
		start, end, index int
		text              string
	}
	var changes []change
	for i, edit := range edits {
		start, e := offset(sql, edit.Range.Start.Line, edit.Range.Start.Character)
		if e != nil {
			return e
		}
		end, e := offset(sql, edit.Range.End.Line, edit.Range.End.Character)
		if e != nil {
			return e
		}
		if end < start {
			return fmt.Errorf("reversed edit range")
		}
		changes = append(changes, change{start, end, i, edit.NewText})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].start == changes[j].start {
			return changes[i].index > changes[j].index
		}
		return changes[i].start > changes[j].start
	})
	actual := sql
	for _, edit := range changes {
		actual = actual[:edit.start] + edit.text + actual[edit.end:]
	}
	if len(edits) == 0 || actual != expected {
		return fmt.Errorf("efm edits did not match CLI output: %s", result)
	}
	return nil
}

// LSP character offsets are UTF-16 code units, not UTF-8 bytes.
func offset(text string, line, character int) (int, error) {
	if line < 0 || character < 0 {
		return 0, fmt.Errorf("invalid edit range")
	}
	start := 0
	for i := 0; i < line; i++ {
		n := strings.IndexByte(text[start:], '\n')
		if n < 0 {
			return 0, fmt.Errorf("edit line outside buffer")
		}
		start += n + 1
	}
	units := 0
	for index, r := range text[start:] {
		if units == character {
			return start + index, nil
		}
		if r == '\n' {
			break
		}
		units++
		if r > 0xffff {
			units++
		}
	}
	if units == character {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			return len(text), nil
		}
		return start + end, nil
	}
	return 0, fmt.Errorf("edit character outside buffer")
}
