// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"fmt"
	"go/ast"
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

// IsFieldType reports whether expr is a type and nothing else, as the parser reads it after the
// name of a struct field. Whether the names in it are types is left to the compiler.
func IsFieldType(expr string) bool {
	const before = clause + "type _ struct {\n_ "
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", before+expr+"\n}", parser.SkipObjectResolution)
	if err != nil {
		return false
	}

	isWhole := false
	ast.Inspect(file, func(n ast.Node) bool {
		if field, isField := n.(*ast.Field); isField && field.Type != nil {
			start, end := fset.Position(field.Type.Pos()).Offset, fset.Position(field.Type.End()).Offset
			isWhole = isWhole || start == len(before) && end == len(before)+len(expr)
		}
		return true
	})
	return isWhole
}
