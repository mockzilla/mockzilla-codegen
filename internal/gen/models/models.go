// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package models turns the declarations of the Go model into the data its templates render.
package models

import (
	"embed"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

//go:embed *.tmpl
var templates embed.FS

// partOrder is the order parts are listed in, and so the order they fill a shared file.
var partOrder = []layout.PartID{
	gomodel.PartTypes,
	gomodel.PartEnums,
	gomodel.PartUnions,
	gomodel.PartParams,
	gomodel.PartBodies,
	gomodel.PartResponses,
}

// Generator builds the template data of every models part.
type Generator struct {
	byPart map[layout.PartID][]*gomodel.Decl
}

func New(m *gomodel.Model) *Generator {
	byPart := make(map[layout.PartID][]*gomodel.Decl, len(partOrder))
	for _, d := range m.Decls {
		byPart[layout.PartID(d.Part)] = append(byPart[layout.PartID(d.Part)], d)
	}
	return &Generator{byPart: byPart}
}

// Templates is the models template set. Every part renders through part.tmpl; no block can be
// overridden yet.
func Templates() render.Set {
	parts := make(map[layout.PartID]string, len(partOrder))
	for _, id := range partOrder {
		parts[id] = "part.tmpl"
	}
	return render.Set{Name: "models", FS: templates, Parts: parts}
}

// Parts returns every models part, even one without declarations, with the parts its
// declarations refer to.
func (g *Generator) Parts() []layout.Part {
	out := make([]layout.Part, len(partOrder))
	for i, id := range partOrder {
		used := make(map[layout.PartID]bool)
		for _, d := range g.byPart[id] {
			for _, ref := range declRefs(d) {
				used[layout.PartID(ref.Part)] = true
			}
		}

		out[i] = layout.Part{ID: id}
		for _, other := range partOrder {
			if other != id && used[other] {
				out[i].Uses = append(out[i].Uses, other)
			}
		}
	}
	return out
}

// View returns the data part.tmpl renders part from, with types written as the file of s spells
// them.
func (g *Generator) View(part layout.PartID, s *gocode.Scope) *PartView {
	decls := g.byPart[part]
	v := &PartView{Decls: make([]DeclView, len(decls))}
	for i, d := range decls {
		v.Decls[i] = declView(d, s)
	}
	return v
}

// declRefs returns the declarations the types of d refer to.
func declRefs(d *gomodel.Decl) []*gomodel.Decl {
	types := []gomodel.Type{d.Target}
	if d.Enum != nil {
		types = append(types, d.Enum.Base)
	}
	if d.Struct != nil {
		for _, f := range d.Struct.Fields {
			types = append(types, f.Type)
		}
		if ap := d.Struct.AdditionalProperties; ap != nil {
			types = append(types, ap.Type)
		}
	}
	if d.Union != nil {
		for _, v := range d.Union.Variants {
			types = append(types, v.FieldType)
		}
	}

	var out []*gomodel.Decl
	for _, t := range types {
		out = appendRefs(out, t)
	}
	return out
}

func appendRefs(out []*gomodel.Decl, t gomodel.Type) []*gomodel.Decl {
	switch t := t.(type) {
	case gomodel.DeclRef:
		return append(out, t.Decl)
	case gomodel.Pointer:
		return appendRefs(out, t.Elem)
	case gomodel.Slice:
		return appendRefs(out, t.Elem)
	case gomodel.Map:
		return appendRefs(appendRefs(out, t.Key), t.Elem)
	}
	return out
}
