// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package version

import "runtime/debug"

// String reports the source product version. Module installs do not always
// retain a Git revision, so it is shown only when Go supplies one.
func String() string {
	value := "dbeaver-format " + Version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				value += " (" + setting.Value + ")"
			}
		}
	}
	return value
}
