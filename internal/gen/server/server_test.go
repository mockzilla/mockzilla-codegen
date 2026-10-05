// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/beego"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/chi"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/echo"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/echov5"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/fasthttp"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/fiber"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gin"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/goframe"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gorillamux"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gozero"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/hertz"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/iris"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/kratos"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/stdhttp"
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

// fixture is a model, its generator, the config that lays the parts out and the block overrides.
type fixture struct {
	m         *gomodel.Model
	g         *Generator
	cfg       string
	templates map[string]string
}

// render renders one part into the file the layout gives it.
func (f fixture) render(t *testing.T, part layout.PartID) []byte {
	t.Helper()

	e, err := render.New(Templates(f.g.Framework()), render.Options{Templates: f.templates, Format: true})
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
	}, g.Routes())
	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1pets/link", Origin: diag.Origin{File: "a.yaml", Line: 3, Col: 5}, Message: "Link is not routed: the router does not take the method LINK"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/pets~1{id/get", Message: "Bad is not routed: the router rejects the path: it must begin with /"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1pets~1{petId}/delete", Message: "DeletePetAgain is not routed: names its path parameters otherwise than DeletePet at /pets/{id}"},
		{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1ping/get", Message: "PingAgain is not routed: repeats the route of Ping"},
		{Severity: diag.Warning, Code: "server-body-unread", Message: "CreatePet takes application/xml, which the server does not decode into Pet; read it from RawRequest"},
	}, diags)
}

func TestNewWarnsOfBodiesTheServerCannotWrite(t *testing.T) {
	t.Parallel()

	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	responses := []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "application/xml", Type: note}}}}
	m := &gomodel.Model{Operations: []*gomodel.Operation{
		{Name: "GetXML", Spec: &spec.Operation{Method: "GET", Path: "/xml", Origin: spec.Origin{Pointer: "/paths/~1xml/get"}}, Responses: responses},
		{Name: "Hook", Spec: &spec.Operation{Method: "POST", Path: "/hook", IsWebhook: true}, Responses: responses},
	}}

	_, diags := New(m, allOptions())

	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     "server-body-unwritable",
		Pointer:  "/paths/~1xml/get",
		Message:  "GetXML answers 200 as application/xml, which the server cannot write Note as; set Body to a string, []byte or runtime.File",
	}}, diags)
}

func TestNewWarnsOfBodiesTheServerCannotRead(t *testing.T) {
	t.Parallel()

	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	scalars := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Scalars", Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: &gomodel.Union{}}}
	bodies := []gomodel.Content{
		{MediaType: "application/octet-stream", Type: gomodel.Pointer{Elem: note}},
		{MediaType: "multipart/form-data", Type: gomodel.Pointer{Elem: scalars}},
		{MediaType: "text/plain", Type: gomodel.Builtin{Name: "string"}},
	}
	m := &gomodel.Model{Operations: []*gomodel.Operation{
		{Name: "PutNote", Spec: &spec.Operation{Method: "PUT", Path: "/note", Origin: spec.Origin{Pointer: "/paths/~1note/put"}}, Bodies: bodies},
		{Name: "Hook", Spec: &spec.Operation{Method: "POST", Path: "/hook", IsWebhook: true}, Bodies: bodies},
	}}

	_, diags := New(m, allOptions())

	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: "server-body-unread", Pointer: "/paths/~1note/put", Message: "PutNote takes application/octet-stream, which the server does not decode into Note; read it from RawRequest"},
		{Severity: diag.Warning, Code: "server-body-unread", Pointer: "/paths/~1note/put", Message: "PutNote takes multipart/form-data, which the server does not decode into Scalars; read it from RawRequest"},
	}, diags)
}

