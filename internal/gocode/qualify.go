// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// A type expression of one package as another package writes it.

package gocode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"unicode"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// Text returns t as the package that declares its named type writes it: never qualified. A nil
// type is any.
func Text(t gomodel.Type) string {
	switch t := t.(type) {
	case gomodel.Builtin:
		return t.Name
	case gomodel.DeclRef:
		return t.Decl.Name
	case gomodel.Qualified:
		return t.Name
	case gomodel.Pointer:
		return "*" + Text(t.Elem)
	case gomodel.Slice:
		return "[]" + Text(t.Elem)
	case gomodel.Map:
		return "map[" + Text(t.Key) + "]" + Text(t.Elem)
	}
	return "any"
}

// Leaf is the type inside the pointers, slices and maps of t: t itself when it holds none.
func Leaf(t gomodel.Type) gomodel.Type {
	switch t := t.(type) {
	case gomodel.Pointer:
		return Leaf(t.Elem)
	case gomodel.Slice:
		return Leaf(t.Elem)
	case gomodel.Map:
		return Leaf(t.Elem)
	}
	return t
}

// Qualify writes expr, a type expression whose trailing identifier is declared in package pkg, as
// another package writes it: []Pet with models gives []models.Pet. An empty pkg leaves expr as it
// is.
func Qualify(expr, pkg string) string {
	if pkg == "" {
		return expr
	}
	start := len(strings.TrimRightFunc(expr, func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }))
	return expr[:start] + pkg + "." + expr[start:]
}

// CanQualify reports whether Qualify writes expr right: expr is an identifier, or pointers,
// slices, arrays, maps and channels around one, and has nothing before the type or after the
// identifier.
func CanQualify(expr string) bool {
	fset := token.NewFileSet()
	node, err := parser.ParseExprFrom(fset, "", expr, 0)
	if err != nil || fset.Position(node.Pos()).Offset != 0 {
		return false
	}

	for {
		switch n := node.(type) {
		case *ast.StarExpr:
			node = n.X
		case *ast.ArrayType:
			// The parser takes [...]T, which is no type outside a composite literal.
			if _, isOpen := n.Len.(*ast.Ellipsis); isOpen {
				return false
			}
			node = n.Elt
		case *ast.MapType:
			node = n.Value
		case *ast.ChanType:
			node = n.Value
		case *ast.Ident:
			return fset.Position(n.End()).Offset == len(expr)
		default:
			return false
		}
	}
}
