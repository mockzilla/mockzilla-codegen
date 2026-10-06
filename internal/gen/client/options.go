// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of options.tmpl: the request options of each operation and their Validate checks.

package client

import (
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// OptionsView is the data of the options part: the request options type of every operation.
type OptionsView struct {
	Operations []RequestOptionsView
}

// RequestOptionsView is the request options of one operation: a field per parameter group and
// per body, and the checks Validate runs. Validation is set when there are checks.
type RequestOptionsView struct {
	Name       string
	Type       string
	Validation string
	Fields     []FieldView
	Checks     []CheckView
}

// FieldView is one field of a generated struct.
type FieldView struct {
	Name string
	Type string
	Doc  string
}

// CheckView validates the Field of the options under Path, a quoted name, when it is set.
type CheckView struct {
	Field string
	Path  string
}

func optionsView(g *Generator, s *gocode.Scope) *OptionsView {
	v := &OptionsView{}
	if len(g.ops) == 0 {
		return v
	}

	for _, op := range g.ops {
		v.Operations = append(v.Operations, requestOptionsView(g, op, s))
	}
	return v
}

func requestOptionsView(g *Generator, op *gomodel.Operation, s *gocode.Scope) RequestOptionsView {
	n := g.opts.Namer
	v := RequestOptionsView{Name: op.Name, Type: n.ClientRequestOptions(op.Name)}
	for _, p := range op.Params {
		field := operation.GroupField(p.In, n)
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(gomodel.Pointer{Elem: gomodel.DeclRef{Decl: p.Decl}})})
		if p.Decl.Validation != nil {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(p.In)})
		}
	}
	if qs := op.QueryString; qs != nil {
		field, t := operation.QueryStringField(op, n), operation.QueryStringType(qs)
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(t), Doc: "Query sent as " + qs.Content.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(spec.InQueryString)})
		}
	}

	fields := operation.BodyFields(op.Bodies, n)
	for i, c := range op.Bodies {
		t := operation.BodyType(c)
		v.Fields = append(v.Fields, FieldView{Name: fields[i], Type: s.Expr(t), Doc: "Body sent as " + c.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: fields[i], Path: gocode.Quote("body")})
		}
	}

	if len(v.Checks) > 0 {
		v.Validation = s.Import(gomodel.Import{Path: gomodel.ValidationPath})
	}
	return v
}