func TestIsWritable(t *testing.T) {
	t.Parallel()

	note := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Note", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}}
	integer := gomodel.Builtin{Name: "int"}
	tests := []struct {
		name    string
		content gomodel.Content
		want    bool
	}{
		{name: "A string anywhere", content: gomodel.Content{MediaType: "application/xml", Type: gomodel.Builtin{Name: "string"}}, want: true},
		{name: "Bytes anywhere", content: gomodel.Content{MediaType: "application/xml"}, want: true},
		{name: "A file anywhere", content: gomodel.Content{MediaType: "image/png", Type: fileType}, want: true},
		{name: "Anything as it is", content: gomodel.Content{MediaType: "application/xml", Type: gomodel.Builtin{Name: "any"}}, want: true},
		{name: "JSON with parameters", content: gomodel.Content{MediaType: "Application/JSON; charset=utf-8", Type: note}, want: true},
		{name: "Frames", content: gomodel.Content{MediaType: "text/event-stream", Type: note}, want: true},
		{name: "A wildcard as JSON", content: gomodel.Content{MediaType: "*/*", Type: note}, want: true},
		{name: "A form", content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: note}, want: true},
		{name: "A struct as multipart", content: gomodel.Content{MediaType: "multipart/form-data", Type: note}, want: true},
		{name: "No struct as multipart", content: gomodel.Content{MediaType: "multipart/form-data", Type: integer}},
		{name: "A number as text", content: gomodel.Content{MediaType: "text/plain", Type: integer}, want: true},
		{name: "A time as text", content: gomodel.Content{MediaType: "text/plain", Type: gomodel.Qualified{Import: gomodel.Import{Path: "time"}, Name: "Time"}}, want: true},
		{name: "A struct as text", content: gomodel.Content{MediaType: "text/plain", Type: note}},
		{name: "A struct as XML", content: gomodel.Content{MediaType: "application/xml", Type: note}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isWritable(tc.content))
		})
	}
}

func TestNewLeavesTheMethodToTheFramework(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{Operations: []*gomodel.Operation{
		{Name: "Search", Spec: &spec.Operation{Method: "QUERY", Path: "/search", Origin: spec.Origin{Pointer: "/paths/~1search/query"}}},
		{Name: "Purge", Spec: &spec.Operation{Method: "PURGE", Path: "/search", Origin: spec.Origin{Pointer: "/paths/~1search/additionalOperations/PURGE"}}},
	}}

	tests := []struct {
		name       string
		fw         framework.Framework
		wantRoutes []framework.Route
		wantDiags  []diag.Diagnostic
	}{
		{
			name: "ServeMux takes any method",
			fw:   stdhttp.Framework{},
			wantRoutes: []framework.Route{
				{Operation: "Search", Method: "QUERY", Path: "/search", Pattern: "QUERY /search"},
				{Operation: "Purge", Method: "PURGE", Path: "/search", Pattern: "PURGE /search"},
			},
		},
		{
			name: "chi takes the methods it has a function for",
			fw:   chi.Framework{},
			wantDiags: []diag.Diagnostic{
				{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1search/query", Message: "Search is not routed: the router does not take the method QUERY"},
				{Severity: diag.Warning, Code: "route-dropped", Pointer: "/paths/~1search/additionalOperations/PURGE", Message: "Purge is not routed: the router does not take the method PURGE"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := allOptions()
			opts.Framework = tc.fw

			g, diags := New(m, opts)

			assert.Equal(t, tc.wantRoutes, g.Routes())
			assert.Equal(t, tc.wantDiags, diags)
		})
	}
}

func TestFrameworks(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]framework.Framework{
		"beego":       beego.Framework{},
		"chi":         chi.Framework{},
		"echo":        echo.Framework{},
		"echo-v5":     echov5.Framework{},
		"fasthttp":    fasthttp.Framework{},
		"fiber":       fiber.Framework{},
		"gin":         gin.Framework{},
		"goframe":     goframe.Framework{},
		"gorilla-mux": gorillamux.Framework{},
		"go-zero":     gozero.Framework{},
		"hertz":       hertz.Framework{},
		"iris":        iris.Framework{},
		"kratos":      kratos.Framework{},
		"std-http":    stdhttp.Framework{},
	}, Frameworks())
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	sets := Templates(chi.Framework{})

	require.Len(t, sets, 2)
	assert.Equal(t, "server", sets[0].Name)
	assert.Equal(t, []string{
		"server.service-header", "server.request-options-extra", "server.response-data-extra", "server.scaffold.service-fields", "server.scaffold.service-method",
	}, sets[0].Blocks)
	assert.Equal(t, "chi", sets[1].Name)
	assert.Equal(t, map[layout.PartID]string{PartRouter: "router.tmpl"}, sets[1].Parts)
	assert.Equal(t, []string{"server.router-extra"}, sets[1].Blocks)
	assert.Equal(t, slices.Concat(sets[0].Blocks, sets[1].Blocks), Blocks())
	_, err := render.New(sets, render.Options{})
	require.NoError(t, err)
}

func TestNeeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		isScaffold bool
		want       map[string]string
	}{
		{name: "Service scaffold written", isScaffold: true},
		{
			name: "Service scaffold left out",
			want: map[string]string{"server.scaffold.service-fields": "server.scaffold.service", "server.scaffold.service-method": "server.scaffold.service"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := allOptions()
			opts.Scaffold.Service = tc.isScaffold
			g, _ := New(petModel(), opts)

			assert.Equal(t, tc.want, g.Needs())
		})
	}
}

// TestBlocks overrides every block with text that reads the user-context, written with and
// without blank space around it.
func TestBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		templates map[string]string
	}{
		{
			name: "Text alone",
			templates: map[string]string{
				blockServiceHeader:         "// Owned by {{.User.owner}}.",
				blockRequestOptionsExtra:   "Owner string // {{.User.owner}}",
				blockResponseDataExtra:     "Owner string // {{.User.owner}}",
				blockRouterExtra:           `r.Get("/owner", {{.User.handler}})`,
				blockScaffoldServiceFields: "owner string // {{.User.owner}}",
				blockScaffoldServiceMethod: "return nil, errors.New({{quote .Name}} + {{quote .User.owner}})",
			},
		},
		{
			name: "Text with line breaks around it",
			templates: map[string]string{
				blockServiceHeader:         "// Owned by {{.User.owner}}.\n",
				blockRequestOptionsExtra:   "\n\tOwner string // {{.User.owner}}",
				blockResponseDataExtra:     "\n\tOwner string // {{.User.owner}}\n\n",
				blockRouterExtra:           "\n\t\tr.Get(\"/owner\", {{.User.handler}})\n",
				blockScaffoldServiceFields: "\n\towner string // {{.User.owner}}\n\n",
				blockScaffoldServiceMethod: "\n\treturn nil, errors.New({{quote .Name}} + {{quote .User.owner}})\n",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := petModel()
			g, _ := New(m, allOptions())
			f := fixture{m: m, g: g, cfg: scaffoldConfig, templates: tc.templates}

			service := string(f.render(t, PartService))
			router := string(f.render(t, PartRouter))
			scaffold := string(f.render(t, layout.PartScaffoldService))

			assert.Contains(t, service, ")\n\n// Owned by platform.\n\n// PetsInterface is what")
			assert.Contains(t, service, "\tCookies    *ListPetsCookies\n\tOwner      string // platform\n\tRawRequest *http.Request\n")
			assert.Contains(t, service, "\tBody    any\n\tOwner   string // platform\n\n\tcontentType string\n")
			assert.Contains(t, router, "r.Get(\"/ping\", a.Ping)\n\t\tr.Get(\"/owner\", ownerHandler)\n\t}\n")
			assert.Contains(t, scaffold, "type Pets struct {\n\towner string // platform\n}\n")
			assert.Contains(t, scaffold, "(*api.PingResponseData, error) {\n\treturn nil, errors.New(\"Ping\" + \"platform\")\n}\n")
		})
	}
}

// TestBlocksOfEveryFramework overrides the block every router template has.
func TestBlocksOfEveryFramework(t *testing.T) {
	t.Parallel()

	for _, name := range slices.Sorted(maps.Keys(Frameworks())) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := petModel()
			opts := allOptions()
			opts.Framework = Frameworks()[name]
			g, _ := New(m, opts)
			f := fixture{m: m, g: g, cfg: scaffoldConfig, templates: map[string]string{blockRouterExtra: "// The routes of {{.User.owner}} end here."}}

			router := string(f.render(t, PartRouter))

			assert.Contains(t, router, ")\n\t\t// The routes of platform end here.\n\t}\n")
		})
	}
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

