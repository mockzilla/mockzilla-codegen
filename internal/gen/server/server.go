// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package server turns the operations of the Go model into the service contract: the interface a
// service implements, what each method receives and what it returns.
package server

import (
	"embed"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// PartService holds the service interface, the request options and the response data.
const PartService layout.PartID = "server.service"

//go:embed *.tmpl
var templates embed.FS

// Generator builds the template data of the server parts.
type Generator struct {
	name  string
	namer *naming.Namer
	ops   []*gomodel.Operation
}

// New returns the generator of the server parts of m, with the service interface named after name.
func New(m *gomodel.Model, name string, n *naming.Namer) *Generator {
	return &Generator{name: name, namer: n, ops: m.Operations}
}

// Templates is the server template set.
func Templates() render.Set {
	return render.Set{Name: "server", FS: templates, Parts: map[layout.PartID]string{PartService: "service.tmpl"}}
}

// Parts returns the server parts with the parts their types come from.
func (g *Generator) Parts() []layout.Part {
	var uses []layout.PartID
	for _, op := range g.ops {
		for _, t := range operationTypes(op) {
			for _, d := range decls(t) {
				if id := layout.PartID(d.Part); !slices.Contains(uses, id) {
					uses = append(uses, id)
				}
			}
		}
	}
	slices.Sort(uses)
	return []layout.Part{{ID: PartService, Uses: uses}}
}

// View returns the data of part, with types written as the file of s spells them.
func (g *Generator) View(_ layout.PartID, s *gocode.Scope) *ServiceView {
	return serviceView(g, s)
}

// operationTypes lists the types an operation's contract names.
func operationTypes(op *gomodel.Operation) []gomodel.Type {
	var out []gomodel.Type
	for _, p := range op.Params {
		out = append(out, gomodel.DeclRef{Decl: p.Decl})
	}
	for _, c := range op.Bodies {
		out = append(out, c.Type)
	}
	for _, r := range op.Responses {
		for _, c := range r.Contents {
			out = append(out, c.Type)
		}
		if r.Headers != nil {
			out = append(out, gomodel.DeclRef{Decl: r.Headers})
		}
	}
	return out
}

// decls returns the declarations a type refers to.
func decls(t gomodel.Type) []*gomodel.Decl {
	switch t := t.(type) {
	case gomodel.DeclRef:
		return []*gomodel.Decl{t.Decl}
	case gomodel.Pointer:
		return decls(t.Elem)
	case gomodel.Slice:
		return decls(t.Elem)
	case gomodel.Map:
		return slices.Concat(decls(t.Key), decls(t.Elem))
	}
	return nil
}
