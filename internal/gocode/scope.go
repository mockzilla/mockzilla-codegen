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

	"github.com/mockzilla/codegen/internal/gomodel"
	"github.com/mockzilla/codegen/internal/layout"
)

// generatorLevel is the runtime API level generated code needs. Raise it when generated code
// starts to use runtime API an older runtime lacks, and add the matching constant to the runtime.
const generatorLevel = 1

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

// RuntimeGuard returns the constant a file that imports the runtime refers to, so a runtime too
// old or too new for the file fails to compile. It is empty when the file does not import it.
func (s *Scope) RuntimeGuard() string {
	if !s.Imports.Has(gomodel.RuntimePath) {
		return ""
	}
	return s.Imports.Add(gomodel.RuntimePath, "") + ".SupportsGeneratorV" + strconv.Itoa(generatorLevel)
}

// decl qualifies a declaration placed in another folder with that folder's package.
func (s *Scope) decl(d *gomodel.Decl) string {
	target := s.Layout.FileOf(layout.PartID(d.Part))
	if target == nil || filepath.Dir(target.Path) == filepath.Dir(s.File.Path) {
		return d.Name
	}
	return s.Imports.Add(target.ImportPath, target.Package) + "." + d.Name
}