// TestViewRendersParts compares each part with testdata/<part>.golden, the router of each
// framework with testdata/server.router.<framework>.golden, and the adapter of each framework
// with handlers of its own shape with testdata/server.adapter.<framework>.golden. UPDATE=1 writes
// them instead.
func TestViewRendersParts(t *testing.T) {
	t.Parallel()

	parts := []layout.PartID{PartService, PartErrors, PartAdapter, layout.PartScaffoldService, layout.PartScaffoldMiddleware, layout.PartScaffoldMain}
	for _, part := range parts {
		t.Run(string(part), func(t *testing.T) {
			t.Parallel()

			m := petModel()
			g, _ := New(m, allOptions())
			got := fixture{m: m, g: g, cfg: scaffoldConfig}.render(t, part)

			assertGolden(t, filepath.Join("testdata", string(part)+".golden"), got)
		})
	}
	for _, name := range slices.Sorted(maps.Keys(Frameworks())) {
		fw := Frameworks()[name]
		perFramework := []layout.PartID{PartRouter}
		if fw.Family() == framework.Native {
			perFramework = append(perFramework, PartAdapter)
		}
		if ownsMain(fw) {
			perFramework = append(perFramework, layout.PartScaffoldMain)
		}
		for _, part := range perFramework {
			t.Run(string(part)+" "+name, func(t *testing.T) {
				t.Parallel()

				m := petModel()
				opts := allOptions()
				opts.Framework = fw
				g, _ := New(m, opts)
				got := fixture{m: m, g: g, cfg: scaffoldConfig}.render(t, part)

				assertGolden(t, filepath.Join("testdata", string(part)+"."+name+".golden"), got)
			})
		}
	}
}

// assertGolden compares got with the file at path, or writes it when UPDATE is set.
func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()

	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
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
		User:               map[string]any{"owner": "platform", "handler": "ownerHandler"},
	}
}

// petModel is a model with every shape the server writes: parameters of each location with
// defaults, querystrings, one or several bodies, bodies without a schema, responses with and
// without bodies, ranges, default, typed headers and error types.
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
			}, Defaults: map[string]string{"limit": "20"}},
			{In: spec.InHeader, Decl: headers, Params: []*spec.Parameter{
				{Name: "X-Trace", In: spec.InHeader, Contents: []*spec.MediaType{{Name: "application/json"}}},
			}, Defaults: map[string]string{"X-Trace": `"none"`}},
			{In: spec.InCookie, Decl: cookies, Params: []*spec.Parameter{
				{Name: "session", In: spec.InCookie, Style: "form", Required: true},
			}},
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
		QueryString: &gomodel.QueryString{
			Param:   &spec.Parameter{Name: "filter", In: spec.InQueryString},
			Content: gomodel.Content{MediaType: "application/x-www-form-urlencoded", Type: gomodel.DeclRef{Decl: pet}},
			Default: `{"name":"all"}`,
		},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Map{Key: str, Elem: str}}},
		Responses: []gomodel.Response{
			{Status: "204"},
			{Status: "404", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	ping := &gomodel.Operation{
		Name:        "Ping",
		Spec:        &spec.Operation{Method: "GET", Path: "/ping"},
		QueryString: &gomodel.QueryString{Param: &spec.Parameter{Name: "body", In: spec.InQueryString, Required: true}, Content: gomodel.Content{MediaType: "application/json"}},
		Bodies:      []gomodel.Content{{MediaType: "application/json", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain", Type: str}}},
			{Status: "202", Contents: []gomodel.Content{{MediaType: "application/x-ndjson", Item: gomodel.DeclRef{Decl: pet}}}},
			{Status: "203", Contents: []gomodel.Content{
				{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}},
				{MediaType: "text/event-stream", Type: gomodel.DeclRef{Decl: pet}, Item: gomodel.DeclRef{Decl: pet}},
			}},
			{Status: "206", Contents: []gomodel.Content{{MediaType: "*/*", Type: gomodel.DeclRef{Decl: pet}}}},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json"}}},
		},
	}
	return &gomodel.Model{
		Decls:      []*gomodel.Decl{pet, problem, query, headers, cookies, path, respHeaders, errHeaders, pets, note, upload},
		Operations: []*gomodel.Operation{list, create, del, ping},
	}
}
