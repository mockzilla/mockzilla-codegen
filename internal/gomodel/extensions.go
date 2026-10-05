// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The x-go-* extensions that set a Go name or type, read once per place.

package gomodel

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// extReader parses the extensions of each place once, so its warnings come out once. Places are
// told apart by their JSON pointer.
type extReader struct {
	namer *naming.Namer
	diags *diag.Collector
	memo  map[string]extension.Set
}

func newExtReader(n *naming.Namer, diags *diag.Collector) *extReader {
	return &extReader{namer: n, diags: diags, memo: map[string]extension.Set{}}
}

func (r *extReader) of(exts []spec.Extension, at spec.Origin) extension.Set {
	if s, ok := r.memo[at.Pointer]; ok {
		return s
	}
	s, diags := extension.Parse(exts, at)
	r.diags.Append(diags...)
	r.memo[at.Pointer] = s
	return s
}

// goName is the Go name x-go-name asks for: as written with x-go-name-exact, else exported.
func (r *extReader) goName(set extension.Set, name string) string {
	if name == "" || set.IsExactName {
		return name
	}
	return r.namer.Exported(name)
}

// goType is the type x-go-type names. A name with a dot is a type of the package x-go-type-import
// gives, else of the one of imports with that name, else of the standard library package the dot
// follows; anything else is written as is.
func goType(t *extension.Type, imports []config.Import) Type {
	if t.Name == "[]byte" {
		// Bodies and checks tell bytes apart by this type, not by its text.
		return Slice{Elem: byteType}
	}
	qual, name, isQualified := strings.Cut(t.Name, ".")
	if !isQualified || strings.ContainsAny(t.Name, "*[]{}() ") || strings.Contains(name, ".") {
		return Builtin{Name: t.Name}
	}

	if t.Path != "" {
		return Qualified{Import: Import{Path: t.Path, Alias: t.Alias}, Name: name}
	}
	if i := slices.IndexFunc(imports, func(listed config.Import) bool { return listed.Name() == qual }); i >= 0 {
		return Qualified{Import: Import{Path: imports[i].Package, Alias: qual}, Name: name}
	}
	return Qualified{Import: Import{Path: qual, Alias: t.Alias}, Name: name}
}
