// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/chi"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// scaffoldConfig places the scaffold files in their own folders, main in a package of its own.
const scaffoldConfig = "output: {file: ./api/gen.go, module: example.com/work}\nserver:\n  framework: chi\n" +
	"  scaffold: {service: ./svc/service.go, middleware: ./mw/middleware.go, main: ./cmd/server/main.go}\n"

func TestNew(t *testing.T) {
	t.Parallel()

	m := petModel()
	m.Operations = append(m.Operations,
		&gomodel.Operation{Name: "Hook", Spec: &spec.Operation{Method: "POST", Path: "/hook", IsWebhook: true}},
		&gomodel.Operation{Name: "Link", Spec: &spec.Operation{Method: "LINK", Path: "/pets", Origin: spec.Origin{Pointer: "/paths/~1pets/link", File: "a.yaml", Line: 3, Col: 5}}},
		&gomodel.Operation{Name: "Bad", Spec: &spec.Operation{Method: "GET", Path: "pets/{id", Origin: spec.Origin{Pointer: "/paths/pets~1{id/get"}}},
		&gomodel.Operation{Name: "DeletePetAgain", Spec: &spec.Operation{Method: "DELETE", Path: "/pets/{petId}", Origin: spec.Origin{Pointer: "/paths/~1pets~1{petId}/delete"}}},
		&gomodel.Operation{Name: "PingAgain", Spec: &spec.Operation{Method: "GET", Path: "/ping", Origin: spec.Origin{Pointer: "/paths/~1ping/get"}}},
	)

	g, diags := New(m, allOptions())

	assert.Equal(t, []framework.Route{
		{Operation: "ListPets", Method: "GET", Path: "/pets", Pattern: "/pets"},
		{Operation: "CreatePet", Method: "POST", Path: "/pets", Pattern: "/pets"},
		{Operation: "DeletePet", Method: "DELETE", Path: "/pets/{id}", Pattern: "/pets/{id}"},
		{Operation: "Ping", Method: "GET", Path: "/ping", Pattern: "/ping"},
	}, g.routes)
	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1pets/link", Origin: diag.Origin{File: "a.yaml", Line: 3, Col: 5}, Message: "Link is not routed: the router does not take the method LINK"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/pets~1{id/get", Message: "Bad is not routed: the router rejects the path: it must begin with /"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1pets~1{petId}/delete", Message: "DeletePetAgain is not routed: names its path parameters otherwise than DeletePet at /pets/{id}"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1ping/get", Message: "PingAgain is not routed: repeats the route of Ping"},
	}, diags)
}

func TestFrameworks(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]framework.Framework{"chi": chi.Framework{}}, Frameworks())
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	sets := Templates(chi.Framework{})

	require.Len(t, sets, 2)
	assert.Equal(t, "server", sets[0].Name)
	assert.Equal(t, "chi", sets[1].Name)
	assert.Equal(t, map[layout.PartID]string{PartRouter: "router.tmpl"}, sets[1].Parts)
	_, err := render.New(sets, render.Options{})
	require.NoError(t, err)
}

func TestParts(t *testing.T) {
	t.Parallel()

	modelParts := []layout.PartID{gomodel.PartParams, gomodel.PartResponses, gomodel.PartTypes}
	base := []layout.Part{
		{ID: PartService, Uses: modelParts},
		{ID: PartErrors},
		{ID: PartAdapter, Uses: []layout.PartID{PartService, gomodel.PartParams, gomodel.PartResponses, gomodel.PartTypes}},
		{ID: PartRouter, Uses: []layout.PartID{PartService, PartAdapter}},
	}
	tests := []struct {
		name     string
		scaffold Scaffold
		want     []layout.Part
	}{
		{name: "No scaffolds", want: base},
		{
			name:     "Main without middleware",
			scaffold: Scaffold{Service: true, Main: true},
			want: append(base,
				layout.Part{ID: layout.PartScaffoldService, Uses: []layout.PartID{PartService, gomodel.PartParams, gomodel.PartResponses, gomodel.PartTypes}},
				layout.Part{ID: layout.PartScaffoldMain, Uses: []layout.PartID{PartAdapter, PartRouter, layout.PartScaffoldService}, Package: "main"},
			),
		},
		{
			name:     "Every scaffold",
			scaffold: Scaffold{Service: true, Middleware: true, Main: true},
			want: append(base,
				layout.Part{ID: layout.PartScaffoldService, Uses: []layout.PartID{PartService, gomodel.PartParams, gomodel.PartResponses, gomodel.PartTypes}},
				layout.Part{ID: layout.PartScaffoldMiddleware},
				layout.Part{ID: layout.PartScaffoldMain, Uses: []layout.PartID{PartAdapter, PartRouter, layout.PartScaffoldService, layout.PartScaffoldMiddleware}, Package: "main"},
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := allOptions()
			opts.Scaffold = tc.scaffold
			g, _ := New(petModel(), opts)

			assert.Equal(t, tc.want, g.Parts())
		})
	}
}

