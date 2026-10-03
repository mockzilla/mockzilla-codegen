// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// splitConfig places the models, the request options and the tool inputs in a folder of their
// own, so the tools qualify them.
const splitConfig = "output:\n  file: ./api/gen.go\n  module: example.com/work\n  files:\n    ./types/types.go: [client.options, mcp.inputs, models]\n"

func TestNew(t *testing.T) {
	t.Parallel()

	g, diags := New(petModel(), testOptions())

	names := make([]string, len(g.tools))
	for i, tool := range g.tools {
		names[i] = tool.name
	}
	assert.Equal(t, []string{"list_pets", "create_pet", "get_pet", "get_pet2", "put_pet", "remove-pet", "ping", "note", "upload", "tail"}, names)
	assert.Equal(t, []diag.Diagnostic{
		{
			Severity: diag.Warning,
			Code:     diag.CodeMCPToolName,
			Pointer:  "/paths/~1pets~1{id}/put/x-mcp/name",
			Origin:   diag.Origin{File: "api.yaml", Line: 30, Col: 5},
			Message:  `x-mcp.name "put pet!" is no tool name, which has letters, digits, _ - and . up to 128 characters; the tool is named "put_pet"`,
		},
		{
			Severity: diag.Info,
			Code:     diag.CodeNameClash,
			Pointer:  "/paths/~1pets~1{id}/post",
			Origin:   diag.Origin{File: "api.yaml", Line: 25, Col: 5},
			Message:  `"get_pet" is used by /paths/~1pets~1{id}/get, renamed to "get_pet2"`,
		},
	}, diags)
	assert.Equal(t, "Fetch a pet by its id.", g.tools[2].desc, "x-mcp.description wins")
	assert.Equal(t, "List pets\n\nReturns pets.", g.tools[0].desc)
}

func TestNewWithDefaultSkip(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.DefaultSkip = true

	g, _ := New(petModel(), opts)

	names := make([]string, len(g.tools))
	for i, tool := range g.tools {
		names[i] = tool.name
	}
	assert.Equal(t, []string{"get_pet", "remove-pet"}, names, "only x-mcp.skip false keeps an operation")
}

func TestNewReportsBadExtensions(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{Operations: []*gomodel.Operation{{
		Name: "Ping",
		Spec: &spec.Operation{ID: "ping", Method: "GET", Path: "/ping", Extensions: []spec.Extension{{Name: "x-mcp", Value: spec.Value{Kind: spec.KindString, Str: "yes"}}}},
	}}}

	g, diags := New(m, testOptions())

	assert.Len(t, g.tools, 1)
	assert.Equal(t, []diag.Diagnostic{{Severity: diag.Warning, Code: diag.CodeExtensionValue, Pointer: "/x-mcp", Message: "x-mcp must be an object; it is left out"}}, diags)
}

func TestNewWarnsOnceAboutADefault(t *testing.T) {
	t.Parallel()

	limit := &spec.Schema{Types: spec.TypeInteger, Default: &spec.Value{Kind: spec.KindString, Str: "20"}, Origin: spec.Origin{Pointer: "/components/schemas/Limit", File: "api.yaml", Line: 40, Col: 5}}
	limitBody := &spec.RequestBody{Contents: []*spec.MediaType{{Name: "application/json", Schema: &spec.Schema{Ref: &spec.Ref{Pointer: "/components/schemas/Limit", Name: "Limit", Target: limit}}}}}
	op := func(id string) *gomodel.Operation {
		return &gomodel.Operation{
			Name:   id,
			Spec:   &spec.Operation{ID: id, Method: "POST", Path: "/" + id, Body: limitBody},
			Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Builtin{Name: "int"}}},
		}
	}

	g, diags := New(&gomodel.Model{Operations: []*gomodel.Operation{op("setA"), op("setB")}}, testOptions())

	assert.Len(t, g.tools, 2)
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodeDefaultIgnored,
		Pointer:  "/components/schemas/Limit",
		Origin:   diag.Origin{File: "api.yaml", Line: 40, Col: 5},
		Message:  `the default "20" does not fit its schema, so the tool input leaves it out: it is a string, the schema wants integer`,
	}}, diags)
	assert.Contains(t, g.tools[1].schema, `"$defs":{"Limit":{"type":"integer"}}`)
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	set := Templates()

	assert.Equal(t, "mcp", set.Name)
	assert.Equal(t, map[layout.PartID]string{PartTools: "tools.tmpl", PartInputs: "inputs.tmpl"}, set.Parts)
	_, err := render.New([]render.Set{set}, render.Options{})
	require.NoError(t, err)
}

