// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The check that text a template wrote parses as Go declarations.

package gocode

import (
	"fmt"
	"go/parser"
	"go/token"
)

// clause is the package clause that makes a file of declarations, for the parser to take.
const clause = "package p\n"

// CheckDecls returns an error when src is no list of Go declarations, as a file holds them below
// its package clause. name stands for src in the error, which shows the lines around the first
// problem.
func CheckDecls(name string, src []byte) error {
	_, err := parser.ParseFile(token.NewFileSet(), "", clause+string(src), parser.SkipObjectResolution)
	if err == nil {
		return nil
	}

	line := locate(err, name, src, len(clause))
	return fmt.Errorf("%w: %w\n%s", ErrParse, err, excerpt(string(src), line))
}