// TestViewRendersParts compares each part with testdata/<part>.golden. UPDATE=1 writes them
// instead.
func TestViewRendersParts(t *testing.T) {
	t.Parallel()

	parts := []layout.PartID{PartService, PartErrors, PartAdapter, PartRouter, layout.PartScaffoldService, layout.PartScaffoldMiddleware, layout.PartScaffoldMain}
	for _, part := range parts {
		t.Run(string(part), func(t *testing.T) {
			t.Parallel()

			m := petModel()
			g, _ := New(m, allOptions())
			got := fixture{m: m, g: g, cfg: scaffoldConfig}.render(t, part)

			path := filepath.Join("testdata", string(part)+".golden")
			if os.Getenv("UPDATE") != "" {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestViewWithoutChecks(t *testing.T) {
	t.Parallel()

	m := petModel()
	opts := allOptions()
	opts.ValidateRequest, opts.ValidateResponse, opts.MultipartMaxMemory = false, false, 0
	g, _ := New(m, opts)

	adapter := string(fixture{m: m, g: g, cfg: scaffoldConfig}.render(t, PartAdapter))

	assert.NotContains(t, adapter, "opts.Validate()")
	assert.NotContains(t, adapter, "ValidateResponse")
	assert.Contains(t, adapter, "MultipartMaxMemory: 33554432")
}

func TestViewWithoutOperations(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{}
	g, _ := New(m, allOptions())
	f := fixture{m: m, g: g, cfg: scaffoldConfig}

	assert.Equal(t, "package api\n\n// PetsInterface is what the generated handlers call. Implement it with the business logic.\n"+
		"type PetsInterface interface {\n}\n", string(f.render(t, PartService)))
	assert.NotContains(t, string(f.render(t, PartRouter)), "adapter")
	assert.NotContains(t, string(f.render(t, layout.PartScaffoldService)), "context")
}

// allOptions asks for every check and scaffold.
func allOptions() Options {
	return Options{
		Name:               "Pets",
		Namer:              naming.New(nil),
		Framework:          chi.Framework{},
		ValidateRequest:    true,
		ValidateResponse:   true,
		MultipartMaxMemory: 8 << 20,
		Scaffold:           Scaffold{Service: true, Middleware: true, Main: true},
		Port:               9090,
		Timeout:            45 * time.Second,
	}
}

// fixture is a model, its generator and the config that lays the parts out.
type fixture struct {
	m   *gomodel.Model
	g   *Generator
	cfg string
}

// render renders one part into the file the layout gives it.
func (f fixture) render(t *testing.T, part layout.PartID) []byte {
	t.Helper()

	e, err := render.New(Templates(f.g.Framework()), render.Options{Format: true})
	require.NoError(t, err)
	s := f.scope(t, part)
	out, err := e.RenderPart(part, f.g.View(part, s))
	require.NoError(t, err)
	got, err := e.RenderFile(render.FileData{Package: s.File.Package, Imports: s.Imports.Decl(), Parts: []string{string(out)}})
	require.NoError(t, err)
	return got
}

// scope is the scope of the file the layout gives part.
func (f fixture) scope(t *testing.T, part layout.PartID) *gocode.Scope {
	t.Helper()

	cfg, err := config.Parse([]byte(f.cfg), "/work")
	require.NoError(t, err)
	l, err := layout.Plan(cfg, append(models.New(f.m).Parts(), f.g.Parts()...), layout.Module{Path: "example.com/work", Dir: "/work"})
	require.NoError(t, err)
	return gocode.NewScope(l.FileOf(part), l)
}

// petModel is a model with every shape the server writes: parameters of each location, one or
// several bodies, bodies without a schema, responses with and without bodies, ranges, default,
// typed headers and error types.
func petModel() *gomodel.Model {
	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	problem := &gomodel.Decl{Name: "Problem", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Error: &gomodel.ErrorMessage{Path: "/detail"}}
	query := &gomodel.Decl{Name: "ListPetsQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Validation: &gomodel.Validation{}, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Limit", Type: gomodel.Pointer{Elem: gomodel.Builtin{Name: "int"}}}, {Name: "Filter", Type: gomodel.Map{Key: str, Elem: str}}},
	}}
	headers := &gomodel.Decl{Name: "ListPetsHeaders", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "XTrace", Type: gomodel.Pointer{Elem: str}}},
	}}
	cookies := &gomodel.Decl{Name: "ListPetsCookies", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Session", Type: str}},
	}}
	querystring := &gomodel.Decl{Name: "ListPetsQueryString", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	path := &gomodel.Decl{Name: "DeletePetPathParams", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "ID", Type: str}},
	}}
	respHeaders := &gomodel.Decl{Name: "ListPetsResponse200Headers", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	errHeaders := &gomodel.Decl{Name: "ListPetsResponseDefaultHeaders", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	pets := &gomodel.Decl{Name: "Pets", Part: gomodel.PartResponses, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}
	note := &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}
	upload := &gomodel.Decl{Name: "Upload", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}

	list := &gomodel.Operation{
		Name: "ListPets",
		Spec: &spec.Operation{Method: "GET", Path: "/pets", Summary: "List pets", Description: "Returns pets."},
		Params: []gomodel.ParamGroup{
			{In: spec.InQuery, Decl: query, Params: []*spec.Parameter{
				{Name: "limit", In: spec.InQuery, Style: "form", Explode: true},
				{Name: "filter", In: spec.InQuery, Style: "deepObject", Explode: true, Required: true},
			}},
			{In: spec.InHeader, Decl: headers, Params: []*spec.Parameter{
				{Name: "X-Trace", In: spec.InHeader, Contents: []*spec.MediaType{{Name: "application/json"}}},
			}},
			{In: spec.InCookie, Decl: cookies, Params: []*spec.Parameter{
				{Name: "session", In: spec.InCookie, Style: "form", Required: true},
			}},
			{In: spec.InQueryString, Decl: querystring},
		},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "application/xml", Type: str}, {MediaType: "application/json", Type: gomodel.DeclRef{Decl: pets}}}, Headers: respHeaders},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}, Headers: errHeaders},
		},
	}
	create := &gomodel.Operation{
		Name: "CreatePet",
		Spec: &spec.Operation{Method: "POST", Path: "/pets", Summary: "Same", Description: "Same", Deprecated: true, Body: &spec.RequestBody{Required: true}},
		Bodies: []gomodel.Content{
			{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "application/x-www-form-urlencoded", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: upload}},
			{MediaType: "text/plain"},
			{MediaType: "text/markdown", Type: gomodel.DeclRef{Decl: note}},
			{MediaType: "image/png"},
			{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}},
		},
		Responses: []gomodel.Response{
			{Status: "201", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}}, Headers: respHeaders},
			{Status: "4XX", Contents: []gomodel.Content{{MediaType: "application/problem+json", Type: gomodel.DeclRef{Decl: problem}}}},
			{Status: "5XX", Contents: []gomodel.Content{{MediaType: "application/problem+json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	del := &gomodel.Operation{
		Name:   "DeletePet",
		Spec:   &spec.Operation{Method: "DELETE", Path: "/pets/{id}", Deprecated: true, Body: &spec.RequestBody{}},
		Params: []gomodel.ParamGroup{{In: spec.InPath, Decl: path, Params: []*spec.Parameter{{Name: "id", In: spec.InPath, Style: "simple", Required: true}}}},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Map{Key: str, Elem: str}}},
		Responses: []gomodel.Response{
			{Status: "204"},
			{Status: "404", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	ping := &gomodel.Operation{
		Name:   "Ping",
		Spec:   &spec.Operation{Method: "GET", Path: "/ping"},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain", Type: str}}},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json"}}},
		},
	}
	return &gomodel.Model{
		Decls:      []*gomodel.Decl{pet, problem, query, headers, cookies, querystring, path, respHeaders, errHeaders, pets, note, upload},
		Operations: []*gomodel.Operation{list, create, del, ping},
	}
}