func TestParts(t *testing.T) {
	t.Parallel()

	g, _ := New(petModel(), testOptions())

	assert.Equal(t, []layout.Part{
		{ID: PartInputs, Uses: []layout.PartID{gomodel.PartBodies, gomodel.PartTypes}},
		{ID: PartTools, Uses: []layout.PartID{client.PartOperations, client.PartOptions, PartInputs, gomodel.PartParams}},
	}, g.Parts())
}

// TestViewRendersParts compares each part with testdata/<part>.golden. UPDATE=1 writes them
// instead.
func TestViewRendersParts(t *testing.T) {
	t.Parallel()

	for _, part := range []layout.PartID{PartTools, PartInputs} {
		t.Run(string(part), func(t *testing.T) {
			t.Parallel()

			m := petModel()
			g, _ := New(m, testOptions())
			got := fixture{m: m, g: g, cfg: splitConfig}.render(t, part)

			path := filepath.Join("testdata", string(part)+".golden")
			if os.Getenv("UPDATE") != "" {
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestViewWithoutTools(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{}
	g, _ := New(m, testOptions())
	f := fixture{m: m, g: g, cfg: splitConfig}

	assert.Equal(t, "package types\n", string(f.render(t, PartInputs)))
	assert.Equal(t, "package api\n\nimport \"github.com/modelcontextprotocol/go-sdk/mcp\"\n\n"+
		"// MCPTools exposes the operations of the API as MCP tools, each calling the client.\ntype MCPTools struct {\n\tclient PetClientInterface\n}\n\n"+
		"// NewMCPTools returns the tools that call c.\nfunc NewMCPTools(c PetClientInterface) *MCPTools {\n\treturn &MCPTools{client: c}\n}\n\n"+
		"// Register adds every tool to s. To add a few, pass the definition and the handler of each to\n// mcp.AddTool instead.\nfunc (t *MCPTools) Register(s *mcp.Server) {\n}\n", string(f.render(t, PartTools)))
}

func TestIsToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "Letters, digits and the three symbols", in: "get-pet_v2.1", want: true},
		{name: "Empty", in: ""},
		{name: "A space", in: "get pet"},
		{name: "A character outside ASCII", in: "gét"},
		{name: "Too long", in: string(make([]byte, 129))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isToolName(tc.in))
		})
	}
}

func testOptions() Options {
	return Options{Client: "PetClient", Namer: naming.New(nil), User: map[string]any{"owner": "platform"}}
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

	e, err := render.New([]render.Set{Templates()}, render.Options{Format: true})
	require.NoError(t, err)
	s := f.scope(t, part)
	out, err := e.RenderPart(part, f.g.View(part, s))
	require.NoError(t, err)
	got, err := e.RenderFile(render.FileData{Package: s.File.Package, Imports: s.Imports.Decl(), Parts: []string{string(out)}})
	require.NoError(t, err)
	return got
}

// scope is the scope of the file the layout gives part, next to the client's parts.
func (f fixture) scope(t *testing.T, part layout.PartID) *gocode.Scope {
	t.Helper()

	cfg, err := config.Parse([]byte(f.cfg), "/work")
	require.NoError(t, err)
	cl, _ := client.New(f.m, client.Options{Name: "PetClient", Namer: naming.New(nil), HasStreams: true})
	parts := append(models.New(f.m).Parts(), cl.Parts()...)
	l, err := layout.Plan(cfg, append(parts, f.g.Parts()...), layout.Module{Path: "example.com/work", Dir: "/work"})
	require.NoError(t, err)
	return gocode.NewScope(l.FileOf(part), l)
}

// xmcp is an x-mcp extension with the given fields.
func xmcp(fields ...spec.Field) []spec.Extension {
	return []spec.Extension{{Name: "x-mcp", Value: spec.Value{Kind: spec.KindObject, Fields: fields}}}
}

func strField(name, value string) spec.Field {
	return spec.Field{Name: name, Value: spec.Value{Kind: spec.KindString, Str: value}}
}

