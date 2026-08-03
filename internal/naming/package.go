// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"go/token"
	"path/filepath"
	"strings"
)

const fallbackPackage = "api"

// Package returns the Go package name for a folder: its name in lower case, letters and digits
// only, no leading digits. It is "api" when nothing is left or the name is a keyword.
func Package(dir string) string {
	name := strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(filepath.Base(dir)))
	name = strings.TrimLeft(name, "0123456789")

	if name == "" || token.IsKeyword(name) {
		return fallbackPackage
	}
	return name
}
