// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// scope places every part of g in one file and returns the scope of that file.
func scope(t *testing.T, g *Generator) *gocode.Scope {
	t.Helper()

	cfg, err := config.Parse(nil, "/work")
	require.NoError(t, err)
	l, err := layout.Plan(cfg, g.Parts(), layout.Module{})
	require.NoError(t, err)
	return gocode.NewScope(l.Files[0], l)
}

func TestParts(t *testing.T) {
	t.Parallel()

	status := &gomodel.Decl{Name: "Status", Part: gomodel.PartEnums, Kind: gomodel.KindEnum, Enum: &gomodel.Enum{Base: gomodel.Builtin{Name: "string"}}}
	payment := &gomodel.Decl{Name: "Payment", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{
			{Name: "Status", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: status}}},
			{Name: "Pay", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: payment}}},
		},
		AdditionalProperties: &gomodel.Field{Type: gomodel.Map{Key: gomodel.Builtin{Name: "string"}, Elem: gomodel.DeclRef{Decl: status}}},
	}}
	payment.Union = groupUnion(gomodel.Group{Variants: []*gomodel.Variant{{Name: "Pet", FieldType: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}}})
	query := &gomodel.Decl{Name: "ListQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Pets", Type: gomodel.Map{Key: gomodel.Builtin{Name: "string"}, Elem: gomodel.DeclRef{Decl: pet}}}},
	}}
	pets := &gomodel.Decl{Name: "Pets", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}

	g := New(&gomodel.Model{Decls: []*gomodel.Decl{pet, status, payment, query, pets}})

	const reason = "and models.types can refer to each other's types"
	assert.Equal(t, []layout.Part{
		{ID: gomodel.PartTypes, Uses: []layout.PartID{gomodel.PartEnums, gomodel.PartUnions}},
		{ID: gomodel.PartEnums, Owner: gomodel.PartTypes, Reason: reason},
		{ID: gomodel.PartUnions, Uses: []layout.PartID{gomodel.PartTypes}, Owner: gomodel.PartTypes, Reason: reason},
		{ID: gomodel.PartParams, Uses: []layout.PartID{gomodel.PartTypes}, Owner: gomodel.PartTypes, Reason: reason},
		{ID: gomodel.PartBodies, Owner: gomodel.PartTypes, Reason: reason},
		{ID: gomodel.PartResponses, Owner: gomodel.PartTypes, Reason: reason},
	}, g.Parts())
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	set := Templates()

	assert.Equal(t, "models", set.Name)
	assert.Empty(t, set.Blocks)
	assert.Len(t, set.Parts, len(partOrder))
	for _, id := range partOrder {
		assert.Equal(t, "part.tmpl", set.Parts[id])
	}
	_, err := render.New([]render.Set{set}, render.Options{})
	require.NoError(t, err)
}
