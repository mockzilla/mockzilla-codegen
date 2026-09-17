// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestRequestOptionsView(t *testing.T) {
	t.Parallel()

	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	query := &gomodel.Decl{Name: "Query", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	op := &gomodel.Operation{
		Name:   "CreatePet",
		Spec:   &spec.Operation{Method: "POST", Path: "/pets"},
		Params: []gomodel.ParamGroup{{In: spec.InQuery, Decl: query}},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}, {MediaType: "text/plain"}},
	}
	m := &gomodel.Model{Decls: []*gomodel.Decl{pet, query}, Operations: []*gomodel.Operation{op}}
	g, _ := New(m, allOptions())
	f := fixture{m: m, g: g, cfg: "output: {file: ./gen.go}\n"}

	got := requestOptionsView(f.g, op, f.scope(t, PartOptions))

	assert.Equal(t, RequestOptionsView{
		Name: "CreatePet",
		Type: "CreatePetRequestOptions",
		Fields: []FieldView{
			{Name: "Query", Type: "*Query"},
			{Name: "BodyJSON", Type: "*Pet", Doc: "Body sent as application/json."},
			{Name: "BodyText", Type: "string", Doc: "Body sent as text/plain."},
		},
		Checks: []CheckView{{Field: "BodyJSON", Path: `"body"`}},
	}, got)
}
