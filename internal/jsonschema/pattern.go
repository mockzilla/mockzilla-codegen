// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The pattern keyword, written so that Go's regexp, which the MCP SDK checks it with, and an
// ECMA-262 engine read it the same.

package jsonschema

import (
	"regexp"

	"github.com/mockzilla/mockzilla-codegen/internal/ecma"
)

// compilePattern compiles an ECMA-262 pattern in the form ecma.Portable writes.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(ecma.Portable(pattern))
}
