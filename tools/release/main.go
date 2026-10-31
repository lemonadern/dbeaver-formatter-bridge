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
		err = fmt.Errorf("usage: go run ./scripts/release.go prepare COMPONENT VERSION | check [COMPONENT] | tag COMPONENT VERSION | preflight ... | collect ... | publish ...")
	} else if err = os.Chdir(root); err == nil {
		// The wrapper compiles from this Go module, but command paths and gh
		// repository discovery belong to the user's repository root.
		switch args[0] {
		case "prepare", "tag":
			if len(args) != 3 {
				err = fmt.Errorf("%s requires COMPONENT and X.Y.Z version", args[0])
				break
			}
			if args[0] == "prepare" {
				err = versioning.PrepareComponent(root, args[1], args[2])
			} else {
				err = versioning.TagComponent(root, args[1], args[2])
			}
		case "check":
			components := []string{"plugin", "cli", "vscode"}
			if len(args) == 2 {
				components = args[1:]
			} else if len(args) != 1 {
				err = fmt.Errorf("check accepts at most one COMPONENT")
				break
			}
			for _, component := range components {
				var value string
				value, err = versioning.CheckComponent(root, component)
				// The historical shared release predates authored component notes.
				// Every new component version must already have a reviewable draft
				// in ordinary PR CI, before any tag can be created.
				if err == nil && value != "0.1.0" {
					err = versioning.CheckNotes(root, component, value)
				}
				if err != nil {
					break
				}
				fmt.Printf("%s %s\n", component, value)
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
