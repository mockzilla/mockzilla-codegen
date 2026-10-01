// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"go/token"
	"path/filepath"
	"strconv"
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

// ImportName returns the name the package at an import path has by the usual layout of module
// paths: a major version suffix (v2) and a go- prefix are not part of the name, nor is anything
// after a dot (yaml.v3). It is empty when what is left is no Go name.
func ImportName(path string) string {
	elems := strings.Split(path, "/")
	name := elems[len(elems)-1]
	if isMajorVersion(name) && len(elems) > 1 {
		name = elems[len(elems)-2]
	}
	name = strings.TrimPrefix(name, "go-")
	name, _, _ = strings.Cut(name, ".")
	name = strings.Map(func(r rune) rune {
		if r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, name)

	if !token.IsIdentifier(name) || name == "_" {
		return ""
	}
	return name
}

func isMajorVersion(elem string) bool {
	n, err := strconv.Atoi(strings.TrimPrefix(elem, "v"))
	return strings.HasPrefix(elem, "v") && err == nil && n > 1
}
