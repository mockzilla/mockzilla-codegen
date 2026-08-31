// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func TestParts(t *testing.T) {
	t.Parallel()

	g := New(petModel(), "Pets", naming.New(nil))

	assert.Equal(t, []layout.Part{{ID: PartService, Uses: []layout.PartID{gomodel.PartParams, gomodel.PartResponses, gomodel.PartTypes}}}, g.Parts())
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	set := Templates()

	assert.Equal(t, "server", set.Name)
	assert.Equal(t, map[layout.PartID]string{PartService: "service.tmpl"}, set.Parts)
	_, err := render.New([]render.Set{set}, render.Options{})
	require.NoError(t, err)
}

// TestViewRendersService compares with testdata/service.golden. UPDATE=1 writes it instead.
func TestViewRendersService(t *testing.T) {
	t.Parallel()

	m := petModel()
	g := New(m, "Pets", naming.New(nil))
	s := scope(t, m, g)
	e, err := render.New([]render.Set{Templates()}, render.Options{Format: true})
	require.NoError(t, err)

	out, err := e.RenderPart(PartService, g.View(PartService, s))
	require.NoError(t, err)
	got, err := e.RenderFile(render.FileData{Package: "api", Imports: s.Imports.Decl(), Parts: []string{string(out)}})
	require.NoError(t, err)

	path := filepath.Join("testdata", "service.golden")
	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

// petModel is a model with every shape the contract writes: parameters of each location, one or
// several bodies, bodies without a schema, responses with and without bodies, ranges, default,
// and typed headers.
func petModel() *gomodel.Model {
	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	problem := &gomodel.Decl{Name: "Problem", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	query := &gomodel.Decl{Name: "ListPetsQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	path := &gomodel.Decl{Name: "GetPetPathParams", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	headers := &gomodel.Decl{Name: "ListPetsResponse200Headers", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	errHeaders := &gomodel.Decl{Name: "ListPetsResponseDefaultHeaders", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	pets := &gomodel.Decl{Name: "Pets", Part: gomodel.PartResponses, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}

	list := &gomodel.Operation{
		Name:   "ListPets",
		Spec:   &spec.Operation{Summary: "List pets", Description: "Returns pets."},
		Params: []gomodel.ParamGroup{{In: spec.InQuery, Decl: query}, {In: "matrix", Decl: path}},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "application/xml", Type: str}, {MediaType: "application/json", Type: gomodel.DeclRef{Decl: pets}}}, Headers: headers},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}, Headers: errHeaders},
		},
	}
	create := &gomodel.Operation{
		Name:   "CreatePet",
		Spec:   &spec.Operation{Summary: "Same", Description: "Same", Deprecated: true},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}, {MediaType: "text/plain"}, {MediaType: "image/png"}},
		Responses: []gomodel.Response{
			{Status: "201", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}}, Headers: headers},
			{Status: "4XX", Contents: []gomodel.Content{{MediaType: "application/problem+json"}}},
		},
	}
	del := &gomodel.Operation{
		Name:      "DeletePet",
		Spec:      &spec.Operation{Deprecated: true},
		Params:    []gomodel.ParamGroup{{In: spec.InPath, Decl: path}},
		Bodies:    []gomodel.Content{{MediaType: "application/json", Type: gomodel.Map{Key: str, Elem: str}}},
		Responses: []gomodel.Response{{Status: "204"}},
	}
	ping := &gomodel.Operation{
		Name:      "Ping",
		Spec:      &spec.Operation{},
		Bodies:    []gomodel.Content{{MediaType: "application/json", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain", Type: str}}}},
	}
	return &gomodel.Model{Decls: []*gomodel.Decl{pet, problem, query, path, headers, errHeaders, pets}, Operations: []*gomodel.Operation{list, create, del, ping}}
}

// scope places every part in one file and returns the scope of that file.
func scope(t *testing.T, m *gomodel.Model, g *Generator) *gocode.Scope {
	t.Helper()

	cfg, err := config.Parse([]byte("server: {framework: chi}\n"), "/work")
	require.NoError(t, err)
	parts := append(models.New(m).Parts(), g.Parts()...)
	l, err := layout.Plan(cfg, parts, layout.Module{})
	require.NoError(t, err)
	return gocode.NewScope(l.Files[0], l)
}
