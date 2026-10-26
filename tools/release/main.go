// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/releasing"
	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/versioning"
	"os"
)

func main() {
	root := os.Getenv("BRIDGE_REPOSITORY_ROOT")
	args := os.Args[1:]
	var err error
	if root == "" || len(args) == 0 {
		err = fmt.Errorf("usage: go run ./scripts/release.go prepare VERSION | check | tag VERSION | preflight ... | collect ... | publish ...")
	} else if err = os.Chdir(root); err == nil {
		// The wrapper compiles from this Go module, but command paths and gh
		// repository discovery belong to the user's repository root.
		switch args[0] {
		case "prepare", "tag":
			if len(args) != 2 {
				err = fmt.Errorf("%s requires one X.Y.Z version", args[0])
				break
			}
			if args[0] == "prepare" {
				err = versioning.Prepare(root, args[1])
			} else {
				err = versioning.Tag(root, args[1])
			}
		case "check":
			if len(args) != 1 {
				err = fmt.Errorf("check takes no arguments")
				break
			}
			var value string
			value, err = versioning.Check(root)
			if err == nil {
				fmt.Println(value)
			}
		default:
			err = releasing.Run(root, args)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}
