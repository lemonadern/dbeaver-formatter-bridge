// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/lemonadern/dbeaver-formatter-bridge/cli/internal/command"
	"os"
)

func main() { os.Exit(command.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
