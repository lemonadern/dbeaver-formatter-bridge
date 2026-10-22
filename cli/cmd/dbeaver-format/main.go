package main

import (
	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/command"
	"os"
)

func main() { os.Exit(command.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
