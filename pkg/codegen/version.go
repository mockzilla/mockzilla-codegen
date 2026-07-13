// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import "runtime/debug"

const (
	modulePath = "github.com/mockzilla/codegen"
	devVersion = "dev"
)

// Version returns the version of this module in the running binary, or "dev" for local builds.
func Version() string {
	return versionFrom(debug.ReadBuildInfo())
}

func versionFrom(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return devVersion
	}

	if info.Main.Path == modulePath {
		return released(info.Main.Version)
	}

	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Replace != nil {
			return released(dep.Replace.Version)
		}
		return released(dep.Version)
	}

	return devVersion
}

func released(v string) string {
	if v == "" || v == "(devel)" {
		return devVersion
	}
	return v
}
