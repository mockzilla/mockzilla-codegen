// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of inputs.tmpl: the input struct of each tool.

package mcp

import (
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// InputsView is the data of the inputs part: the input type of every tool.
type InputsView struct {
	Inputs []InputView
}

// InputView is the input of one tool: a field per parameter and one for the body, tagged with
// the property names of the schema.
type InputView struct {
	Type   string
	Tool   string
	Fields []FieldView
}

// FieldView is one field of an input struct.
type FieldView struct {
	Name string
	Type string
	Tag  string
	Doc  string
}

func inputsView(g *Generator, s *gocode.Scope) *InputsView {
	v := &InputsView{}
	for _, t := range g.tools {
		v.Inputs = append(v.Inputs, inputView(g, t, s))
	}
	return v
}

func inputView(g *Generator, t *tool, s *gocode.Scope) InputView {
	v := InputView{Type: g.opts.Namer.ToolInput(t.op.Name), Tool: t.name}
	for _, p := range t.params {
		v.Fields = append(v.Fields, FieldView{
			Name: p.goName,
			Type: s.Expr(p.field.Type),
			Tag:  jsonTag(p.name, p.spec.Required || p.spec.In == spec.InPath),
			Doc:  p.spec.Description,
		})
	}
	if b := t.body; b != nil {
		doc := bodyDescription(t.op)
		if doc == "" {
			doc = "The request body, sent as " + b.content.MediaType + "."
		}
		v.Fields = append(v.Fields, FieldView{Name: b.goName, Type: s.Expr(operation.BodyType(b.content)), Tag: jsonTag(b.name, b.isRequired), Doc: doc})
	}
	return v
}

// jsonTag is the json tag of a field named name, left out of the output when it is not required
// and empty.
func jsonTag(name string, isRequired bool) string {
	if !isRequired {
		name += ",omitempty"
	}
	return gocode.Tag([]gomodel.Tag{{Key: "json", Value: name}})
}
