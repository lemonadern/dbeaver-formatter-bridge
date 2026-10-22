package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/client"
	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/instance"
)

func Run(args []string, in io.Reader, out, diagnostics io.Writer) int {
	fs := flag.NewFlagSet("dbeaver-format", flag.ContinueOnError)
	fs.SetOutput(diagnostics)
	workspace := fs.String("workspace", "", "DBeaver workspace path (required)")
	connection := fs.String("connection", "", "DBeaver connection ID; default uses global SQL settings")
	timeout := fs.Duration("timeout", 10*time.Second, "request timeout, e.g. 10s")
	indent := fs.String("indent", "", "indent string (spaces or tabs); default uses DBeaver settings")
	fs.Usage = func() {
		fmt.Fprintln(diagnostics, "Usage: dbeaver-format --workspace PATH [--connection ID] [--timeout 10s] [--indent STRING] < input.sql")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(diagnostics, "dbeaver-format:", err); return 1 }
	if *workspace == "" || fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		return 2
	}
	var indentValue *string
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "indent" {
			indentValue = indent
		}
	})
	if indentValue != nil && (len(*indentValue) == 0 || len(*indentValue) > 32 || strings.Trim(*indentValue, " \t") != "") {
		return fail(fmt.Errorf("--indent accepts at most 32 spaces or tabs"))
	}
	target, err := instance.Load(*workspace)
	if err != nil {
		return fail(err)
	}
	sql, err := io.ReadAll(io.LimitReader(in, client.MaxSQLBytes+1))
	if err != nil {
		return fail(fmt.Errorf("cannot read SQL from stdin"))
	}
	formatted, err := client.Format(context.Background(), target, client.Request{Version: 1, SQL: string(sql), Connection: *connection, Indent: indentValue}, *timeout)
	if err != nil {
		return fail(err)
	}
	if _, err = io.WriteString(out, formatted); err != nil {
		return fail(fmt.Errorf("cannot write formatted SQL"))
	}
	return 0
}