func boolField(name string, value bool) spec.Field {
	return spec.Field{Name: name, Value: spec.Value{Kind: spec.KindBool, Bool: value}}
}

// petModel is a model with every shape the tools write: parameters of each location with a name
// taken twice, a querystring group that is left out, bodies of every kind, results that are
// values, text, pointers to text and nothing, an operation that streams alone, x-mcp in every
// form, two operations whose tools would share a name, and a webhook.
func petModel() *gomodel.Model {
	str := gomodel.Builtin{Name: "string"}
	strSchema := &spec.Schema{Types: spec.TypeString}
	intSchema := &spec.Schema{Types: spec.TypeInteger}
	petSchema := &spec.Schema{Types: spec.TypeObject, Required: []string{"name"}, Properties: []*spec.Property{{Name: "name", Schema: strSchema, Required: true}}}
	petRef := &spec.Schema{Ref: &spec.Ref{Pointer: "/components/schemas/Pet", Name: "Pet", Target: petSchema}}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	problem := &gomodel.Decl{Name: "Problem", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Error: &gomodel.ErrorMessage{Path: "detail"}}
	note := &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}
	upload := &gomodel.Decl{Name: "Upload", Part: gomodel.PartBodies, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	file := gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
	query := &gomodel.Decl{Name: "ListPetsQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Limit", Type: gomodel.Pointer{Elem: gomodel.Builtin{Name: "int"}}}, {Name: "Filter", Type: gomodel.Map{Key: str, Elem: str}}},
	}}
	headers := &gomodel.Decl{Name: "ListPetsHeaders", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "XTrace", Type: gomodel.Pointer{Elem: str}}},
	}}
	cookies := &gomodel.Decl{Name: "ListPetsCookies", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Session", Type: str}},
	}}
	querystring := &gomodel.Decl{Name: "ListPetsQueryString", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{{Name: "Raw", Type: str}}}}
	path := &gomodel.Decl{Name: "GetPetPathParams", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{{Name: "ID", Type: str}}}}
	getQuery := &gomodel.Decl{Name: "GetPetQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "ID", Type: gomodel.Pointer{Elem: str}}, {Name: "Body", Type: gomodel.Pointer{Elem: str}}},
	}}
	jsonBody := &spec.RequestBody{Required: true, Description: "The pet to add.", Contents: []*spec.MediaType{{Name: "application/xml", Schema: petRef}, {Name: "application/json", Schema: petRef}, {Name: "text/plain"}}}

	list := &gomodel.Operation{
		Name: "ListPets",
		Spec: &spec.Operation{ID: "listPets", Method: "GET", Path: "/pets", Origin: spec.Origin{Pointer: "/paths/~1pets/get"}, Summary: "List pets", Description: "Returns pets."},
		Params: []gomodel.ParamGroup{
			{In: spec.InQuery, Decl: query, Params: []*spec.Parameter{
				{Name: "limit", In: spec.InQuery, Description: "How many at most.", Schema: intSchema, Deprecated: true},
				{Name: "filter", In: spec.InQuery, Required: true, Contents: []*spec.MediaType{{Name: "application/json", Schema: &spec.Schema{Types: spec.TypeObject}}}},
			}},
			{In: spec.InHeader, Decl: headers, Params: []*spec.Parameter{{Name: "X-Trace", In: spec.InHeader, Schema: strSchema}}},
			{In: spec.InCookie, Decl: cookies, Params: []*spec.Parameter{{Name: "session", In: spec.InCookie, Required: true, Schema: strSchema}}},
			{In: spec.InQueryString, Decl: querystring, Params: []*spec.Parameter{{Name: "raw", In: spec.InQueryString}}},
		},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}}},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	create := &gomodel.Operation{
		Name: "CreatePet",
		Spec: &spec.Operation{ID: "createPet", Method: "POST", Path: "/pets", Origin: spec.Origin{Pointer: "/paths/~1pets/post"}, Body: jsonBody, Extensions: xmcp(strField("description", ""))},
		Bodies: []gomodel.Content{
			{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "text/plain"},
		},
		Responses: []gomodel.Response{{Status: "201", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}}}},
	}
	get := &gomodel.Operation{
		Name: "GetPet",
		Spec: &spec.Operation{ID: "getPet", Method: "GET", Path: "/pets/{id}", Origin: spec.Origin{Pointer: "/paths/~1pets~1{id}/get", File: "api.yaml", Line: 20, Col: 5}, Extensions: xmcp(boolField("skip", false), strField("description", "Fetch a pet by its id."))},
		Params: []gomodel.ParamGroup{
			{In: spec.InPath, Decl: path, Params: []*spec.Parameter{{Name: "id", In: spec.InPath, Required: true, Schema: strSchema}}},
			{In: spec.InQuery, Decl: getQuery, Params: []*spec.Parameter{{Name: "id", In: spec.InQuery, Schema: strSchema}, {Name: "body", In: spec.InQuery, Schema: strSchema}}},
		},
		Bodies:    []gomodel.Content{{MediaType: "application/json", Type: gomodel.Map{Key: str, Elem: str}}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}}}},
	}
	getAgain := &gomodel.Operation{
		Name:      "GetPet2",
		Spec:      &spec.Operation{ID: "get-pet", Method: "POST", Path: "/pets/{id}", Origin: spec.Origin{Pointer: "/paths/~1pets~1{id}/post", File: "api.yaml", Line: 25, Col: 5}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}}}},
	}
	put := &gomodel.Operation{
		Name:      "PutPet",
		Spec:      &spec.Operation{ID: "putPet", Method: "PUT", Path: "/pets/{id}", Origin: spec.Origin{Pointer: "/paths/~1pets~1{id}/put", File: "api.yaml", Line: 30, Col: 5}, Extensions: xmcp(strField("name", "put pet!"))},
		Bodies:    []gomodel.Content{{MediaType: "image/png", Type: file}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain", Type: str}}}},
	}
	del := &gomodel.Operation{
		Name:      "DeletePet",
		Spec:      &spec.Operation{ID: "deletePet", Method: "DELETE", Path: "/pets/{id}", Origin: spec.Origin{Pointer: "/paths/~1pets~1{id}/delete"}, Deprecated: true, Extensions: xmcp(boolField("skip", false), strField("name", "remove-pet"))},
		Responses: []gomodel.Response{{Status: "204"}},
	}
	ping := &gomodel.Operation{
		Name:      "Ping",
		Spec:      &spec.Operation{ID: "ping", Method: "QUERY", Path: "/ping", Origin: spec.Origin{Pointer: "/paths/~1ping/query"}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain"}}}},
	}
	noteOp := &gomodel.Operation{
		Name:      "Note",
		Spec:      &spec.Operation{ID: "note", Method: "POST", Path: "/note", Origin: spec.Origin{Pointer: "/paths/~1note/post"}, Body: &spec.RequestBody{Contents: []*spec.MediaType{{Name: "text/markdown"}}}},
		Bodies:    []gomodel.Content{{MediaType: "text/markdown", Type: gomodel.DeclRef{Decl: note}}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "text/markdown", Type: gomodel.DeclRef{Decl: note}}}}},
	}
	uploadOp := &gomodel.Operation{
		Name:      "Upload",
		Spec:      &spec.Operation{ID: "upload", Method: "POST", Path: "/upload", Origin: spec.Origin{Pointer: "/paths/~1upload/post"}},
		Bodies:    []gomodel.Content{{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: upload}}, {MediaType: "image/png"}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "image/png"}}}},
	}
	tail := &gomodel.Operation{
		Name:      "Tail",
		Spec:      &spec.Operation{ID: "tail", Method: "GET", Path: "/tail", Origin: spec.Origin{Pointer: "/paths/~1tail/get"}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "text/event-stream", Type: str, Item: str}}}},
	}
	skipped := &gomodel.Operation{Name: "Reset", Spec: &spec.Operation{ID: "reset", Method: "POST", Path: "/reset", Extensions: xmcp(boolField("skip", true))}}
	hook := &gomodel.Operation{Name: "Hook", Spec: &spec.Operation{ID: "hook", Method: "POST", Path: "/hook", IsWebhook: true}}
	return &gomodel.Model{
		Decls:      []*gomodel.Decl{pet, problem, note, upload, query, headers, cookies, querystring, path, getQuery},
		Operations: []*gomodel.Operation{list, create, get, getAgain, put, del, ping, noteOp, uploadOp, tail, skipped, hook},
	}
}
