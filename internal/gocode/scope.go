// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package gocode writes Go source text: type expressions as a given file spells them, imports,
// comments, tags and literals, and formats the result. No other package builds Go code as text.
package gocode

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// generatorLevel is the runtime API level generated code needs. Raise it when generated code
// starts to use runtime API an older runtime lacks, and add the matching constant to the runtime.
const generatorLevel = 2

// Scope is the file code is written into. It qualifies types declared in other packages and
// records the imports they need.
type Scope struct {
	File    *layout.File
	Layout  *layout.Layout
	Imports *ImportSet
}

func NewScope(f *layout.File, l *layout.Layout) *Scope {
	return &Scope{File: f, Layout: l, Imports: NewImportSet()}
}

// Expr returns t as code in the scope's file writes it. A nil type is any.
func (s *Scope) Expr(t gomodel.Type) string {
	switch t := t.(type) {
	case gomodel.Builtin:
		return t.Name
	case gomodel.DeclRef:
		return s.decl(t.Decl)
	case gomodel.Qualified:
		return s.Import(t.Import) + "." + t.Name
	case gomodel.Pointer:
		return "*" + s.Expr(t.Elem)
	case gomodel.Nullable:
		return s.Import(gomodel.Import{Path: gomodel.RuntimePath}) + ".Nullable[" + s.Expr(t.Elem) + "]"
	case gomodel.Slice:
		return "[]" + s.Expr(t.Elem)
	case gomodel.Map:
		return "map[" + s.Expr(t.Key) + "]" + s.Expr(t.Elem)
	}
	return "any"
}

// Import adds imp to the file and returns the name to qualify with.
func (s *Scope) Import(imp gomodel.Import) string {
	return s.Imports.Add(imp.Path, imp.Alias)
}

// Symbol returns name, declared in the file that holds part, as the scope's file writes it.
func (s *Scope) Symbol(part layout.PartID, name string) string {
	target := s.Layout.FileOf(part)
	if target == nil || filepath.Dir(target.Path) == filepath.Dir(s.File.Path) {
		return name
	}
	return s.Imports.Add(target.ImportPath, target.Package) + "." + name
}

// Qualified returns expr, a type expression whose trailing identifier is declared in the package
// imp, as the scope's file writes it: plain in that package or without one, else qualified with
// the name the file imports imp under.
func (s *Scope) Qualified(expr string, imp gomodel.Import) string {
	if imp.Path == "" || imp.Path == s.File.ImportPath {
		return expr
	}
	return Qualify(expr, s.Import(imp))
}

// RuntimeGuard returns the constant a file that imports the runtime refers to, so a runtime too
// old or too new for the file fails to compile. It is empty when the file does not import it
// under a name: its code then uses nothing of the runtime.
func (s *Scope) RuntimeGuard() string {
	name, ok := s.Imports.Name(gomodel.RuntimePath)
	if !ok {
		return ""
	}
	return name + ".SupportsGeneratorV" + strconv.Itoa(generatorLevel)
}

// Value returns v as a value of type t: an enum constant, a list, or a constant.
func (s *Scope) Value(t gomodel.Type, v spec.Value) string {
	r, ok := t.(gomodel.DeclRef)
	switch {
	case ok && r.Decl.Kind == gomodel.KindEnum:
		name, _ := r.Decl.Enum.Const(v)
		return s.Symbol(layout.PartID(r.Decl.Part), name)
	case ok && (r.Decl.Kind == gomodel.KindAlias || r.Decl.Kind == gomodel.KindDefined):
		if l, isList := gomodel.Underlying(t).(gomodel.Slice); isList {
			return s.list(t, l.Elem, v)
		}
		return s.Value(r.Decl.Target, v)
	}
	if l, isList := t.(gomodel.Slice); isList {
		return s.list(t, l.Elem, v)
	}
	return Literal(v)
}

// decl qualifies a declaration placed in another folder with that folder's package.
func (s *Scope) decl(d *gomodel.Decl) string {
	return s.Symbol(layout.PartID(d.Part), d.Name)
}

// list writes the items of v as values of elem, in a composite literal of t.
func (s *Scope) list(t, elem gomodel.Type, v spec.Value) string {
	items := make([]string, len(v.Items))
	for i, item := range v.Items {
		items[i] = s.Value(elem, item)
	}
	return s.Expr(t) + "{" + strings.Join(items, ", ") + "}"
}
